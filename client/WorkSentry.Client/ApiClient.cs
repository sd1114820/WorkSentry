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
        if (response.IsSuccessStatusCode)
        {
            var payload = await response.Content.ReadAsStringAsync(ct).ConfigureAwait(false);
            return JsonSerializer.Deserialize<T>(payload, _options) ?? throw new ApiException(response.StatusCode, "响应解析失败");
        }

        var error = await ReadErrorResponseAsync(response, ct).ConfigureAwait(false);
        var message = !string.IsNullOrWhiteSpace(error.Message)
            ? error.Message
            : $"请求失败: {(int)response.StatusCode}";
        if (!string.IsNullOrWhiteSpace(error.Code) && string.Equals(error.Code, "need_reason", StringComparison.OrdinalIgnoreCase))
        {
            throw new NeedReasonException(response.StatusCode, message, error.Data);
        }
        if (response.StatusCode is HttpStatusCode.Unauthorized or HttpStatusCode.Forbidden)
        {
            throw new UnauthorizedException(message);
        }
        if (response.StatusCode == HttpStatusCode.UpgradeRequired)
        {
            throw new UpgradeRequiredException(message);
        }

        throw new ApiException(response.StatusCode, message);
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
        if (response.IsSuccessStatusCode)
        {
            var payload = await response.Content.ReadAsStringAsync(ct).ConfigureAwait(false);
            return JsonSerializer.Deserialize<T>(payload, _options) ?? throw new ApiException(response.StatusCode, "响应解析失败");
        }

        var error = await ReadErrorResponseAsync(response, ct).ConfigureAwait(false);
        var message = !string.IsNullOrWhiteSpace(error.Message)
            ? error.Message
            : $"请求失败: {(int)response.StatusCode}";
        if (!string.IsNullOrWhiteSpace(error.Code) && string.Equals(error.Code, "need_reason", StringComparison.OrdinalIgnoreCase))
        {
            throw new NeedReasonException(response.StatusCode, message, error.Data);
        }
        if (response.StatusCode is HttpStatusCode.Unauthorized or HttpStatusCode.Forbidden)
        {
            throw new UnauthorizedException(message);
        }
        if (response.StatusCode == HttpStatusCode.UpgradeRequired)
        {
            throw new UpgradeRequiredException(message);
        }

        throw new ApiException(response.StatusCode, message);
    }

    private async Task<ApiErrorResponse> ReadErrorResponseAsync(HttpResponseMessage response, CancellationToken ct)
    {
        try
        {
            var payload = await response.Content.ReadAsStringAsync(ct).ConfigureAwait(false);
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
        return new ApiErrorResponse
        {
            Message = string.Format("请求失败: {0}", (int)response.StatusCode)
        };
    }
}

internal class ApiException : Exception
{
    public HttpStatusCode StatusCode { get; }

    public ApiException(HttpStatusCode statusCode, string message) : base(message)
    {
        StatusCode = statusCode;
    }
}

internal sealed class UnauthorizedException : ApiException
{
    public UnauthorizedException(string message) : base(HttpStatusCode.Unauthorized, message)
    {
    }
}

internal sealed class UpgradeRequiredException : ApiException
{
    public UpgradeRequiredException(string message) : base(HttpStatusCode.UpgradeRequired, message)
    {
    }
}

internal sealed class NeedReasonException : ApiException
{
    public new JsonElement? Data { get; }

    public NeedReasonException(HttpStatusCode statusCode, string message, JsonElement? data) : base(statusCode, message)
    {
        Data = data;
    }
}




