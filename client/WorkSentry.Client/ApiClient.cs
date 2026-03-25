using System;
using System.Net;
using System.Net.Http;
using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using System.Threading;
using System.Threading.Tasks;

namespace WorkSentry.Client;

internal sealed class ApiClient
{
    private readonly HttpClient _httpClient;
    private readonly JsonSerializerOptions _options = new() { PropertyNamingPolicy = JsonNamingPolicy.CamelCase };

    public ApiClient(string serverUrl)
    {
        _httpClient = new HttpClient
        {
            BaseAddress = new Uri(serverUrl.TrimEnd('/')),
            Timeout = TimeSpan.FromSeconds(10)
        };
    }

    public Task<ClientBindResponse> BindAsync(ClientBindRequest request, CancellationToken ct)
    {
        return PostAsync<ClientBindResponse>("/api/v1/client/bind", request, null, ct);
    }

    public Task<ClientReportResponse> ReportAsync(ClientReportRequest request, string token, CancellationToken ct)
    {
        return PostAsync<ClientReportResponse>("/api/v1/client/report", request, token, ct);
    }

    public Task<ClientErrorReportResponse> ReportErrorAsync(ClientErrorReportRequest request, string token, CancellationToken ct)
    {
        return PostAsync<ClientErrorReportResponse>("/api/v1/client/error", request, token, ct);
    }

    public Task<CheckoutTemplateResponse> GetCheckoutTemplateAsync(string token, CancellationToken ct)
    {
        return GetAsync<CheckoutTemplateResponse>("/api/v1/client/checkout-template", token, ct);
    }

    private async Task<T> GetAsync<T>(string path, string? token, CancellationToken ct)
    {
        using var request = new HttpRequestMessage(HttpMethod.Get, path);
        if (!string.IsNullOrWhiteSpace(token))
        {
            request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        }

        using var response = await _httpClient.SendAsync(request, ct).ConfigureAwait(false);
        var payload = await response.Content.ReadAsStringAsync(ct).ConfigureAwait(false);
        if (response.IsSuccessStatusCode)
        {
            return JsonSerializer.Deserialize<T>(payload, _options)
                ?? throw CreateApiException(response, new ApiErrorResponse
                {
                    Message = "响应解析失败",
                    Code = "invalid_response"
                }, payload);
        }

        var error = ReadErrorResponse(response.StatusCode, payload);
        throw CreateApiException(response, error, payload);
    }

    private async Task<T> PostAsync<T>(string path, object body, string? token, CancellationToken ct)
    {
        var json = JsonSerializer.Serialize(body, _options);
        using var request = new HttpRequestMessage(HttpMethod.Post, path)
        {
            Content = new StringContent(json, Encoding.UTF8, "application/json")
        };
        if (!string.IsNullOrWhiteSpace(token))
        {
            request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        }

        using var response = await _httpClient.SendAsync(request, ct).ConfigureAwait(false);
        var payload = await response.Content.ReadAsStringAsync(ct).ConfigureAwait(false);
        if (response.IsSuccessStatusCode)
        {
            return JsonSerializer.Deserialize<T>(payload, _options)
                ?? throw CreateApiException(response, new ApiErrorResponse
                {
                    Message = "响应解析失败",
                    Code = "invalid_response"
                }, payload);
        }

        var error = ReadErrorResponse(response.StatusCode, payload);
        throw CreateApiException(response, error, payload);
    }

    private Exception CreateApiException(HttpResponseMessage response, ApiErrorResponse error, string? rawResponseBody)
    {
        var message = !string.IsNullOrWhiteSpace(error.Message)
            ? error.Message
            : $"请求失败: {(int)response.StatusCode}";
        var code = string.IsNullOrWhiteSpace(error.Code) ? null : error.Code;
        var requestUri = response.RequestMessage?.RequestUri?.ToString();
        var requestMethod = response.RequestMessage?.Method.Method;
        var reasonPhrase = response.ReasonPhrase;

        if (string.Equals(code, "need_reason", StringComparison.OrdinalIgnoreCase))
        {
            return new NeedReasonException(response.StatusCode, message, error.Data, requestUri, requestMethod, reasonPhrase, rawResponseBody);
        }

        if (response.StatusCode == HttpStatusCode.Unauthorized)
        {
            return new UnauthorizedException(message, code, error.Data, requestUri, requestMethod, reasonPhrase, rawResponseBody);
        }
        if (response.StatusCode == HttpStatusCode.Forbidden)
        {
            return new ForbiddenException(message, code, error.Data, requestUri, requestMethod, reasonPhrase, rawResponseBody);
        }
        if (response.StatusCode == HttpStatusCode.UpgradeRequired)
        {
            return new UpgradeRequiredException(message, code, error.Data, requestUri, requestMethod, reasonPhrase, rawResponseBody);
        }

        return new ApiException(response.StatusCode, message, code, error.Data, requestUri, requestMethod, reasonPhrase, rawResponseBody);
    }

    private ApiErrorResponse ReadErrorResponse(HttpStatusCode statusCode, string? payload)
    {
        if (!string.IsNullOrWhiteSpace(payload))
        {
            try
            {
                var parsed = JsonSerializer.Deserialize<ApiErrorResponse>(payload, _options);
                if (parsed != null)
                {
                    return parsed;
                }
            }
            catch
            {
                // ignore
            }
        }

        return new ApiErrorResponse
        {
            Message = string.Format("请求失败: {0}", (int)statusCode)
        };
    }
}

internal class ApiException : Exception
{
    public HttpStatusCode StatusCode { get; }
    public string? ErrorCode { get; }
    public JsonElement? ErrorData { get; }
    public string? RequestUri { get; }
    public string? RequestMethod { get; }
    public string? ReasonPhrase { get; }
    public string? RawResponseBody { get; }

    public ApiException(
        HttpStatusCode statusCode,
        string message,
        string? errorCode = null,
        JsonElement? errorData = null,
        string? requestUri = null,
        string? requestMethod = null,
        string? reasonPhrase = null,
        string? rawResponseBody = null) : base(message)
    {
        StatusCode = statusCode;
        ErrorCode = string.IsNullOrWhiteSpace(errorCode) ? null : errorCode;
        ErrorData = errorData;
        RequestUri = string.IsNullOrWhiteSpace(requestUri) ? null : requestUri;
        RequestMethod = string.IsNullOrWhiteSpace(requestMethod) ? null : requestMethod;
        ReasonPhrase = string.IsNullOrWhiteSpace(reasonPhrase) ? null : reasonPhrase;
        RawResponseBody = string.IsNullOrWhiteSpace(rawResponseBody) ? null : rawResponseBody;
    }
}

internal sealed class UnauthorizedException : ApiException
{
    public UnauthorizedException(
        string message,
        string? errorCode = null,
        JsonElement? errorData = null,
        string? requestUri = null,
        string? requestMethod = null,
        string? reasonPhrase = null,
        string? rawResponseBody = null)
        : base(HttpStatusCode.Unauthorized, message, errorCode, errorData, requestUri, requestMethod, reasonPhrase, rawResponseBody)
    {
    }
}

internal sealed class ForbiddenException : ApiException
{
    public ForbiddenException(
        string message,
        string? errorCode = null,
        JsonElement? errorData = null,
        string? requestUri = null,
        string? requestMethod = null,
        string? reasonPhrase = null,
        string? rawResponseBody = null)
        : base(HttpStatusCode.Forbidden, message, errorCode, errorData, requestUri, requestMethod, reasonPhrase, rawResponseBody)
    {
    }
}

internal sealed class UpgradeRequiredException : ApiException
{
    public UpgradeRequiredException(
        string message,
        string? errorCode = null,
        JsonElement? errorData = null,
        string? requestUri = null,
        string? requestMethod = null,
        string? reasonPhrase = null,
        string? rawResponseBody = null)
        : base(HttpStatusCode.UpgradeRequired, message, errorCode, errorData, requestUri, requestMethod, reasonPhrase, rawResponseBody)
    {
    }
}

internal sealed class NeedReasonException : ApiException
{
    public new JsonElement? Data { get; }

    public NeedReasonException(
        HttpStatusCode statusCode,
        string message,
        JsonElement? data,
        string? requestUri = null,
        string? requestMethod = null,
        string? reasonPhrase = null,
        string? rawResponseBody = null)
        : base(statusCode, message, "need_reason", data, requestUri, requestMethod, reasonPhrase, rawResponseBody)
    {
        Data = data;
    }
}
