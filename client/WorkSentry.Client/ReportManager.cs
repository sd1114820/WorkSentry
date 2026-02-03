using System;
using System.Net.Http;
using System.Text.Json;
using System.Threading;
using System.Threading.Tasks;

namespace WorkSentry.Client;

internal sealed class ReportResult
{
    public bool Success { get; }
    public string? Error { get; }
    public string? ErrorCode { get; }
    public JsonElement? ErrorData { get; }

    private ReportResult(bool success, string? error, string? errorCode, JsonElement? errorData)
    {
        Success = success;
        Error = error;
        ErrorCode = errorCode;
        ErrorData = errorData;
    }

    public static ReportResult Ok()
    {
        return new ReportResult(true, null, null, null);
    }

    public static ReportResult Fail(string? error, string? code = null, JsonElement? data = null)
    {
        return new ReportResult(false, error, code, data);
    }
}

internal sealed class ReportManager
{
    private readonly AppConfig _config;
    private readonly ConfigStore _configStore;
    private readonly TokenStore _tokenStore;
    private readonly Logger _logger;
    private ApiClient _apiClient;
    private readonly NetworkBackoff _backoff = new();
    private CancellationTokenSource? _cts;
    private SampleState? _lastSample;
    private DateTime _lastHeartbeatAt = DateTime.MinValue;
    private string? _token;
    private string _optionalUpdateNotified = "";
    private bool _forceReport;
    private bool _isBreaking;

    public event Action<string?, string?>? ForcedUpdate;
    public event Action<string?, string?>? OptionalUpdate;
    public event Action<string>? StatusChanged;
    public event Action<AppConfig>? SettingsChanged;

    public ReportManager(AppConfig config, ConfigStore configStore, TokenStore tokenStore, Logger logger)
    {
        _config = config;
        _configStore = configStore;
        _tokenStore = tokenStore;
        _logger = logger;
        _apiClient = new ApiClient(_config.ServerUrl);
    }

    public async Task StartAsync()
    {
        _token = _tokenStore.LoadToken();
        await EnsureBoundAsync(CancellationToken.None).ConfigureAwait(false);

        var startupSample = Win32Interop.CaptureSample(_config.IdleThresholdSeconds);
        await SafeSendAsync(startupSample, "startup", CancellationToken.None).ConfigureAwait(false);

        _cts = new CancellationTokenSource();
        _ = Task.Run(() => LoopAsync(_cts.Token));
    }

    public void Stop()
    {
        _cts?.Cancel();
        _cts?.Dispose();
        _cts = null;
    }

    public void RequestImmediateReport()
    {
        _forceReport = true;
    }

    public void SetBreakState(bool isBreaking)
    {
        _isBreaking = isBreaking;
        _forceReport = true;
    }

    public void UpdateServerUrl(string url)
    {
        _apiClient = new ApiClient(url);
    }

    public void ResetBinding()
    {
        _tokenStore.ClearToken();
        _token = null;
    }

    public void ResetOptionalUpdateNotice()
    {
        _optionalUpdateNotified = "";
    }

    public async Task CheckUpdateAsync(CancellationToken ct)
    {
        if (string.IsNullOrWhiteSpace(_config.EmployeeCode))
        {
            return;
        }

        try
        {
            var fingerprint = FingerprintProvider.GetFingerprint(_logger);
            var response = await _apiClient.BindAsync(new ClientBindRequest
            {
                EmployeeCode = _config.EmployeeCode,
                Fingerprint = fingerprint,
                ClientVersion = AppConstants.ClientVersion
            }, ct).ConfigureAwait(false);

            _token = response.Token;
            _tokenStore.SaveToken(response.Token);
            ApplyServerSettings(response.IdleThresholdSeconds, response.HeartbeatIntervalSeconds, response.OfflineThresholdSeconds, response.UpdatePolicy, response.LatestVersion, response.UpdateUrl);
        }
        catch (Exception ex)
        {
            _logger.Warn($"启动更新检查失败: {ex.Message}");
        }
    }

    public async Task<bool> SendWorkStartAsync(CancellationToken ct)
    {
        var sample = Win32Interop.CaptureSample(_config.IdleThresholdSeconds);
        return await TrySendReportAsync(sample, "work_start", ct, false, null).ConfigureAwait(false);
    }

    public async Task<ReportResult> SendWorkEndAsync(ClientCheckoutPayload? checkout, string? reason, CancellationToken ct)
    {
        var sample = Win32Interop.CaptureSample(_config.IdleThresholdSeconds);
        return await TrySendReportWithErrorAsync(sample, "work_end", ct, false, checkout, reason).ConfigureAwait(false);
    }

    public async Task<CheckoutTemplateResponse?> GetCheckoutTemplateAsync(CancellationToken ct)
    {
        await EnsureBoundAsync(ct).ConfigureAwait(false);
        if (string.IsNullOrWhiteSpace(_token))
        {
            throw new UnauthorizedException("缺少令牌");
        }
        return await _apiClient.GetCheckoutTemplateAsync(_token!, ct).ConfigureAwait(false);
    }

    private async Task LoopAsync(CancellationToken ct)
    {
        var timer = new PeriodicTimer(TimeSpan.FromSeconds(5));
        while (await timer.WaitForNextTickAsync(ct).ConfigureAwait(false))
        {
            if (_isBreaking)
            {
                var breakSample = CreateBreakSample();
                var shouldHeartbeatOnBreak = DateTime.UtcNow - _lastHeartbeatAt >= TimeSpan.FromSeconds(_config.HeartbeatIntervalSeconds);
                if (_forceReport || shouldHeartbeatOnBreak)
                {
                    _forceReport = false;
                    await SafeSendAsync(breakSample, "break", ct).ConfigureAwait(false);
                    _lastHeartbeatAt = DateTime.UtcNow;
                }
                _lastSample = breakSample;
                continue;
            }

            var sample = Win32Interop.CaptureSample(_config.IdleThresholdSeconds);
            var shouldChange = _forceReport || IsChange(sample, _lastSample);
            var shouldHeartbeat = DateTime.UtcNow - _lastHeartbeatAt >= TimeSpan.FromSeconds(_config.HeartbeatIntervalSeconds);

            if (shouldChange)
            {
                _forceReport = false;
                await SafeSendAsync(sample, "change", ct).ConfigureAwait(false);
                _lastHeartbeatAt = DateTime.UtcNow;
            }
            else if (shouldHeartbeat)
            {
                await SafeSendAsync(sample, "heartbeat", ct).ConfigureAwait(false);
                _lastHeartbeatAt = DateTime.UtcNow;
            }

            _lastSample = sample;
        }
    }
    private static SampleState CreateBreakSample()
    {
        return new SampleState(string.Empty, string.Empty, 0, false);
    }
    private static bool IsChange(SampleState current, SampleState? previous)
    {
        if (previous == null)
        {
            return true;
        }

        if (!string.Equals(current.ProcessName, previous.ProcessName, StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }
        if (!string.Equals(current.WindowTitle, previous.WindowTitle, StringComparison.OrdinalIgnoreCase))
        {
            return true;
        }
        if (current.IsIdle != previous.IsIdle)
        {
            return true;
        }
        return false;
    }

    private async Task EnsureBoundAsync(CancellationToken ct)
    {
        if (!string.IsNullOrWhiteSpace(_token))
        {
            return;
        }

        if (string.IsNullOrWhiteSpace(_config.EmployeeCode))
        {
            throw new InvalidOperationException("工号不能为空");
        }

        var fingerprint = FingerprintProvider.GetFingerprint(_logger);
        var response = await _apiClient.BindAsync(new ClientBindRequest
        {
            EmployeeCode = _config.EmployeeCode,
            Fingerprint = fingerprint,
            ClientVersion = AppConstants.ClientVersion
        }, ct).ConfigureAwait(false);

        _token = response.Token;
        _tokenStore.SaveToken(response.Token);
        ApplyServerSettings(response.IdleThresholdSeconds, response.HeartbeatIntervalSeconds, response.OfflineThresholdSeconds, response.UpdatePolicy, response.LatestVersion, response.UpdateUrl);
        StatusChanged?.Invoke("已绑定");
    }

    private async Task SafeSendAsync(SampleState sample, string reportType, CancellationToken ct)
    {
        _ = await TrySendReportAsync(sample, reportType, ct, true, null).ConfigureAwait(false);
    }

    private async Task<bool> TrySendReportAsync(SampleState sample, string reportType, CancellationToken ct, bool notifyStatus, ClientCheckoutPayload? checkout)
    {
        if (!_backoff.CanSend)
        {
            return false;
        }

        try
        {
            await EnsureBoundAsync(ct).ConfigureAwait(false);
            var response = await SendReportAsync(sample, reportType, ct, checkout).ConfigureAwait(false);
            _backoff.RegisterSuccess();
            ApplyServerSettings(response.IdleThresholdSeconds, response.HeartbeatIntervalSeconds, response.OfflineThresholdSeconds, response.UpdatePolicy, response.LatestVersion, response.UpdateUrl);
            if (notifyStatus)
            {
                StatusChanged?.Invoke("已上报");
            }
            return true;
        }
        catch (UpgradeRequiredException ex)
        {
            _logger.Warn(ex.Message);
            TriggerForcedUpdate();
        }
        catch (UnauthorizedException ex)
        {
            _logger.Warn(ex.Message);
            _tokenStore.ClearToken();
            _token = null;
            _backoff.RegisterFailure();
        }
        catch (HttpRequestException ex)
        {
            _logger.Warn(ex.Message);
            _backoff.RegisterFailure();
            StatusChanged?.Invoke("网络异常");
        }
        catch (TaskCanceledException ex) when (!ct.IsCancellationRequested)
        {
            _logger.Warn($"请求超时: {ex.Message}");
            _backoff.RegisterFailure();
            StatusChanged?.Invoke("网络异常");
        }
        catch (NeedReasonException ex)
        {
            _logger.Warn(ex.Message);
            return false;
        }
        catch (ApiException ex)
        {
            _logger.Warn(ex.Message);
            _backoff.RegisterFailure();
            if (ex.StatusCode != System.Net.HttpStatusCode.BadRequest)
            {
                StatusChanged?.Invoke("网络异常");
            }
        }
        catch (Exception ex)
        {
            _logger.Error(ex.Message);
            _backoff.RegisterFailure();
        }
        return false;
    }

    private async Task<ReportResult> TrySendReportWithErrorAsync(SampleState sample, string reportType, CancellationToken ct, bool notifyStatus, ClientCheckoutPayload? checkout, string? reason)
    {
        if (!_backoff.CanSend)
        {
            return ReportResult.Fail(string.Empty);
        }

        try
        {
            await EnsureBoundAsync(ct).ConfigureAwait(false);
            var response = await SendReportAsync(sample, reportType, ct, checkout, reason).ConfigureAwait(false);
            _backoff.RegisterSuccess();
            ApplyServerSettings(response.IdleThresholdSeconds, response.HeartbeatIntervalSeconds, response.OfflineThresholdSeconds, response.UpdatePolicy, response.LatestVersion, response.UpdateUrl);
            if (notifyStatus)
            {
                StatusChanged?.Invoke("已上报");
            }
            return ReportResult.Ok();
        }
        catch (UpgradeRequiredException ex)
        {
            _logger.Warn(ex.Message);
            TriggerForcedUpdate();
            return ReportResult.Fail(ex.Message);
        }
        catch (UnauthorizedException ex)
        {
            _logger.Warn(ex.Message);
            _tokenStore.ClearToken();
            _token = null;
            _backoff.RegisterFailure();
            return ReportResult.Fail(ex.Message);
        }
        catch (HttpRequestException ex)
        {
            _logger.Warn(ex.Message);
            _backoff.RegisterFailure();
            StatusChanged?.Invoke("网络异常");
            return ReportResult.Fail(string.Empty);
        }
        catch (TaskCanceledException ex) when (!ct.IsCancellationRequested)
        {
            _logger.Warn($"请求超时: {ex.Message}");
            _backoff.RegisterFailure();
            StatusChanged?.Invoke("网络异常");
            return ReportResult.Fail(string.Empty);
        }
        catch (NeedReasonException ex)
        {
            _logger.Warn(ex.Message);
            return ReportResult.Fail(ex.Message, "need_reason", ex.Data);
        }
        catch (ApiException ex)
        {
            _logger.Warn(ex.Message);
            _backoff.RegisterFailure();
            if (ex.StatusCode != System.Net.HttpStatusCode.BadRequest)
            {
                StatusChanged?.Invoke("网络异常");
            }
            return ReportResult.Fail(ex.Message);
        }
        catch (Exception ex)
        {
            _logger.Error(ex.Message);
            _backoff.RegisterFailure();
            return ReportResult.Fail(string.Empty);
        }
    }

    private async Task<ClientReportResponse> SendReportAsync(SampleState sample, string reportType, CancellationToken ct, ClientCheckoutPayload? checkout, string? reason = null)
    {
        if (string.IsNullOrWhiteSpace(_token))
        {
            throw new UnauthorizedException("缺少令牌");
        }

        return await _apiClient.ReportAsync(new ClientReportRequest
        {
            ProcessName = sample.ProcessName,
            WindowTitle = sample.WindowTitle,
            IdleSeconds = sample.IdleSeconds,
            ClientVersion = AppConstants.ClientVersion,
            ReportType = reportType,
            Checkout = checkout,
            Reason = reason ?? string.Empty
        }, _token!, ct).ConfigureAwait(false);
    }

    private void ApplyServerSettings(int idleThreshold, int heartbeatInterval, int offlineThreshold, int updatePolicy, string latestVersion, string updateUrl)
    {
        _config.IdleThresholdSeconds = idleThreshold;
        _config.HeartbeatIntervalSeconds = heartbeatInterval;
        _config.OfflineThresholdSeconds = offlineThreshold;
        _config.UpdatePolicy = updatePolicy;
        _config.LatestVersion = latestVersion ?? string.Empty;
        _config.UpdateUrl = updateUrl ?? string.Empty;
        _config.LastConfigAt = DateTime.Now;
        _configStore.Save(_config);
        SettingsChanged?.Invoke(_config);

        if (VersionHelper.IsOutdated(AppConstants.ClientVersion, _config.LatestVersion))
        {
            if (_config.UpdatePolicy == 1)
            {
                TriggerForcedUpdate();
            }
            else if (_config.UpdatePolicy == 0 && _optionalUpdateNotified != _config.LatestVersion)
            {
                _optionalUpdateNotified = _config.LatestVersion;
                OptionalUpdate?.Invoke(_config.LatestVersion, _config.UpdateUrl);
            }
        }
    }

    private void TriggerForcedUpdate()
    {
        ForcedUpdate?.Invoke(_config.LatestVersion, _config.UpdateUrl);
        Stop();
    }
}

