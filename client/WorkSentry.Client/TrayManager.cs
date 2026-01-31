using System;
using System.Diagnostics;
using System.Drawing;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Threading;
using Forms = System.Windows.Forms;

namespace WorkSentry.Client;

internal sealed class TrayManager : IDisposable
{
    private readonly ConfigStore _configStore;
    private readonly TokenStore _tokenStore;
    private readonly Logger _logger;
    private readonly Forms.NotifyIcon _notifyIcon;
    private readonly Dispatcher _dispatcher;
    private readonly Forms.ToolStripMenuItem _statusItem;
    private readonly Forms.ToolStripMenuItem _startWorkItem;
    private readonly Forms.ToolStripMenuItem _stopWorkItem;
    private readonly Forms.ToolStripMenuItem _reportItem;
    private readonly MainWindow _mainWindow;
    private AppConfig _config;
    private ReportManager? _reportManager;
    private string _pendingUpdateUrl = string.Empty;
    private bool _isWorking;
    private bool _isStarting;
    private bool _allowExit;

    public TrayManager(Dispatcher dispatcher)
    {
        _dispatcher = dispatcher;
        _configStore = new ConfigStore();
        _config = _configStore.Load();
        _tokenStore = new TokenStore(_configStore.BaseDirectory);
        _logger = new Logger(_configStore.BaseDirectory);

        _mainWindow = new MainWindow();
        _mainWindow.LoadConfig(_config);
        _mainWindow.SaveConfigRequested += OnSaveConfig;
        _mainWindow.StartRequested += async () => await StartWorkingAsync();
        _mainWindow.StopRequested += StopWorking;
        _mainWindow.ExitRequested += Exit;
        _mainWindow.Closing += (_, e) =>
        {
            if (!_allowExit)
            {
                e.Cancel = true;
                _mainWindow.Hide();
            }
        };

        _notifyIcon = new Forms.NotifyIcon
        {
            Icon = SystemIcons.Application,
            Visible = true,
            Text = "WorkSentry 客户端"
        };

        var menu = new Forms.ContextMenuStrip();
        _statusItem = new Forms.ToolStripMenuItem("状态：待上班") { Enabled = false };
        _startWorkItem = new Forms.ToolStripMenuItem("开始上班", null, async (_, _) => await StartWorkingAsync());
        _stopWorkItem = new Forms.ToolStripMenuItem("下班", null, (_, _) => StopWorking()) { Enabled = false };
        _reportItem = new Forms.ToolStripMenuItem("立即上报", null, (_, _) => _reportManager?.RequestImmediateReport()) { Enabled = false };
        var openMainItem = new Forms.ToolStripMenuItem("打开主界面", null, (_, _) => ShowMainWindow());
        var exitItem = new Forms.ToolStripMenuItem("退出", null, (_, _) => Exit());

        menu.Items.Add(_statusItem);
        menu.Items.Add(new Forms.ToolStripSeparator());
        menu.Items.Add(_startWorkItem);
        menu.Items.Add(_stopWorkItem);
        menu.Items.Add(_reportItem);
        menu.Items.Add(new Forms.ToolStripSeparator());
        menu.Items.Add(openMainItem);
        menu.Items.Add(new Forms.ToolStripSeparator());
        menu.Items.Add(exitItem);
        _notifyIcon.ContextMenuStrip = menu;

        _notifyIcon.DoubleClick += (_, _) => ShowMainWindow();
        _notifyIcon.BalloonTipClicked += (_, _) =>
        {
            if (!string.IsNullOrWhiteSpace(_pendingUpdateUrl))
            {
                OpenUrl(_pendingUpdateUrl);
            }
        };

        _ = InitializeAsync();
    }

    public void Show()
    {
        _mainWindow.Show();
        _mainWindow.Activate();
    }

    public void Dispose()
    {
        _reportManager?.Stop();
        _notifyIcon.Visible = false;
        _notifyIcon.Dispose();
    }

    private async Task InitializeAsync()
    {
        AutoStartHelper.EnsureAutoStart(_logger);
        UpdateStatus("待上班");
        await Task.CompletedTask;
    }

    private void ShowMainWindow()
    {
        InvokeOnUi(() =>
        {
            if (_mainWindow.IsVisible)
            {
                _mainWindow.Activate();
                return;
            }
            _mainWindow.Show();
            _mainWindow.Activate();
        });
    }

    private void OnSaveConfig(string employeeCode)
    {
        if (_isWorking)
        {
            InvokeOnUi(() =>
            {
                System.Windows.MessageBox.Show("请先下班再修改工号", "提示", MessageBoxButton.OK, MessageBoxImage.Information);
            });
            return;
        }

        _config.EmployeeCode = employeeCode;
        _configStore.Save(_config);
        InvokeOnUi(() => _mainWindow.LoadConfig(_config));
        _reportManager?.ResetBinding();
        UpdateStatus("配置已保存");
    }

    private async Task StartWorkingAsync()
    {
        if (_isWorking || _isStarting)
        {
            return;
        }

        _isStarting = true;
        try
        {
            if (string.IsNullOrWhiteSpace(_config.EmployeeCode))
            {
                InvokeOnUi(() =>
                {
                    System.Windows.MessageBox.Show("请先填写工号", "提示", MessageBoxButton.OK, MessageBoxImage.Information);
                    _mainWindow.FocusEmployeeCode();
                });
                ShowMainWindow();
                return;
            }

            EnsureReportManager();
            if (_reportManager == null)
            {
                return;
            }

            await _reportManager.StartAsync().ConfigureAwait(false);
            _isWorking = true;
            InvokeOnUi(() =>
            {
                _startWorkItem.Enabled = false;
                _stopWorkItem.Enabled = true;
                _reportItem.Enabled = true;
                _mainWindow.SetWorkingState(true);
            });
            UpdateStatus("已上班");
        }
        catch (Exception ex)
        {
            _logger.Error(ex.Message);
            ShowBalloon("启动失败", ex.Message);
        }
        finally
        {
            _isStarting = false;
        }
    }

    private void StopWorking()
    {
        if (!_isWorking)
        {
            return;
        }

        _reportManager?.Stop();
        _isWorking = false;
        InvokeOnUi(() =>
        {
            _startWorkItem.Enabled = true;
            _stopWorkItem.Enabled = false;
            _reportItem.Enabled = false;
            _mainWindow.SetWorkingState(false);
        });
        UpdateStatus("已下班");
    }

    private void EnsureReportManager()
    {
        if (_reportManager != null)
        {
            return;
        }

        _reportManager = new ReportManager(_config, _configStore, _tokenStore, _logger);
        _reportManager.StatusChanged += status =>
        {
            if (status == "已上报")
            {
                InvokeOnUi(() => _mainWindow.UpdateLastReport(DateTime.Now));
            }
            UpdateStatus(status);
        };
        _reportManager.OptionalUpdate += OnOptionalUpdate;
        _reportManager.ForcedUpdate += OnForcedUpdate;
        _reportManager.SettingsChanged += config =>
        {
            InvokeOnUi(() => _mainWindow.UpdateUpdateInfo(config.UpdatePolicy, config.LatestVersion));
        };
    }

    private void UpdateStatus(string status)
    {
        InvokeOnUi(() =>
        {
            if (_statusItem.GetCurrentParent() == null)
            {
                return;
            }
            _statusItem.Text = $"状态：{status}";
            _mainWindow.UpdateStatus(status);
        });
    }

    private void OnOptionalUpdate(string? version, string? url)
    {
        InvokeOnUi(() =>
        {
            _pendingUpdateUrl = url ?? string.Empty;
            ShowBalloon("发现新版本", $"最新版本 {version}，可选择稍后更新。", 5000);
        });
    }

    private void OnForcedUpdate(string? version, string? url)
    {
        InvokeOnUi(() =>
        {
            var message = string.IsNullOrWhiteSpace(version)
                ? "需要强制更新才能继续使用。"
                : $"检测到新版本 {version}，需要强制更新才能继续使用。";

            var result = System.Windows.MessageBox.Show(message + "\n点击确定开始更新。", "强制更新", MessageBoxButton.OKCancel, MessageBoxImage.Warning);
            if (result == MessageBoxResult.OK && !string.IsNullOrWhiteSpace(url))
            {
                OpenUrl(url);
            }
            Exit();
        });
    }

    private void InvokeOnUi(Action action)
    {
        if (_dispatcher.CheckAccess())
        {
            action();
            return;
        }
        _dispatcher.Invoke(action);
    }

    private void ShowBalloon(string title, string message, int timeout = 3000)
    {
        _notifyIcon.ShowBalloonTip(timeout, title, message, Forms.ToolTipIcon.Info);
    }

    private void OpenUrl(string url)
    {
        try
        {
            Process.Start(new ProcessStartInfo
            {
                FileName = url,
                UseShellExecute = true
            });
        }
        catch
        {
            // ignore
        }
    }

    private void Exit()
    {
        _allowExit = true;
        _reportManager?.Stop();
        _notifyIcon.Visible = false;
        _notifyIcon.Dispose();
        InvokeOnUi(() =>
        {
            if (_mainWindow.IsVisible)
            {
                _mainWindow.Close();
            }
            System.Windows.Application.Current.Shutdown();
        });
        _ = Task.Run(async () =>
        {
            await Task.Delay(500);
            Environment.Exit(0);
        });
    }
}

