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

    private static readonly MediaBrush PolicyHintBackground = new MediaSolidColorBrush(MediaColor.FromRgb(243, 244, 246));
    private static readonly MediaBrush PolicyHintForeground = new MediaSolidColorBrush(MediaColor.FromRgb(55, 65, 81));
    private static readonly MediaBrush PolicyForceBackground = new MediaSolidColorBrush(MediaColor.FromRgb(254, 226, 226));
    private static readonly MediaBrush PolicyForceForeground = new MediaSolidColorBrush(MediaColor.FromRgb(185, 28, 28));

    public event Action<string>? SaveConfigRequested;
    public event Action? StartRequested;
    public event Action? StopRequested;
    public event Action? ExitRequested;
    public event Action? UpdateNowRequested;
    public event Action? UpdateLaterRequested;

    public MainWindow()
    {
        InitializeComponent();
        VersionBadgeText.Text = $"v{AppConstants.ClientVersion}";
        VersionText.Text = AppConstants.ClientVersion;

        SaveButton.Click += (_, _) => SaveConfig();
        StartButton.Click += (_, _) => StartRequested?.Invoke();
        StopButton.Click += (_, _) => StopRequested?.Invoke();
        ExitButton.Click += (_, _) => ExitRequested?.Invoke();
        UpdateNowButton.Click += (_, _) => UpdateNowRequested?.Invoke();
        UpdateLaterButton.Click += (_, _) => UpdateLaterRequested?.Invoke();
    }

    private void SaveConfig()
    {
        var code = EmployeeCodeBox.Text.Trim();
        if (string.IsNullOrWhiteSpace(code))
        {
            System.Windows.MessageBox.Show("工号不能为空", "提示", MessageBoxButton.OK, MessageBoxImage.Information);
            EmployeeCodeBox.Focus();
            return;
        }
        SaveConfigRequested?.Invoke(code);
    }

    internal void LoadConfig(AppConfig config)
    {
        EmployeeCodeBox.Text = config.EmployeeCode;
        UpdateUpdateInfo(config.UpdatePolicy, config.LatestVersion);
    }

    internal void SetWorkingState(bool working)
    {
        StartButton.IsEnabled = !working;
        StartButton.Visibility = working ? Visibility.Collapsed : Visibility.Visible;
        StopButton.IsEnabled = working;
        StopButton.Visibility = working ? Visibility.Visible : Visibility.Collapsed;
        SaveButton.IsEnabled = !working;
        EmployeeCodeBox.IsEnabled = !working;
        UpdateStatus(working ? "已上班" : "待上班");
        if (!working)
        {
            LastReportText.Text = "-";
        }
    }

    internal void UpdateStatus(string status)
    {
        StatusValueText.Text = status;
        var (fg, cardBg, cardBorder, icon) = status switch
        {
            "已上班" => (WorkingForeground, WorkingCardBackground, WorkingCardBorder, WorkingIcon),
            "网络异常" => (NetworkForeground, NetworkCardBackground, NetworkCardBorder, NetworkIcon),
            "已下班" => (OffForeground, OffCardBackground, OffCardBorder, OffIcon),
            _ => (IdleForeground, IdleCardBackground, IdleCardBorder, IdleIcon)
        };

        StatusValueText.Foreground = fg;
        StatusCard.Background = cardBg;
        StatusCard.BorderBrush = cardBorder;
        StatusIconText.Foreground = icon;

        if (status == "已上班" && LastReportText.Text == "-")
        {
            LastReportText.Text = "正在监控中...";
        }
    }

    internal void UpdateLastReport(DateTime? time)
    {
        if (time.HasValue)
        {
            LastReportText.Text = $"最后上报: {time.Value:HH:mm:ss}";
        }
    }

    internal void UpdateUpdateInfo(int updatePolicy, string latestVersion)
    {
        LatestVersionText.Text = string.IsNullOrWhiteSpace(latestVersion) ? "-" : latestVersion;
        if (updatePolicy == 1)
        {
            UpdatePolicyText.Text = "强制更新";
            UpdatePolicyBadge.Background = PolicyForceBackground;
            UpdatePolicyText.Foreground = PolicyForceForeground;
        }
        else
        {
            UpdatePolicyText.Text = "提示更新";
            UpdatePolicyBadge.Background = PolicyHintBackground;
            UpdatePolicyText.Foreground = PolicyHintForeground;
        }
    }

    internal void ShowUpdatePrompt(bool forced, string? version, string? messageOverride = null)
    {
        var versionText = string.IsNullOrWhiteSpace(version) ? string.Empty : $" {version}";
        var message = messageOverride ?? (forced
            ? $"检测到新版本{versionText}，需要强制更新才能继续使用。"
            : $"检测到新版本{versionText}，是否立即更新？");
        ResetUpdateProgress();

        UpdateTitleText.Text = forced ? "强制更新" : "发现新版本";
        UpdateMessageText.Text = message;
        UpdateLaterButton.Visibility = forced ? Visibility.Collapsed : Visibility.Visible;
        UpdateLaterButton.IsEnabled = !forced;
        UpdateNowButton.Content = forced ? "确定" : "更新";
        UpdateNowButton.IsEnabled = true;
        UpdateOverlay.Visibility = Visibility.Visible;
    }

    internal void SetUpdateProgress(string message)
    {
        UpdateMessageText.Text = message;
        UpdateNowButton.IsEnabled = false;
        UpdateLaterButton.IsEnabled = false;
        UpdateOverlay.Visibility = Visibility.Visible;
    }

    internal void UpdateDownloadProgress(string stage, long receivedBytes, long? totalBytes)
    {
        UpdateProgressBar.Visibility = Visibility.Visible;
        UpdateProgressText.Visibility = Visibility.Visible;

        if (totalBytes.HasValue && totalBytes.Value > 0)
        {
            UpdateProgressBar.IsIndeterminate = false;
            var percent = Math.Min(100, Math.Round(receivedBytes * 100d / totalBytes.Value, 1));
            UpdateProgressBar.Value = percent;
            UpdateProgressText.Text = $"{stage}：{FormatBytes(receivedBytes)} / {FormatBytes(totalBytes.Value)}（{percent:0.0}%）";
        }
        else
        {
            UpdateProgressBar.IsIndeterminate = true;
            UpdateProgressText.Text = $"{stage}：已下载 {FormatBytes(receivedBytes)}";
        }
    }

    private void ResetUpdateProgress()
    {
        UpdateProgressBar.Visibility = Visibility.Collapsed;
        UpdateProgressText.Visibility = Visibility.Collapsed;
        UpdateProgressBar.IsIndeterminate = false;
        UpdateProgressBar.Value = 0;
        UpdateProgressText.Text = string.Empty;
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
        UpdateOverlay.Visibility = Visibility.Collapsed;
    }
    internal void FocusEmployeeCode()
    {
        EmployeeCodeBox.Focus();
        EmployeeCodeBox.SelectAll();
    }
}

