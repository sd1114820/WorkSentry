using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Net;
using System.Net.Http;
using System.Net.Sockets;
using System.Security.Authentication;
using System.Text;
using System.Text.Json;
using System.Threading.Tasks;

namespace WorkSentry.Client;

internal sealed class NetworkDiagnosticSnapshot
{
    public const string FeedbackPrefix = "WSNET1";

    public string FeedbackPayload { get; init; } = string.Empty;
    public string Category { get; init; } = string.Empty;
    public string StatusToken { get; init; } = string.Empty;
    public string? ErrorCode { get; init; }
    public string? TargetHost { get; init; }
    public string? RequestUrl { get; init; }
    public string? ResponsePreview { get; init; }
    public int? HttpStatusCode { get; init; }
    public string? HttpReasonPhrase { get; init; }
    public string? SocketErrorCode { get; init; }
    public string? HttpRequestError { get; init; }
    public string? DnsError { get; init; }
    public IReadOnlyList<string> DnsAddresses { get; init; } = Array.Empty<string>();
    public long? DnsLookupMilliseconds { get; init; }
    public DateTime OccurredAtUtc { get; init; }
}

internal sealed class DiagnosticFeedbackEnvelope
{
    public int SchemaVersion { get; set; }
    public string FeedbackType { get; set; } = string.Empty;
    public string OccurredAtUtc { get; set; } = string.Empty;
    public string Category { get; set; } = string.Empty;
    public string StatusToken { get; set; } = string.Empty;
    public string ErrorCode { get; set; } = string.Empty;
    public string ServerUrl { get; set; } = string.Empty;
    public string RequestUrl { get; set; } = string.Empty;
    public string Host { get; set; } = string.Empty;
    public string Scheme { get; set; } = string.Empty;
    public int? Port { get; set; }
    public string EmployeeCode { get; set; } = string.Empty;
    public string ReportType { get; set; } = string.Empty;
    public string ClientVersion { get; set; } = string.Empty;
    public string OsVersion { get; set; } = string.Empty;
    public string DotNetVersion { get; set; } = string.Empty;
    public int? HttpStatusCode { get; set; }
    public string HttpReasonPhrase { get; set; } = string.Empty;
    public string ResponsePreview { get; set; } = string.Empty;
    public string ExceptionType { get; set; } = string.Empty;
    public string ExceptionMessage { get; set; } = string.Empty;
    public string ExceptionChain { get; set; } = string.Empty;
    public string SocketErrorCode { get; set; } = string.Empty;
    public string HttpRequestError { get; set; } = string.Empty;
    public string DnsError { get; set; } = string.Empty;
    public long? DnsLookupMilliseconds { get; set; }
    public string[] DnsAddresses { get; set; } = Array.Empty<string>();
    public string LastSuccessfulReportAtUtc { get; set; } = string.Empty;
}

internal sealed class DiagnosticDnsResult
{
    public string[] Addresses { get; init; } = Array.Empty<string>();
    public string? Error { get; init; }
    public long? LookupMilliseconds { get; init; }
}

internal static class NetworkDiagnostics
{
    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase
    };

    private const int ResponsePreviewMaxLength = 8192;
    private const int ExceptionChainMaxLength = 16384;

    public static NetworkDiagnosticSnapshot? Create(
        Exception exception,
        string serverUrl,
        string employeeCode,
        string? reportType,
        DateTime lastSuccessfulReportAtUtc,
        string? fallbackStatusToken)
    {
        if (!ShouldCapture(exception))
        {
            return null;
        }

        var occurredAtUtc = DateTime.UtcNow;
        var uri = TryParseUri(serverUrl);
        var apiException = FindApiException(exception);
        var httpException = FindInnerException<HttpRequestException>(exception);
        var socketException = FindInnerException<SocketException>(exception);
        var authException = FindInnerException<AuthenticationException>(exception);
        var dnsResult = ResolveDns(uri);
        var statusToken = ResolveStatusToken(fallbackStatusToken, exception, apiException);
        var category = ResolveCategory(exception, apiException, httpException, socketException, authException, dnsResult, statusToken, uri);
        if (!ShouldExposeFeedback(category))
        {
            return null;
        }

        var requestUrl = apiException?.RequestUri ?? uri?.ToString();
        var responsePreview = Truncate(apiException?.RawResponseBody, ResponsePreviewMaxLength);
        var errorCode = NormalizeCode(apiException?.ErrorCode, null);

        var envelope = new DiagnosticFeedbackEnvelope
        {
            SchemaVersion = 1,
            FeedbackType = "network_diagnostic",
            OccurredAtUtc = occurredAtUtc.ToString("O"),
            Category = category,
            StatusToken = statusToken,
            ErrorCode = errorCode ?? string.Empty,
            ServerUrl = serverUrl ?? string.Empty,
            RequestUrl = requestUrl ?? string.Empty,
            Host = uri?.Host ?? string.Empty,
            Scheme = uri?.Scheme ?? string.Empty,
            Port = uri is { IsDefaultPort: false } ? uri.Port : null,
            EmployeeCode = employeeCode ?? string.Empty,
            ReportType = reportType ?? string.Empty,
            ClientVersion = AppConstants.ClientVersion,
            OsVersion = Environment.OSVersion.VersionString,
            DotNetVersion = Environment.Version.ToString(),
            HttpStatusCode = apiException != null
                ? (int)apiException.StatusCode
                : httpException?.StatusCode is HttpStatusCode statusCode ? (int)statusCode : null,
            HttpReasonPhrase = apiException?.ReasonPhrase ?? string.Empty,
            ResponsePreview = responsePreview ?? string.Empty,
            ExceptionType = exception.GetType().FullName ?? string.Empty,
            ExceptionMessage = exception.Message ?? string.Empty,
            ExceptionChain = Truncate(BuildExceptionChain(exception), ExceptionChainMaxLength) ?? string.Empty,
            SocketErrorCode = socketException?.SocketErrorCode.ToString() ?? string.Empty,
            HttpRequestError = httpException?.HttpRequestError.ToString() ?? string.Empty,
            DnsError = dnsResult.Error ?? string.Empty,
            DnsLookupMilliseconds = dnsResult.LookupMilliseconds,
            DnsAddresses = dnsResult.Addresses,
            LastSuccessfulReportAtUtc = lastSuccessfulReportAtUtc == DateTime.MinValue ? string.Empty : lastSuccessfulReportAtUtc.ToString("O")
        };

        var json = JsonSerializer.Serialize(envelope, JsonOptions);
        var feedbackPayload = Convert.ToBase64String(Encoding.UTF8.GetBytes(json));

        return new NetworkDiagnosticSnapshot
        {
            FeedbackPayload = feedbackPayload,
            Category = category,
            StatusToken = statusToken,
            ErrorCode = errorCode,
            TargetHost = uri?.Host,
            RequestUrl = requestUrl,
            ResponsePreview = responsePreview,
            HttpStatusCode = envelope.HttpStatusCode,
            HttpReasonPhrase = envelope.HttpReasonPhrase,
            SocketErrorCode = envelope.SocketErrorCode,
            HttpRequestError = envelope.HttpRequestError,
            DnsError = envelope.DnsError,
            DnsAddresses = envelope.DnsAddresses,
            DnsLookupMilliseconds = envelope.DnsLookupMilliseconds,
            OccurredAtUtc = occurredAtUtc
        };
    }

    public static string GetSummary(NetworkDiagnosticSnapshot snapshot)
    {
        return snapshot.Category switch
        {
            "dns_error" => LanguageService.GetString("DiagnosticSummaryDns"),
            "connection_refused" => LanguageService.GetString("DiagnosticSummaryConnectionRefused"),
            "timeout" => LanguageService.GetString("DiagnosticSummaryTimeout"),
            "tls_error" => LanguageService.GetString("DiagnosticSummaryTls"),
            "auth_error" => LanguageService.GetString("DiagnosticSummaryAuth"),
            "employee_not_found" => LanguageService.GetString("DiagnosticSummaryEmployeeNotFound"),
            "employee_disabled" => LanguageService.GetString("DiagnosticSummaryEmployeeDisabled"),
            "device_mismatch" => LanguageService.GetString("DiagnosticSummaryDeviceMismatch"),
            "config_error" => LanguageService.GetString("DiagnosticSummaryConfig"),
            "server_error" => LanguageService.GetString("DiagnosticSummaryServer"),
            "request_error" => LanguageService.GetString("DiagnosticSummaryRequest"),
            _ => LanguageService.GetString("DiagnosticSummaryUnknown")
        };
    }

    public static string GetPossibleCause(NetworkDiagnosticSnapshot snapshot)
    {
        return snapshot.Category switch
        {
            "dns_error" => LanguageService.GetString("DiagnosticCauseDns"),
            "connection_refused" => LanguageService.GetString("DiagnosticCauseConnectionRefused"),
            "timeout" => LanguageService.GetString("DiagnosticCauseTimeout"),
            "tls_error" => LanguageService.GetString("DiagnosticCauseTls"),
            "auth_error" => LanguageService.GetString("DiagnosticCauseAuth"),
            "employee_not_found" => LanguageService.GetString("DiagnosticCauseEmployeeNotFound"),
            "employee_disabled" => LanguageService.GetString("DiagnosticCauseEmployeeDisabled"),
            "device_mismatch" => LanguageService.GetString("DiagnosticCauseDeviceMismatch"),
            "config_error" => LanguageService.GetString("DiagnosticCauseConfig"),
            "server_error" => LanguageService.GetString("DiagnosticCauseServer"),
            "request_error" => LanguageService.GetString("DiagnosticCauseRequest"),
            _ => LanguageService.GetString("DiagnosticCauseUnknown")
        };
    }

    public static string GetDetailText(NetworkDiagnosticSnapshot snapshot)
    {
        var details = new List<string>();

        if (!string.IsNullOrWhiteSpace(snapshot.TargetHost))
        {
            details.Add(LanguageService.Format("DiagnosticDetailTargetFormat", snapshot.TargetHost));
        }

        var dnsText = GetDnsText(snapshot);
        if (!string.IsNullOrWhiteSpace(dnsText))
        {
            details.Add(LanguageService.Format("DiagnosticDetailDnsFormat", dnsText));
        }

        if (snapshot.HttpStatusCode.HasValue)
        {
            var httpText = snapshot.HttpStatusCode.Value.ToString();
            if (!string.IsNullOrWhiteSpace(snapshot.HttpReasonPhrase))
            {
                httpText += " " + snapshot.HttpReasonPhrase;
            }
            details.Add(LanguageService.Format("DiagnosticDetailHttpFormat", httpText));
        }

        if (!string.IsNullOrWhiteSpace(snapshot.ErrorCode))
        {
            details.Add(LanguageService.Format("DiagnosticDetailCodeFormat", snapshot.ErrorCode));
        }

        if (!string.IsNullOrWhiteSpace(snapshot.SocketErrorCode))
        {
            details.Add(LanguageService.Format("DiagnosticDetailSocketFormat", snapshot.SocketErrorCode));
        }

        return details.Count == 0
            ? LanguageService.GetString("DiagnosticDetailFallback")
            : string.Join("  ", details);
    }

    private static bool ShouldExposeFeedback(string category)
    {
        return category is "dns_error"
            or "connection_refused"
            or "timeout"
            or "tls_error"
            or "config_error"
            or "server_error"
            or "network_unknown";
    }

    public static Dictionary<string, string> BuildQueueContext(NetworkDiagnosticSnapshot? snapshot)
    {
        if (snapshot == null)
        {
            return new Dictionary<string, string>();
        }

        return new Dictionary<string, string>
        {
            ["diagnosticCategory"] = snapshot.Category,
            ["diagnosticStatus"] = snapshot.StatusToken,
            ["diagnosticErrorCode"] = snapshot.ErrorCode ?? string.Empty,
            ["diagnosticHttpStatus"] = snapshot.HttpStatusCode?.ToString() ?? string.Empty,
            ["diagnosticSocketError"] = snapshot.SocketErrorCode ?? string.Empty,
            ["diagnosticDnsError"] = snapshot.DnsError ?? string.Empty,
            ["diagnosticDnsAddresses"] = snapshot.DnsAddresses.Count == 0 ? string.Empty : string.Join(",", snapshot.DnsAddresses),
            ["diagnosticFeedbackPrefix"] = NetworkDiagnosticSnapshot.FeedbackPrefix
        };
    }

    private static bool ShouldCapture(Exception exception)
    {
        if (exception is UriFormatException || exception is ArgumentException || exception is TaskCanceledException)
        {
            return true;
        }

        if (exception is ApiException || exception is HttpRequestException)
        {
            return true;
        }

        return FindInnerException<HttpRequestException>(exception) != null
            || FindInnerException<SocketException>(exception) != null
            || FindInnerException<AuthenticationException>(exception) != null;
    }

    private static string ResolveStatusToken(string? fallbackStatusToken, Exception exception, ApiException? apiException)
    {
        if (!string.IsNullOrWhiteSpace(fallbackStatusToken))
        {
            return fallbackStatusToken.Trim();
        }

        if (exception is UriFormatException || exception is ArgumentException)
        {
            return "配置错误";
        }

        if (exception is TaskCanceledException)
        {
            return "网络异常";
        }

        if (apiException != null)
        {
            return apiException.StatusCode switch
            {
                HttpStatusCode.Unauthorized => "登录失效",
                HttpStatusCode.Forbidden => "登录失效",
                HttpStatusCode.NotFound => "工号不存在",
                HttpStatusCode.UpgradeRequired => "需要更新",
                _ when (int)apiException.StatusCode >= 500 => "服务端异常",
                _ => "网络异常"
            };
        }

        return "网络异常";
    }

    private static string ResolveCategory(
        Exception exception,
        ApiException? apiException,
        HttpRequestException? httpException,
        SocketException? socketException,
        AuthenticationException? authException,
        DiagnosticDnsResult dnsResult,
        string statusToken,
        Uri? uri)
    {
        if (exception is UriFormatException || exception is ArgumentException || uri == null)
        {
            return "config_error";
        }

        if (statusToken == "登录失效")
        {
            return "auth_error";
        }
        if (statusToken == "工号不存在")
        {
            return "employee_not_found";
        }
        if (statusToken == "员工已停用")
        {
            return "employee_disabled";
        }
        if (statusToken == "设备不匹配")
        {
            return "device_mismatch";
        }

        var requestError = httpException?.HttpRequestError.ToString() ?? string.Empty;
        if (string.Equals(requestError, "NameResolutionError", StringComparison.OrdinalIgnoreCase))
        {
            return "dns_error";
        }
        if (string.Equals(requestError, "SecureConnectionError", StringComparison.OrdinalIgnoreCase))
        {
            return "tls_error";
        }

        if (authException != null)
        {
            return "tls_error";
        }

        if (socketException != null)
        {
            if (socketException.SocketErrorCode is SocketError.HostNotFound or SocketError.NoData or SocketError.TryAgain)
            {
                return "dns_error";
            }
            if (socketException.SocketErrorCode == SocketError.ConnectionRefused)
            {
                return "connection_refused";
            }
            if (socketException.SocketErrorCode == SocketError.TimedOut)
            {
                return "timeout";
            }
        }

        if (!string.IsNullOrWhiteSpace(dnsResult.Error) && apiException == null)
        {
            return "dns_error";
        }

        if (exception is TaskCanceledException)
        {
            return "timeout";
        }

        if (apiException != null)
        {
            if ((int)apiException.StatusCode >= 500)
            {
                return "server_error";
            }
            if ((int)apiException.StatusCode >= 400)
            {
                return "request_error";
            }
        }

        return "network_unknown";
    }

    private static DiagnosticDnsResult ResolveDns(Uri? uri)
    {
        if (uri == null || string.IsNullOrWhiteSpace(uri.Host))
        {
            return new DiagnosticDnsResult();
        }

        var sw = Stopwatch.StartNew();
        try
        {
            var task = Dns.GetHostAddressesAsync(uri.DnsSafeHost);
            if (!task.Wait(TimeSpan.FromSeconds(3)))
            {
                return new DiagnosticDnsResult
                {
                    Error = "lookup_timeout",
                    LookupMilliseconds = sw.ElapsedMilliseconds
                };
            }

            return new DiagnosticDnsResult
            {
                Addresses = Array.ConvertAll(task.Result, item => item.ToString()),
                LookupMilliseconds = sw.ElapsedMilliseconds
            };
        }
        catch (Exception ex)
        {
            return new DiagnosticDnsResult
            {
                Error = ex.Message,
                LookupMilliseconds = sw.ElapsedMilliseconds
            };
        }
    }

    private static ApiException? FindApiException(Exception exception)
    {
        return exception as ApiException ?? FindInnerException<ApiException>(exception);
    }

    private static T? FindInnerException<T>(Exception exception) where T : Exception
    {
        Exception? current = exception;
        while (current != null)
        {
            if (current is T matched)
            {
                return matched;
            }
            current = current.InnerException;
        }
        return null;
    }

    private static Uri? TryParseUri(string? serverUrl)
    {
        if (string.IsNullOrWhiteSpace(serverUrl))
        {
            return null;
        }

        return Uri.TryCreate(serverUrl.Trim(), UriKind.Absolute, out var uri) ? uri : null;
    }

    private static string BuildExceptionChain(Exception exception)
    {
        var parts = new List<string>();
        Exception? current = exception;
        while (current != null)
        {
            parts.Add($"{current.GetType().FullName}: {current.Message}");
            current = current.InnerException;
        }
        return string.Join(" | ", parts);
    }

    private static string? NormalizeCode(string? code, string? fallback)
    {
        if (!string.IsNullOrWhiteSpace(code))
        {
            return code.Trim();
        }
        if (!string.IsNullOrWhiteSpace(fallback))
        {
            return fallback.Trim();
        }
        return null;
    }

    private static string? Truncate(string? value, int maxLength)
    {
        if (string.IsNullOrWhiteSpace(value))
        {
            return value;
        }

        var text = value.Trim();
        if (text.Length <= maxLength)
        {
            return text;
        }

        return text.Substring(0, maxLength) + "...";
    }

    private static string? GetDnsText(NetworkDiagnosticSnapshot snapshot)
    {
        if (!string.IsNullOrWhiteSpace(snapshot.DnsError))
        {
            return LanguageService.Format("DiagnosticDnsFailedFormat", snapshot.DnsError);
        }

        if (snapshot.DnsAddresses.Count > 0)
        {
            return LanguageService.Format("DiagnosticDnsSuccessFormat", string.Join(", ", snapshot.DnsAddresses));
        }

        return null;
    }
}
