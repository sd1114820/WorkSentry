using System;
using System.Threading;
using System.Threading.Tasks;

namespace WorkSentry.Client;

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

    private async Task LoopAsync(CancellationToken ct)
    {
        var timer = new PeriodicTimer(TimeSpan.FromSeconds(5));
        while (await timer.WaitForNextTickAsync(ct).ConfigureAwait(false))
        {
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
        if (!_backoff.CanSend)
        {
            return;
        }

        try
        {
            await EnsureBoundAsync(ct).ConfigureAwait(false);
            var response = await SendReportAsync(sample, reportType, ct).ConfigureAwait(false);
            _backoff.RegisterSuccess();
            ApplyServerSettings(response.IdleThresholdSeconds, response.HeartbeatIntervalSeconds, response.OfflineThresholdSeconds, response.UpdatePolicy, response.LatestVersion, response.UpdateUrl);
            StatusChanged?.Invoke("已上报");
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
        catch (ApiException ex)
        {
            _logger.Warn(ex.Message);
            _backoff.RegisterFailure();
            StatusChanged?.Invoke("网络异常");
        }
        catch (Exception ex)
        {
            _logger.Error(ex.Message);
            _backoff.RegisterFailure();
        }
    }

    private async Task<ClientReportResponse> SendReportAsync(SampleState sample, string reportType, CancellationToken ct)
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
            ReportType = reportType
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

