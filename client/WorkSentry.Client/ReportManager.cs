using System;
using System.Collections.Generic;
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
    public bool FocusEmployeeCode { get; }

    private ReportResult(bool success, string? error, string? errorCode, JsonElement? errorData, bool focusEmployeeCode)
    {
        Success = success;
        Error = error;
        ErrorCode = errorCode;
        ErrorData = errorData;
        FocusEmployeeCode = focusEmployeeCode;
    }

    public static ReportResult Ok()
    {
        return new ReportResult(true, null, null, null, false);
    }

    public static ReportResult Fail(string? error, string? code = null, JsonElement? data = null, bool focusEmployeeCode = false)
    {
        return new ReportResult(false, error, code, data, focusEmployeeCode);
    }
}

internal sealed class OperationResult<T>
{
    public bool Success { get; }
    public T? Value { get; }
    public string? Error { get; }
    public string? ErrorCode { get; }
    public JsonElement? ErrorData { get; }
    public bool FocusEmployeeCode { get; }

    private OperationResult(bool success, T? value, string? error, string? errorCode, JsonElement? errorData, bool focusEmployeeCode)
    {
        Success = success;
        Value = value;
        Error = error;
        ErrorCode = errorCode;
        ErrorData = errorData;
        FocusEmployeeCode = focusEmployeeCode;
    }

    public static OperationResult<T> Ok(T? value)
    {
        return new OperationResult<T>(true, value, null, null, null, false);
    }

    public static OperationResult<T> Fail(string? error, string? code = null, JsonElement? data = null, bool focusEmployeeCode = false)
    {
        return new OperationResult<T>(false, default, error, code, data, focusEmployeeCode);
    }
}

internal sealed class ReportManager
{
    private const int MaxPendingClientEventIds = 256;

    private readonly AppConfig _config;
    private readonly ConfigStore _configStore;
    private readonly TokenStore _tokenStore;
    private readonly Logger _logger;
    private readonly ClientErrorQueue _errorQueue;
    private ApiClient _apiClient;
    private readonly NetworkBackoff _backoff = new();
    private CancellationTokenSource? _cts;
    private SampleState? _lastSample;
    private DateTime _lastHeartbeatAt = DateTime.MinValue;
    private DateTime _lastSuccessfulReportAtUtc = DateTime.MinValue;
    private DateTime _lastErrorFlushAtUtc = DateTime.MinValue;
    private DateTime _lastConnectivityErrorEnqueuedAtUtc = DateTime.MinValue;
    private string? _token;
    private string _optionalUpdateNotified = "";
    private readonly Dictionary<string, string> _pendingClientEventIds = new(StringComparer.Ordinal);
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
        _errorQueue = new ClientErrorQueue(_configStore.BaseDirectory);
        _apiClient = new ApiClient(_config.ServerUrl);
    }

    public async Task StartAsync()
    {
        _token = _tokenStore.LoadToken();
        await EnsureBoundAsync(CancellationToken.None).ConfigureAwait(false);
        await FlushErrorQueueSafeAsync(CancellationToken.None).ConfigureAwait(false);

        var startupSample = Win32Interop.CaptureSample(_config.IdleThresholdSeconds);
        _ = await SafeSendAsync(startupSample, "startup", CancellationToken.None).ConfigureAwait(false);

        _cts = new CancellationTokenSource();
        _ = Task.Run(async () =>
        {
            try
            {
                await LoopAsync(_cts.Token).ConfigureAwait(false);
            }
            catch (OperationCanceledException) when (_cts.Token.IsCancellationRequested)
            {
                // ignore
            }
            catch (Exception ex)
            {
                EnqueueClientError("report_loop_crashed", ex, null);
                StatusChanged?.Invoke("上报异常");
            }
        });
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

    public async Task<ReportResult> SendWorkStartAsync(CancellationToken ct)
    {
        var sample = Win32Interop.CaptureSample(_config.IdleThresholdSeconds);
        return await TrySendReportAsync(sample, "work_start", ct, false, null, null).ConfigureAwait(false);
    }

    public async Task<ReportResult> SendWorkEndAsync(ClientCheckoutPayload? checkout, string? reason, CancellationToken ct)
    {
        var sample = Win32Interop.CaptureSample(_config.IdleThresholdSeconds);
        return await TrySendReportAsync(sample, "work_end", ct, false, checkout, reason).ConfigureAwait(false);
    }

    public async Task<OperationResult<CheckoutTemplateResponse?>> GetCheckoutTemplateAsync(CancellationToken ct)
    {
        try
        {
            await EnsureBoundAsync(ct).ConfigureAwait(false);
            if (string.IsNullOrWhiteSpace(_token))
            {
                throw new UnauthorizedException("缺少令牌", "token_missing");
            }

            var response = await _apiClient.GetCheckoutTemplateAsync(_token!, ct).ConfigureAwait(false);
            return OperationResult<CheckoutTemplateResponse?>.Ok(response);
        }
        catch (TaskCanceledException) when (ct.IsCancellationRequested)
        {
            return OperationResult<CheckoutTemplateResponse?>.Fail(null);
        }
        catch (Exception ex)
        {
            var fail = HandleReportException(ex, false);
            return OperationResult<CheckoutTemplateResponse?>.Fail(fail.Error, fail.ErrorCode, fail.ErrorData, fail.FocusEmployeeCode);
        }
    }
    private async Task LoopAsync(CancellationToken ct)
    {
        var timer = new PeriodicTimer(TimeSpan.FromSeconds(5));
        while (await timer.WaitForNextTickAsync(ct).ConfigureAwait(false))
        {
            try
            {
                if (_isBreaking)
                {
                    var breakSample = CreateBreakSample();
                    var shouldHeartbeatOnBreak = DateTime.UtcNow - _lastHeartbeatAt >= TimeSpan.FromSeconds(_config.HeartbeatIntervalSeconds);
                    if (_forceReport || shouldHeartbeatOnBreak)
                    {
                        var result = await SafeSendAsync(breakSample, "break", ct).ConfigureAwait(false);
                        if (result.Success)
                        {
                            _lastHeartbeatAt = DateTime.UtcNow;
                            _forceReport = false;
                        }
                    }
                    _lastSample = breakSample;
                    continue;
                }

                SampleState sample;
                try
                {
                    sample = Win32Interop.CaptureSample(_config.IdleThresholdSeconds);
                }
                catch (Exception ex)
                {
                    EnqueueClientError("capture_sample_failed", ex, null);
                    _logger.Warn($"采样失败: {ex.Message}");
                    sample = _lastSample ?? CreateBreakSample();
                }

                var shouldChange = _forceReport || IsChange(sample, _lastSample);
                var shouldHeartbeat = DateTime.UtcNow - _lastHeartbeatAt >= TimeSpan.FromSeconds(_config.HeartbeatIntervalSeconds);

                if (shouldChange)
                {
                    var forced = _forceReport;
                    var result = await SafeSendAsync(sample, "change", ct).ConfigureAwait(false);
                    if (result.Success)
                    {
                        _lastHeartbeatAt = DateTime.UtcNow;
                        _forceReport = false;
                        _lastSample = sample;
                    }
                    else
                    {
                        if (forced)
                        {
                            _forceReport = true;
                        }
                    }
                }
                else if (shouldHeartbeat)
                {
                    var result = await SafeSendAsync(sample, "heartbeat", ct).ConfigureAwait(false);
                    if (result.Success)
                    {
                        _lastHeartbeatAt = DateTime.UtcNow;
                    }
                    _lastSample = sample;
                }
                else
                {
                    _lastSample = sample;
                }
            }
            catch (OperationCanceledException) when (ct.IsCancellationRequested)
            {
                break;
            }
            catch (Exception ex)
            {
                EnqueueClientError("loop_tick_exception", ex, null);
                _logger.Error($"上报循环异常: {ex.Message}");
            }
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

    private async Task<ReportResult> SafeSendAsync(SampleState sample, string reportType, CancellationToken ct)
    {
        return await TrySendReportAsync(sample, reportType, ct, true, null, null).ConfigureAwait(false);
    }

    private async Task<ReportResult> TrySendReportAsync(SampleState sample, string reportType, CancellationToken ct, bool notifyStatus, ClientCheckoutPayload? checkout, string? reason)
    {
        if (!_backoff.CanSend)
        {
            StatusChanged?.Invoke("网络异常");
            return ReportResult.Fail(LanguageService.GetString("ErrRetryLater"), "backoff_blocked");
        }

        var reportKey = BuildReportEventKey(sample, reportType, checkout, reason);
        try
        {
            await EnsureBoundAsync(ct).ConfigureAwait(false);
            var clientEventId = GetOrCreateClientEventId(reportKey);
            var response = await SendReportAsync(sample, reportType, clientEventId, ct, checkout, reason).ConfigureAwait(false);
            ClearClientEventId(reportKey);
            _backoff.RegisterSuccess();
            _lastSuccessfulReportAtUtc = DateTime.UtcNow;
            if (DateTime.UtcNow - _lastErrorFlushAtUtc >= TimeSpan.FromMinutes(1))
            {
                _ = FlushErrorQueueSafeAsync(ct);
            }
            ApplyServerSettings(response.IdleThresholdSeconds, response.HeartbeatIntervalSeconds, response.OfflineThresholdSeconds, response.UpdatePolicy, response.LatestVersion, response.UpdateUrl);
            if (notifyStatus)
            {
                StatusChanged?.Invoke("已上报");
            }
            return ReportResult.Ok();
        }
        catch (TaskCanceledException) when (ct.IsCancellationRequested)
        {
            return ReportResult.Fail(null);
        }
        catch (NeedReasonException ex)
        {
            ClearClientEventId(reportKey);
            _logger.Warn(ex.Message);
            return ReportResult.Fail(ex.Message, "need_reason", ex.Data);
        }
        catch (Exception ex)
        {
            if (ex is ApiException apiException && (int)apiException.StatusCode < 500)
            {
                ClearClientEventId(reportKey);
            }
            if (ex is ApiException || ex is HttpRequestException || ex is TaskCanceledException)
            {
                var lastSuccess = _lastSuccessfulReportAtUtc == DateTime.MinValue
                    ? DateTime.UtcNow.AddYears(-1)
                    : _lastSuccessfulReportAtUtc;
                var thresholdSeconds = Math.Max(120, _config.OfflineThresholdSeconds);
                if (DateTime.UtcNow - lastSuccess >= TimeSpan.FromSeconds(thresholdSeconds) &&
                    DateTime.UtcNow - _lastConnectivityErrorEnqueuedAtUtc >= TimeSpan.FromMinutes(10))
                {
                    EnqueueClientError("report_connectivity_issue", ex, reportType);
                    _lastConnectivityErrorEnqueuedAtUtc = DateTime.UtcNow;
                }
            }
            else
            {
                EnqueueClientError("report_exception", ex, reportType);
            }

            return HandleReportException(ex, true);
        }
    }

    private ReportResult HandleReportException(Exception ex, bool registerBackoff)
    {
        if (ex is ApiException or HttpRequestException or TaskCanceledException or InvalidOperationException)
        {
            _logger.Warn(ex.Message);
        }
        else
        {
            _logger.Error(ex.Message);
        }

        var mapping = ClientErrorMapper.Map(ex, _configStore.ConfigPath);

        if (mapping.ShouldClearToken)
        {
            _tokenStore.ClearToken();
            _token = null;
        }

        if (registerBackoff)
        {
            _backoff.RegisterFailure();
        }

        if (!string.IsNullOrWhiteSpace(mapping.StatusToken))
        {
            StatusChanged?.Invoke(mapping.StatusToken);
        }

        if (mapping.ShouldTriggerForcedUpdate)
        {
            TriggerForcedUpdate();
        }

        return ReportResult.Fail(mapping.Message, mapping.ErrorCode, mapping.ErrorData, mapping.ShouldFocusEmployeeCode);
    }

    private async Task<ClientReportResponse> SendReportAsync(SampleState sample, string reportType, string clientEventId, CancellationToken ct, ClientCheckoutPayload? checkout, string? reason = null)
    {
        if (string.IsNullOrWhiteSpace(_token))
        {
            throw new UnauthorizedException("缺少令牌", "token_missing");
        }

        return await _apiClient.ReportAsync(new ClientReportRequest
        {
            ProcessName = sample.ProcessName,
            WindowTitle = sample.WindowTitle,
            IdleSeconds = sample.IdleSeconds,
            ClientVersion = AppConstants.ClientVersion,
            ReportType = reportType,
            ClientEventId = clientEventId,
            Checkout = checkout,
            Reason = reason ?? string.Empty
        }, _token!, ct).ConfigureAwait(false);
    }

    private string GetOrCreateClientEventId(string reportKey)
    {
        if (_pendingClientEventIds.TryGetValue(reportKey, out var existing) && !string.IsNullOrWhiteSpace(existing))
        {
            return existing;
        }

        var created = Guid.NewGuid().ToString("N");
        if (_pendingClientEventIds.Count >= MaxPendingClientEventIds)
        {
            _pendingClientEventIds.Clear();
        }
        _pendingClientEventIds[reportKey] = created;
        return created;
    }

    private void ClearClientEventId(string reportKey)
    {
        _pendingClientEventIds.Remove(reportKey);
    }

    private static string BuildReportEventKey(SampleState sample, string reportType, ClientCheckoutPayload? checkout, string? reason)
    {
        var checkoutKey = checkout == null
            ? ""
            : JsonSerializer.Serialize(checkout);
        return string.Join('\u001f',
            reportType,
            sample.ProcessName,
            sample.WindowTitle,
            sample.IdleSeconds.ToString(),
            checkoutKey,
            reason ?? string.Empty);
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


    private async Task FlushErrorQueueSafeAsync(CancellationToken ct)
    {
        if (string.IsNullOrWhiteSpace(_token))
        {
            return;
        }

        try
        {
            await _errorQueue.FlushAsync(_apiClient, _token!, _logger, ct).ConfigureAwait(false);
            _lastErrorFlushAtUtc = DateTime.UtcNow;
        }
        catch (Exception ex)
        {
            _logger.Warn($"刷新错误上报队列失败: {ex.Message}");
        }
    }

    private void EnqueueClientError(string errorType, Exception ex, string? reportType)
    {
        try
        {
            var context = new Dictionary<string, string>
            {
                ["serverUrl"] = _config.ServerUrl ?? string.Empty,
                ["employeeCode"] = _config.EmployeeCode ?? string.Empty,
                ["reportType"] = reportType ?? string.Empty,
                ["lastSuccessfulReportAtUtc"] = _lastSuccessfulReportAtUtc == DateTime.MinValue ? string.Empty : _lastSuccessfulReportAtUtc.ToString("O"),
            };

            if (_lastSample != null)
            {
                context["lastProcessName"] = _lastSample.ProcessName ?? string.Empty;
                context["lastWindowTitle"] = _lastSample.WindowTitle ?? string.Empty;
                context["lastIdleSeconds"] = _lastSample.IdleSeconds.ToString();
            }

            _errorQueue.Enqueue(new ClientErrorReportRequest
            {
                OccurredAt = DateTime.UtcNow.ToString("O"),
                ErrorType = errorType ?? string.Empty,
                ExceptionType = ex.GetType().FullName ?? string.Empty,
                Message = ex.Message ?? string.Empty,
                StackTrace = ex.ToString(),
                ClientVersion = AppConstants.ClientVersion,
                Context = context
            });
        }
        catch
        {
            // ignore
        }
    }    private void TriggerForcedUpdate()
    {
        ForcedUpdate?.Invoke(_config.LatestVersion, _config.UpdateUrl);
        Stop();
    }
}








