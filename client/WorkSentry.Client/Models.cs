using System;

namespace WorkSentry.Client;

internal sealed class AppConfig
{
    public string ServerUrl { get; set; } = "http://127.0.0.1:8080";
    public string EmployeeCode { get; set; } = "";
    public string ClientVersion { get; set; } = AppConstants.ClientVersion;
    public int IdleThresholdSeconds { get; set; } = 300;
    public int HeartbeatIntervalSeconds { get; set; } = 300;
    public int OfflineThresholdSeconds { get; set; } = 600;
    public int UpdatePolicy { get; set; } = 0;
    public string LatestVersion { get; set; } = "";
    public string UpdateUrl { get; set; } = "";
    public bool SuppressCloseTip { get; set; } = false;
    public string LanguageOverride { get; set; } = "";
    public DateTime? LastConfigAt { get; set; }
}

internal sealed class ClientBindRequest
{
    public string EmployeeCode { get; set; } = "";
    public string Fingerprint { get; set; } = "";
    public string ClientVersion { get; set; } = "";
}

internal sealed class ClientBindResponse
{
    public string Token { get; set; } = "";
    public int IdleThresholdSeconds { get; set; }
    public int HeartbeatIntervalSeconds { get; set; }
    public int OfflineThresholdSeconds { get; set; }
    public int UpdatePolicy { get; set; }
    public string LatestVersion { get; set; } = "";
    public string UpdateUrl { get; set; } = "";
    public string ServerTime { get; set; } = "";
}

internal sealed class ClientReportRequest
{
    public string ProcessName { get; set; } = "";
    public string WindowTitle { get; set; } = "";
    public int IdleSeconds { get; set; }
    public string ClientVersion { get; set; } = "";
    public string ReportType { get; set; } = "";
}

internal sealed class ClientReportResponse
{
    public int IdleThresholdSeconds { get; set; }
    public int HeartbeatIntervalSeconds { get; set; }
    public int OfflineThresholdSeconds { get; set; }
    public int UpdatePolicy { get; set; }
    public string LatestVersion { get; set; } = "";
    public string UpdateUrl { get; set; } = "";
    public string ServerTime { get; set; } = "";
}

internal sealed class ApiErrorResponse
{
    public string Message { get; set; } = "";
}

internal sealed record SampleState(string ProcessName, string WindowTitle, int IdleSeconds, bool IsIdle);
