using System;
using System.Diagnostics;
using System.Drawing;
using System.Threading;
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
    private readonly UpdateManager _updateManager;
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
    private string _pendingUpdateVersion = string.Empty;
    private bool _pendingUpdateForced;
    private bool _pendingUpdateReady;
    private bool _isUpdating;
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

        _updateManager = new UpdateManager(_configStore.BaseDirectory, _logger);
        _updateManager.PrepareWorkspace();
        _pendingUpdateReady = _updateManager.HasPendingUpdate();

        _mainWindow = new MainWindow();
        _mainWindow.LoadConfig(_config);
        _mainWindow.SaveConfigRequested += OnSaveConfig;
        _mainWindow.StartRequested += async () => await StartWorkingAsync();
        _mainWindow.StopRequested += StopWorking;
        _mainWindow.ExitRequested += Exit;
        _mainWindow.UpdateNowRequested += () => _ = Task.Run(HandleUpdateNowAsync);
        _mainWindow.UpdateLaterRequested += HandleUpdateLater;
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
            if (string.IsNullOrWhiteSpace(_pendingUpdateUrl) && !_pendingUpdateReady)
            {
                return;
            }
            ShowMainWindow();
            InvokeOnUi(() => _mainWindow.ShowUpdatePrompt(_pendingUpdateForced, _pendingUpdateVersion));
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
        await CheckUpdateOnStartupAsync().ConfigureAwait(false);
    }

    private async Task CheckUpdateOnStartupAsync()
    {
        if (string.IsNullOrWhiteSpace(_config.EmployeeCode))
        {
            return;
        }

        try
        {
            EnsureReportManager();
            if (_reportManager != null)
            {
                await _reportManager.CheckUpdateAsync(CancellationToken.None).ConfigureAwait(false);
            }
        }
        catch (Exception ex)
        {
            _logger.Warn($"启动更新检查失败: {ex.Message}");
        }
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

        if (_pendingUpdateForced)
        {
            InvokeOnUi(() =>
            {
                ShowMainWindow();
                _mainWindow.ShowUpdatePrompt(true, _pendingUpdateVersion);
            });
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
        _ = Task.Run(() => HandleOptionalUpdateAsync(version, url));
    }

    private void OnForcedUpdate(string? version, string? url)
    {
        _ = Task.Run(() => HandleForcedUpdateAsync(version, url));
    }

    private async Task HandleOptionalUpdateAsync(string? version, string? url)
    {
        if (string.IsNullOrWhiteSpace(url))
        {
            return;
        }

        _pendingUpdateUrl = url;
        _pendingUpdateVersion = version ?? string.Empty;
        _pendingUpdateForced = false;
        _pendingUpdateReady = _updateManager.HasPendingUpdate();

        InvokeOnUi(() =>
        {
            ShowMainWindow();
            _mainWindow.ShowUpdatePrompt(false, _pendingUpdateVersion);
        });
        await Task.CompletedTask;
    }

    private async Task HandleForcedUpdateAsync(string? version, string? url)
    {
        _pendingUpdateVersion = version ?? string.Empty;
        _pendingUpdateUrl = url ?? string.Empty;
        _pendingUpdateForced = true;
        _pendingUpdateReady = _updateManager.HasPendingUpdate();

        _reportManager?.Stop();
        _isWorking = false;
        UpdateStatus("需要更新");

        InvokeOnUi(() =>
        {
            _startWorkItem.Enabled = false;
            _stopWorkItem.Enabled = false;
            _reportItem.Enabled = false;
            _mainWindow.SetWorkingState(false);
            ShowMainWindow();
            _mainWindow.ShowUpdatePrompt(true, _pendingUpdateVersion);
        });
        await Task.CompletedTask;
    }
    private void HandleUpdateLater()
    {
        if (_pendingUpdateForced)
        {
            return;
        }

        _pendingUpdateVersion = string.Empty;
        _pendingUpdateUrl = string.Empty;
        _pendingUpdateReady = _updateManager.HasPendingUpdate();
        InvokeOnUi(() => _mainWindow.HideUpdatePrompt());
        _reportManager?.ResetOptionalUpdateNotice();
    }

    private async Task HandleUpdateNowAsync()
    {
        if (_isUpdating)
        {
            return;
        }

        if (string.IsNullOrWhiteSpace(_pendingUpdateUrl) && !_pendingUpdateReady)
        {
            return;
        }

        _isUpdating = true;
        try
        {
            var progress = new Progress<UpdateProgress>(p =>
            {
                InvokeOnUi(() => _mainWindow.UpdateDownloadProgress(p.Stage, p.ReceivedBytes, p.TotalBytes));
            });

            UpdateDownloadResult result = UpdateDownloadResult.Ok(UpdatePackageType.Exe);
            if (!_pendingUpdateReady)
            {
                InvokeOnUi(() => _mainWindow.SetUpdateProgress("正在下载更新，请稍后..."));
                result = await _updateManager.DownloadUpdateAsync(_pendingUpdateUrl, progress, CancellationToken.None).ConfigureAwait(false);
                if (!result.Success)
                {
                    if (_pendingUpdateForced)
                    {
                        InvokeOnUi(() => _mainWindow.SetUpdateProgress($"更新下载失败：{result.Error}"));
                        await Task.Delay(1500).ConfigureAwait(false);
                        Exit();
                        return;
                    }

                    InvokeOnUi(() => _mainWindow.ShowUpdatePrompt(false, _pendingUpdateVersion, $"更新下载失败：{result.Error}"));
                    return;
                }
            }

            _pendingUpdateReady = true;
            InvokeOnUi(() => _mainWindow.SetUpdateProgress("正在应用更新，稍后自动重启..."));
            if (_updateManager.ApplyPendingUpdate())
            {
                InvokeOnUi(() => _mainWindow.SetUpdateProgress("正在重启，请稍候..."));
                await Task.Delay(300).ConfigureAwait(false);
                Environment.Exit(0);
                return;
            }

            if (_pendingUpdateForced)
            {
                InvokeOnUi(() => _mainWindow.SetUpdateProgress("启动更新失败，程序将退出。"));
                await Task.Delay(1500).ConfigureAwait(false);
                Exit();
                return;
            }

            InvokeOnUi(() => _mainWindow.ShowUpdatePrompt(false, _pendingUpdateVersion, "启动更新失败，请稍后重试。"));
        }
        finally
        {
            _isUpdating = false;
        }
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

