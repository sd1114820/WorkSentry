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

    public MainWindow()
    {
        InitializeComponent();
        VersionBadgeText.Text = $"v{AppConstants.ClientVersion}";
        VersionText.Text = AppConstants.ClientVersion;

        SaveButton.Click += (_, _) => SaveConfig();
        StartButton.Click += (_, _) => StartRequested?.Invoke();
        StopButton.Click += (_, _) => StopRequested?.Invoke();
        ExitButton.Click += (_, _) => ExitRequested?.Invoke();
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

    internal void FocusEmployeeCode()
    {
        EmployeeCodeBox.Focus();
        EmployeeCodeBox.SelectAll();
    }
}

