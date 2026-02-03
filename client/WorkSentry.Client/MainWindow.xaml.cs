using System;
using System.Windows;
using MediaBrush = System.Windows.Media.Brush;
using MediaColor = System.Windows.Media.Color;
using MediaSolidColorBrush = System.Windows.Media.SolidColorBrush;

namespace WorkSentry.Client;

internal sealed partial class MainWindow : Window
{
    private static readonly MediaBrush WorkingForeground = new MediaSolidColorBrush(MediaColor.FromRgb(22, 163, 74));
    private static readonly MediaBrush WorkingCardBackground = new MediaSolidColorBrush(MediaColor.FromRgb(236, 253, 245));
    private static readonly MediaBrush WorkingCardBorder = new MediaSolidColorBrush(MediaColor.FromRgb(187, 247, 208));
    private static readonly MediaBrush WorkingIcon = new MediaSolidColorBrush(MediaColor.FromRgb(187, 247, 208));
    private static readonly MediaBrush IdleForeground = new MediaSolidColorBrush(MediaColor.FromRgb(31, 41, 55));
    private static readonly MediaBrush IdleCardBackground = new MediaSolidColorBrush(MediaColor.FromRgb(239, 246, 255));
    private static readonly MediaBrush IdleCardBorder = new MediaSolidColorBrush(MediaColor.FromRgb(219, 234, 254));
    private static readonly MediaBrush IdleIcon = new MediaSolidColorBrush(MediaColor.FromRgb(191, 219, 254));
    private static readonly MediaBrush NetworkForeground = new MediaSolidColorBrush(MediaColor.FromRgb(234, 88, 12));
    private static readonly MediaBrush NetworkCardBackground = new MediaSolidColorBrush(MediaColor.FromRgb(255, 247, 237));
    private static readonly MediaBrush NetworkCardBorder = new MediaSolidColorBrush(MediaColor.FromRgb(254, 215, 170));
    private static readonly MediaBrush NetworkIcon = new MediaSolidColorBrush(MediaColor.FromRgb(253, 186, 116));
    private static readonly MediaBrush OffForeground = new MediaSolidColorBrush(MediaColor.FromRgb(107, 114, 128));
    private static readonly MediaBrush OffCardBackground = new MediaSolidColorBrush(MediaColor.FromRgb(249, 250, 251));
    private static readonly MediaBrush OffCardBorder = new MediaSolidColorBrush(MediaColor.FromRgb(229, 231, 235));
    private static readonly MediaBrush OffIcon = new MediaSolidColorBrush(MediaColor.FromRgb(226, 232, 240));
    private static readonly MediaBrush BreakForeground = new MediaSolidColorBrush(MediaColor.FromRgb(217, 119, 6));
    private static readonly MediaBrush BreakCardBackground = new MediaSolidColorBrush(MediaColor.FromRgb(255, 251, 235));
    private static readonly MediaBrush BreakCardBorder = new MediaSolidColorBrush(MediaColor.FromRgb(253, 230, 138));
    private static readonly MediaBrush BreakIcon = new MediaSolidColorBrush(MediaColor.FromRgb(252, 211, 77));
    private static readonly MediaBrush PolicyHintBackground = new MediaSolidColorBrush(MediaColor.FromRgb(243, 244, 246));
    private static readonly MediaBrush PolicyHintForeground = new MediaSolidColorBrush(MediaColor.FromRgb(55, 65, 81));
    private static readonly MediaBrush PolicyForceBackground = new MediaSolidColorBrush(MediaColor.FromRgb(254, 226, 226));
    private static readonly MediaBrush PolicyForceForeground = new MediaSolidColorBrush(MediaColor.FromRgb(185, 28, 28));

    private string _currentStatusToken = "待上班";
    private bool _isWorking;
    private bool _isBreaking;
    private DateTime? _lastReportTime;
    private int _lastUpdatePolicy;
    private string _lastLatestVersion = string.Empty;
    private bool _languageInitializing;
    private bool _showingPrompt;
    private bool _updatePromptForced;
    private string? _updatePromptVersion;
    private string? _updatePromptMessageOverride;
    private bool _updateProgressVisible;
    private string? _updateProgressStage;
    private long _updateProgressReceived;
    private long? _updateProgressTotal;

    public event Action<string>? SaveConfigRequested;
    public event Action? StartRequested;
    public event Action? StopRequested;
    public event Action? BreakToggleRequested;
    public event Action? ExitRequested;
    public event Action? UpdateNowRequested;
    public event Action? UpdateLaterRequested;
    public event Action<string>? LanguageChangedRequested;

    public MainWindow()
    {
        InitializeComponent();
        VersionBadgeText.Text = $"v{AppConstants.ClientVersion}";
        VersionText.Text = AppConstants.ClientVersion;
        SaveButton.Click += (_, _) => SaveConfig();
        StartButton.Click += (_, _) => StartRequested?.Invoke();
        StopButton.Click += (_, _) => StopRequested?.Invoke();
        BreakButton.Click += (_, _) => BreakToggleRequested?.Invoke();
        ExitButton.Click += (_, _) => ExitRequested?.Invoke();
        UpdateNowButton.Click += (_, _) => UpdateNowRequested?.Invoke();
        UpdateLaterButton.Click += (_, _) => UpdateLaterRequested?.Invoke();
        LanguageComboBox.SelectionChanged += (_, _) => HandleLanguageSelection();
    }

    private void HandleLanguageSelection()
    {
        if (_languageInitializing)
        {
            return;
        }
        var selected = LanguageComboBox.SelectedValue?.ToString() ?? LanguageService.Auto;
        LanguageChangedRequested?.Invoke(selected);
    }

    private void SaveConfig()
    {
        var code = EmployeeCodeBox.Text.Trim();
        if (string.IsNullOrWhiteSpace(code))
        {
            System.Windows.MessageBox.Show(LanguageService.GetString("MsgEmployeeCodeEmpty"), LanguageService.GetString("DialogTitleTip"), MessageBoxButton.OK, MessageBoxImage.Information);
            EmployeeCodeBox.Focus();
            return;
        }
        SaveConfigRequested?.Invoke(code);
    }

    internal void LoadConfig(AppConfig config)
    {
        EmployeeCodeBox.Text = config.EmployeeCode;
        SetLanguageSelection(config.LanguageOverride);
        UpdateUpdateInfo(config.UpdatePolicy, config.LatestVersion);
        UpdateBreakButton();
    }

    private void SetLanguageSelection(string? overrideValue)
    {
        _languageInitializing = true;
        LanguageComboBox.SelectedValue = NormalizeLanguageOption(overrideValue);
        _languageInitializing = false;
    }

    private static string NormalizeLanguageOption(string? overrideValue)
    {
        if (string.IsNullOrWhiteSpace(overrideValue) || string.Equals(overrideValue, LanguageService.Auto, StringComparison.OrdinalIgnoreCase))
        {
            return LanguageService.Auto;
        }
        if (overrideValue.StartsWith("vi", StringComparison.OrdinalIgnoreCase))
        {
            return LanguageService.ViVn;
        }
        if (overrideValue.StartsWith("en", StringComparison.OrdinalIgnoreCase))
        {
            return LanguageService.EnUs;
        }
        if (overrideValue.StartsWith("zh", StringComparison.OrdinalIgnoreCase))
        {
            return LanguageService.ZhCn;
        }
        return LanguageService.Auto;
    }

    internal void RefreshLanguage()
    {
        UpdateStatus(_currentStatusToken);
        UpdateUpdateInfo(_lastUpdatePolicy, _lastLatestVersion);
        if (_lastReportTime.HasValue)
        {
            UpdateLastReport(_lastReportTime);
        }
        if (_showingPrompt)
        {
            ShowUpdatePrompt(_updatePromptForced, _updatePromptVersion, _updatePromptMessageOverride);
        }
        else if (_updateProgressVisible && !string.IsNullOrWhiteSpace(_updateProgressStage))
        {
            UpdateDownloadProgress(_updateProgressStage, _updateProgressReceived, _updateProgressTotal);
        }
    }

    internal void SetWorkingState(bool working)
    {
        _isWorking = working;
        if (!working)
        {
            _isBreaking = false;
        }
        StartButton.IsEnabled = !working;
        StartButton.Visibility = working ? Visibility.Collapsed : Visibility.Visible;
        StopButton.IsEnabled = working;
        StopButton.Visibility = working ? Visibility.Visible : Visibility.Collapsed;
        SaveButton.IsEnabled = !working;
        EmployeeCodeBox.IsEnabled = !working;
        UpdateBreakButton();
        UpdateStatus(working ? "已上班" : "待上班");
        if (!working)
        {
            _lastReportTime = null;
            LastReportText.Text = LanguageService.GetString("LastReportPlaceholder");
        }
    }


    internal void SetBreakState(bool isBreaking)
    {
        _isBreaking = isBreaking;
        UpdateBreakButton();
    }

    private void UpdateBreakButton()
    {
        BreakButton.Content = LanguageService.GetString(_isBreaking ? "BreakStopButton" : "BreakStartButton");
        BreakButton.IsEnabled = _isWorking;
        BreakButton.Visibility = _isWorking ? Visibility.Visible : Visibility.Collapsed;
    }
    internal void UpdateStatus(string status)
    {
        _currentStatusToken = status;
        StatusValueText.Text = LanguageService.GetStatusDisplay(status);
        var (fg, cardBg, cardBorder, icon) = status switch
        {
            "已上班" => (WorkingForeground, WorkingCardBackground, WorkingCardBorder, WorkingIcon),
            "休息中" => (BreakForeground, BreakCardBackground, BreakCardBorder, BreakIcon),
            "网络异常" => (NetworkForeground, NetworkCardBackground, NetworkCardBorder, NetworkIcon),
            "已下班" => (OffForeground, OffCardBackground, OffCardBorder, OffIcon),
            "连接中" => (IdleForeground, IdleCardBackground, IdleCardBorder, IdleIcon),
            "需要更新" => (NetworkForeground, NetworkCardBackground, NetworkCardBorder, NetworkIcon),
            _ => (IdleForeground, IdleCardBackground, IdleCardBorder, IdleIcon)
        };
        StatusValueText.Foreground = fg;
        StatusCard.Background = cardBg;
        StatusCard.BorderBrush = cardBorder;
        StatusIconText.Foreground = icon;
        if (status == "连接中")
        {
            LastReportText.Text = LanguageService.GetString("LastReportConnecting");
        }
        else if (status == "网络异常")
        {
            LastReportText.Text = LanguageService.GetString("LastReportNetwork");
        }
        else if (status == "已上班" && !_lastReportTime.HasValue)
        {
            LastReportText.Text = LanguageService.GetString("LastReportMonitoring");
        }
    }

    internal void UpdateLastReport(DateTime? time)
    {
        _lastReportTime = time;
        if (time.HasValue)
        {
            LastReportText.Text = LanguageService.Format("LastReportFormat", time.Value.ToString("HH:mm:ss"));
        }
    }

    internal void UpdateUpdateInfo(int updatePolicy, string latestVersion)
    {
        _lastUpdatePolicy = updatePolicy;
        _lastLatestVersion = latestVersion;
        LatestVersionText.Text = string.IsNullOrWhiteSpace(latestVersion) ? "-" : latestVersion;
        if (updatePolicy == 1)
        {
            UpdatePolicyText.Text = LanguageService.GetString("UpdatePolicyForce");
            UpdatePolicyBadge.Background = PolicyForceBackground;
            UpdatePolicyText.Foreground = PolicyForceForeground;
        }
        else
        {
            UpdatePolicyText.Text = LanguageService.GetString("UpdatePolicyHint");
            UpdatePolicyBadge.Background = PolicyHintBackground;
            UpdatePolicyText.Foreground = PolicyHintForeground;
        }
    }

    internal void ShowUpdatePrompt(bool forced, string? version, string? messageOverride = null)
    {
        _showingPrompt = true;
        _updatePromptForced = forced;
        _updatePromptVersion = version ?? string.Empty;
        _updatePromptMessageOverride = messageOverride;
        var versionText = string.IsNullOrWhiteSpace(version) ? string.Empty : $" {version}";
        var message = messageOverride ?? LanguageService.Format(forced ? "UpdateMessageForced" : "UpdateMessageOptional", versionText);
        ResetUpdateProgress();
        UpdateTitleText.Text = LanguageService.GetString(forced ? "UpdateTitleForced" : "UpdateTitleOptional");
        UpdateMessageText.Text = message;
        UpdateLaterButton.Visibility = forced ? Visibility.Collapsed : Visibility.Visible;
        UpdateLaterButton.IsEnabled = !forced;
        UpdateNowButton.Content = LanguageService.GetString(forced ? "UpdateNowForcedButton" : "UpdateNowButton");
        UpdateNowButton.IsEnabled = true;
        UpdateOverlay.Visibility = Visibility.Visible;
    }

    internal void SetUpdateProgress(string message)
    {
        _showingPrompt = false;
        _updateProgressVisible = false;
        _updateProgressStage = null;
        UpdateMessageText.Text = message;
        UpdateNowButton.IsEnabled = false;
        UpdateLaterButton.IsEnabled = false;
        UpdateOverlay.Visibility = Visibility.Visible;
    }

    internal void UpdateDownloadProgress(string stage, long receivedBytes, long? totalBytes)
    {
        _showingPrompt = false;
        _updateProgressVisible = true;
        _updateProgressStage = stage;
        _updateProgressReceived = receivedBytes;
        _updateProgressTotal = totalBytes;
        UpdateProgressBar.Visibility = Visibility.Visible;
        UpdateProgressText.Visibility = Visibility.Visible;
        var displayStage = LanguageService.TranslateUpdateStage(stage);
        if (totalBytes.HasValue && totalBytes.Value > 0)
        {
            UpdateProgressBar.IsIndeterminate = false;
            var percent = Math.Min(100, Math.Round(receivedBytes * 100d / totalBytes.Value, 1));
            UpdateProgressBar.Value = percent;
            UpdateProgressText.Text = LanguageService.Format("UpdateProgressDetail", displayStage, FormatBytes(receivedBytes), FormatBytes(totalBytes.Value), percent);
        }
        else
        {
            UpdateProgressBar.IsIndeterminate = true;
            UpdateProgressText.Text = LanguageService.Format("UpdateProgressDetailNoTotal", displayStage, FormatBytes(receivedBytes));
        }
    }

    private void ResetUpdateProgress()
    {
        UpdateProgressBar.Visibility = Visibility.Collapsed;
        UpdateProgressText.Visibility = Visibility.Collapsed;
        UpdateProgressBar.IsIndeterminate = false;
        UpdateProgressBar.Value = 0;
        UpdateProgressText.Text = string.Empty;
        _updateProgressVisible = false;
        _updateProgressStage = null;
        _updateProgressReceived = 0;
        _updateProgressTotal = null;
    }

    private static string FormatBytes(long bytes)
    {
        const double kb = 1024d;
        const double mb = kb * 1024d;
        const double gb = mb * 1024d;
        if (bytes >= gb)
        {
            return $"{bytes / gb:0.0} GB";
        }
        if (bytes >= mb)
        {
            return $"{bytes / mb:0.0} MB";
        }
        if (bytes >= kb)
        {
            return $"{bytes / kb:0.0} KB";
        }
        return $"{bytes} B";
    }

    internal void HideUpdatePrompt()
    {
        _showingPrompt = false;
        UpdateOverlay.Visibility = Visibility.Collapsed;
    }

    internal void FocusEmployeeCode()
    {
        EmployeeCodeBox.Focus();
    }
}
