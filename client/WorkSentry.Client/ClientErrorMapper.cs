using System;
using System.Net;
using System.Net.Http;
using System.Text.Json;
using System.Threading.Tasks;

namespace WorkSentry.Client;

internal sealed class ClientErrorMapping
{
    public string StatusToken { get; }
    public string Message { get; }
    public string? ErrorCode { get; }
    public JsonElement? ErrorData { get; }
    public bool ShouldClearToken { get; }
    public bool ShouldTriggerForcedUpdate { get; }
    public bool ShouldFocusEmployeeCode { get; }

    public ClientErrorMapping(
        string statusToken,
        string message,
        string? errorCode = null,
        JsonElement? errorData = null,
        bool shouldClearToken = false,
        bool shouldTriggerForcedUpdate = false,
        bool shouldFocusEmployeeCode = false)
    {
        StatusToken = statusToken;
        Message = message;
        ErrorCode = string.IsNullOrWhiteSpace(errorCode) ? null : errorCode;
        ErrorData = errorData;
        ShouldClearToken = shouldClearToken;
        ShouldTriggerForcedUpdate = shouldTriggerForcedUpdate;
        ShouldFocusEmployeeCode = shouldFocusEmployeeCode;
    }
}

internal static class ClientErrorMapper
{
    public static ClientErrorMapping Map(Exception exception, string configPath)
    {
        if (exception is UpgradeRequiredException upgrade)
        {
            return new ClientErrorMapping(
                "需要更新",
                LanguageService.GetString("ErrUpgradeRequired"),
                NormalizeCode(upgrade.ErrorCode, "upgrade_required"),
                upgrade.ErrorData,
                shouldTriggerForcedUpdate: true);
        }

        if (exception is HttpRequestException)
        {
            return new ClientErrorMapping(
                "网络异常",
                LanguageService.Format("ErrNetworkWithConfigFormat", configPath),
                "network_error");
        }

        if (exception is TaskCanceledException)
        {
            return new ClientErrorMapping(
                "网络异常",
                LanguageService.Format("ErrTimeoutWithConfigFormat", configPath),
                "request_timeout");
        }

        if (exception is UriFormatException || exception is ArgumentException)
        {
            return new ClientErrorMapping(
                "配置错误",
                LanguageService.Format("ErrConfigServerUrlInvalidWithConfigFormat", configPath),
                "invalid_server_url");
        }

        if (exception is InvalidOperationException invalid && invalid.Message.Contains("工号不能为空", StringComparison.OrdinalIgnoreCase))
        {
            return new ClientErrorMapping(
                "配置错误",
                LanguageService.GetString("MsgFillEmployeeCode"),
                "employee_code_empty",
                shouldFocusEmployeeCode: true);
        }

        if (exception is ApiException apiException)
        {
            return MapApiError(apiException);
        }

        return new ClientErrorMapping(
            "服务端异常",
            LanguageService.GetString("ErrServerError"),
            "unknown_error");
    }

    private static ClientErrorMapping MapApiError(ApiException exception)
    {
        var code = NormalizeCode(exception.ErrorCode, null);
        if (string.Equals(code, "need_reason", StringComparison.OrdinalIgnoreCase))
        {
            return new ClientErrorMapping("连接中", exception.Message, code, exception.ErrorData);
        }

        if (string.Equals(code, "employee_not_found", StringComparison.OrdinalIgnoreCase))
        {
            return new ClientErrorMapping(
                "工号不存在",
                LanguageService.GetString("ErrEmployeeNotFound"),
                code,
                exception.ErrorData,
                shouldFocusEmployeeCode: true);
        }

        if (string.Equals(code, "employee_disabled", StringComparison.OrdinalIgnoreCase))
        {
            return new ClientErrorMapping(
                "员工已停用",
                LanguageService.GetString("ErrEmployeeDisabled"),
                code,
                exception.ErrorData);
        }

        if (string.Equals(code, "device_mismatch", StringComparison.OrdinalIgnoreCase))
        {
            return new ClientErrorMapping(
                "设备不匹配",
                LanguageService.GetString("ErrDeviceMismatch"),
                code,
                exception.ErrorData);
        }

        if (string.Equals(code, "employee_code_or_fingerprint_empty", StringComparison.OrdinalIgnoreCase))
        {
            return new ClientErrorMapping(
                "配置错误",
                LanguageService.GetString("MsgFillEmployeeCode"),
                code,
                exception.ErrorData,
                shouldFocusEmployeeCode: true);
        }

        if (string.Equals(code, "token_missing", StringComparison.OrdinalIgnoreCase)
            || string.Equals(code, "token_invalid", StringComparison.OrdinalIgnoreCase)
            || string.Equals(code, "token_revoked", StringComparison.OrdinalIgnoreCase)
            || string.Equals(code, "token_expired", StringComparison.OrdinalIgnoreCase)
            || string.Equals(code, "token_validation_failed", StringComparison.OrdinalIgnoreCase))
        {
            return new ClientErrorMapping(
                "登录失效",
                LanguageService.GetString("ErrAuthInvalid"),
                code,
                exception.ErrorData,
                shouldClearToken: true);
        }

        if (string.Equals(code, "upgrade_required", StringComparison.OrdinalIgnoreCase))
        {
            return new ClientErrorMapping(
                "需要更新",
                LanguageService.GetString("ErrUpgradeRequired"),
                code,
                exception.ErrorData,
                shouldTriggerForcedUpdate: true);
        }

        if (string.Equals(code, "server_error", StringComparison.OrdinalIgnoreCase))
        {
            return new ClientErrorMapping(
                "服务端异常",
                LanguageService.GetString("ErrServerError"),
                code,
                exception.ErrorData);
        }

        if (exception.StatusCode == HttpStatusCode.Unauthorized)
        {
            return new ClientErrorMapping(
                "登录失效",
                LanguageService.GetString("ErrAuthInvalid"),
                code ?? "unauthorized",
                exception.ErrorData,
                shouldClearToken: true);
        }

        if (exception.StatusCode == HttpStatusCode.Forbidden)
        {
            if (!string.IsNullOrWhiteSpace(exception.Message) && exception.Message.Contains("停用", StringComparison.OrdinalIgnoreCase))
            {
                return new ClientErrorMapping(
                    "员工已停用",
                    LanguageService.GetString("ErrEmployeeDisabled"),
                    code ?? "employee_disabled",
                    exception.ErrorData);
            }

            if (!string.IsNullOrWhiteSpace(exception.Message) && exception.Message.Contains("设备不匹配", StringComparison.OrdinalIgnoreCase))
            {
                return new ClientErrorMapping(
                    "设备不匹配",
                    LanguageService.GetString("ErrDeviceMismatch"),
                    code ?? "device_mismatch",
                    exception.ErrorData);
            }

            return new ClientErrorMapping(
                "登录失效",
                LanguageService.GetString("ErrAuthInvalid"),
                code ?? "forbidden",
                exception.ErrorData,
                shouldClearToken: true);
        }

        if (exception.StatusCode == HttpStatusCode.NotFound)
        {
            return new ClientErrorMapping(
                "工号不存在",
                LanguageService.GetString("ErrEmployeeNotFound"),
                code ?? "not_found",
                exception.ErrorData,
                shouldFocusEmployeeCode: true);
        }

        if (exception.StatusCode == HttpStatusCode.UpgradeRequired)
        {
            return new ClientErrorMapping(
                "需要更新",
                LanguageService.GetString("ErrUpgradeRequired"),
                code ?? "upgrade_required",
                exception.ErrorData,
                shouldTriggerForcedUpdate: true);
        }

        if ((int)exception.StatusCode >= 500)
        {
            return new ClientErrorMapping(
                "服务端异常",
                LanguageService.GetString("ErrServerError"),
                code ?? "server_error",
                exception.ErrorData);
        }

        var message = string.IsNullOrWhiteSpace(exception.Message)
            ? LanguageService.GetString("ErrServerError")
            : exception.Message;

        return new ClientErrorMapping(
            "服务端异常",
            message,
            code ?? "api_error",
            exception.ErrorData);
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
}
