using System;
using System.Collections.Generic;
using System.Text.Json;
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
    private readonly Forms.ToolStripMenuItem _breakItem;
    private readonly Forms.ToolStripMenuItem _reportItem;
    private readonly Forms.ToolStripMenuItem _openMainItem;
    private readonly Forms.ToolStripMenuItem _exitItem;
    private readonly MainWindow _mainWindow;
    private AppConfig _config;
    private ReportManager? _reportManager;
    private string _pendingUpdateUrl = string.Empty;
    private string _pendingUpdateVersion = string.Empty;
    private bool _pendingUpdateForced;
    private bool _pendingUpdateReady;
    private bool _isUpdating;
    private bool _isWorking;
    private bool _isBreaking;
    private bool _isStarting;
    private bool _allowExit;
    private string _currentStatusToken = "待上班";

    public TrayManager(Dispatcher dispatcher)
    {
        _dispatcher = dispatcher;
        _configStore = new ConfigStore();
        _config = _configStore.Load();
        _tokenStore = new TokenStore(_configStore.BaseDirectory);
        _logger = new Logger(_configStore.BaseDirectory);

        LanguageService.ApplyLanguage(LanguageService.ResolveLanguage(_config));

        _updateManager = new UpdateManager(_configStore.BaseDirectory, _logger);
        _updateManager.PrepareWorkspace();
        _pendingUpdateReady = _updateManager.HasPendingUpdate();

        _mainWindow = new MainWindow();
        _mainWindow.LoadConfig(_config);
        _mainWindow.SaveConfigRequested += OnSaveConfig;
        _mainWindow.StartRequested += async () => await StartWorkingAsync();
        _mainWindow.StopRequested += async () => await StopWorkingAsync();
        _mainWindow.BreakToggleRequested += async () => await ToggleBreakAsync();
        _mainWindow.ExitRequested += async () => await RequestExitAsync();
        _mainWindow.UpdateNowRequested += () => _ = Task.Run(HandleUpdateNowAsync);
        _mainWindow.UpdateLaterRequested += HandleUpdateLater;
        _mainWindow.LanguageChangedRequested += OnLanguageChangedRequested;
        _mainWindow.Closing += (_, e) =>
        {
            if (!_allowExit)
            {
                if (!_config.SuppressCloseTip)
                {
                    ShowCloseToTrayTip();
                }
                e.Cancel = true;
                _mainWindow.Hide();
            }
        };

        var appIcon = SystemIcons.Application;
        try
        {
            var exePath = Process.GetCurrentProcess().MainModule?.FileName;
            if (!string.IsNullOrWhiteSpace(exePath))
            {
                appIcon = Icon.ExtractAssociatedIcon(exePath) ?? SystemIcons.Application;
            }
        }
        catch
        {
            // ignore
        }
        _notifyIcon = new Forms.NotifyIcon
        {
            Icon = appIcon,
            Visible = true,
            Text = LanguageService.GetString("TrayTooltip")
        };

        var menu = new Forms.ContextMenuStrip();
        _statusItem = new Forms.ToolStripMenuItem(string.Empty) { Enabled = false };
        _startWorkItem = new Forms.ToolStripMenuItem(string.Empty, null, async (_, _) => await StartWorkingAsync());
        _stopWorkItem = new Forms.ToolStripMenuItem(string.Empty, null, async (_, _) => await StopWorkingAsync()) { Enabled = false };
        _breakItem = new Forms.ToolStripMenuItem(string.Empty, null, async (_, _) => await ToggleBreakAsync()) { Enabled = false };
        _reportItem = new Forms.ToolStripMenuItem(string.Empty, null, (_, _) => _reportManager?.RequestImmediateReport()) { Enabled = false };
        _openMainItem = new Forms.ToolStripMenuItem(string.Empty, null, (_, _) => ShowMainWindow());
        _exitItem = new Forms.ToolStripMenuItem(string.Empty, null, async (_, _) => await RequestExitAsync());

        menu.Items.Add(_statusItem);
        menu.Items.Add(new Forms.ToolStripSeparator());
        menu.Items.Add(_startWorkItem);
        menu.Items.Add(_breakItem);
        menu.Items.Add(_stopWorkItem);
        menu.Items.Add(_reportItem);
        menu.Items.Add(new Forms.ToolStripSeparator());
        menu.Items.Add(_openMainItem);
        menu.Items.Add(new Forms.ToolStripSeparator());
        menu.Items.Add(_exitItem);
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

        ApplyTrayLocalization();
        LanguageService.LanguageChanged += HandleLanguageChanged;

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

    private void ApplyTrayLocalization()
    {
        _notifyIcon.Text = LanguageService.GetString("TrayTooltip");
        _statusItem.Text = LanguageService.Format("TrayStatusFormat", LanguageService.GetStatusDisplay(_currentStatusToken));
        _startWorkItem.Text = LanguageService.GetString("TrayStartWork");
        _stopWorkItem.Text = LanguageService.GetString("TrayStopWork");
        _breakItem.Text = _isBreaking ? LanguageService.GetString("TrayBreakStop") : LanguageService.GetString("TrayBreakStart");
        _breakItem.Enabled = _isWorking;
        _reportItem.Text = LanguageService.GetString("TrayReportNow");
        if (_openMainItem != null)
        {
            _openMainItem.Text = LanguageService.GetString("TrayOpenMain");
        }
        if (_exitItem != null)
        {
            _exitItem.Text = LanguageService.GetString("TrayExit");
        }
    }

    private void HandleLanguageChanged()
    {
        InvokeOnUi(() =>
        {
            ApplyTrayLocalization();
            _mainWindow.RefreshLanguage();
        });
    }

    private void OnLanguageChangedRequested(string languageCode)
    {
        _config.LanguageOverride = string.IsNullOrWhiteSpace(languageCode) ? LanguageService.Auto : languageCode;
        _configStore.Save(_config);
        LanguageService.ApplyLanguage(LanguageService.ResolveLanguage(_config));
        ApplyTrayLocalization();
        _mainWindow.RefreshLanguage();
    }

    private void ShowCloseToTrayTip()
    {
        try
        {
            var tip = new CloseToTrayTipWindow
            {
                Owner = _mainWindow
            };
            tip.ShowDialog();
            if (tip.Remember)
            {
                _config.SuppressCloseTip = true;
                _configStore.Save(_config);
            }
        }
        catch
        {
            System.Windows.MessageBox.Show(LanguageService.GetString("CloseToTrayContent"), LanguageService.GetString("DialogTitleTip"), MessageBoxButton.OK, MessageBoxImage.Information);
        }
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
                System.Windows.MessageBox.Show(LanguageService.GetString("MsgNeedStopToEdit"), LanguageService.GetString("DialogTitleTip"), MessageBoxButton.OK, MessageBoxImage.Information);
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
                    System.Windows.MessageBox.Show(LanguageService.GetString("MsgFillEmployeeCode"), LanguageService.GetString("DialogTitleTip"), MessageBoxButton.OK, MessageBoxImage.Information);
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

            var startResult = await _reportManager.SendWorkStartAsync(CancellationToken.None).ConfigureAwait(false);
            if (!startResult)
            {
                InvokeOnUi(() =>
                {
                    System.Windows.MessageBox.Show(LanguageService.GetString("MsgWorkStartFailed"), LanguageService.GetString("DialogTitleTip"), MessageBoxButton.OK, MessageBoxImage.Warning);
                });
                return;
            }
            await _reportManager.StartAsync().ConfigureAwait(false);
            _isWorking = true;
            ApplyBreakState(false, false);
            InvokeOnUi(() =>
            {
                _startWorkItem.Enabled = false;
                _stopWorkItem.Enabled = true;
                _reportItem.Enabled = true;
                _mainWindow.SetWorkingState(true);
            });
            UpdateStatus("连接中");
        }
        catch (Exception ex)
        {
            _logger.Error(ex.Message);
            ShowBalloon(LanguageService.GetString("MsgStartFailedTitle"), ex.Message);
        }
        finally
        {
            _isStarting = false;
        }
    }

    private async Task ToggleBreakAsync()
    {
        if (!_isWorking)
        {
            return;
        }

        EnsureReportManager();
        if (_reportManager == null)
        {
            return;
        }

        ApplyBreakState(!_isBreaking, true);
        await Task.CompletedTask;
    }

    private async Task<bool> StopWorkingAsync()
    {
        if (!_isWorking)
        {
            return true;
        }

        ClientCheckoutPayload? checkoutPayload = null;
        if (_reportManager != null)
        {
            CheckoutTemplateResponse? templateResponse = null;
            try
            {
                templateResponse = await _reportManager.GetCheckoutTemplateAsync(CancellationToken.None).ConfigureAwait(false);
            }
            catch (Exception ex)
            {
                _logger.Warn(ex.Message);
                InvokeOnUi(() =>
                {
                    var message = ex is ApiException ? ex.Message : LanguageService.GetString("MsgCheckoutTemplateFailed");
                    System.Windows.MessageBox.Show(message, LanguageService.GetString("DialogTitleTip"), MessageBoxButton.OK, MessageBoxImage.Warning);
                });
                return false;
            }

            if (templateResponse != null && templateResponse.Exists)
            {
                if (templateResponse.Template == null)
                {
                    InvokeOnUi(() =>
                    {
                        System.Windows.MessageBox.Show(LanguageService.GetString("MsgCheckoutTemplateFailed"), LanguageService.GetString("DialogTitleTip"), MessageBoxButton.OK, MessageBoxImage.Warning);
                    });
                    return false;
                }

                if (templateResponse.Template.Fields == null || templateResponse.Template.Fields.Count == 0)
                {
                    checkoutPayload = new ClientCheckoutPayload
                    {
                        TemplateId = templateResponse.Template.TemplateId
                    };
                }
                else
                {
                    var checkoutResult = await ShowCheckoutDialogAsync(templateResponse.Template).ConfigureAwait(false);
                    if (checkoutResult == null)
                    {
                        return false;
                    }
                    checkoutPayload = checkoutResult;
                }
            }

            var stopResult = await _reportManager.SendWorkEndAsync(checkoutPayload, null, CancellationToken.None).ConfigureAwait(false);
            if (!stopResult.Success && string.Equals(stopResult.ErrorCode, "need_reason", StringComparison.OrdinalIgnoreCase))
            {
                var reason = await ShowWorkEndReasonDialogAsync(stopResult.ErrorData).ConfigureAwait(false);
                if (string.IsNullOrWhiteSpace(reason))
                {
                    return false;
                }
                stopResult = await _reportManager.SendWorkEndAsync(checkoutPayload, reason, CancellationToken.None).ConfigureAwait(false);
            }

            if (!stopResult.Success)
            {
                InvokeOnUi(() =>
                {
                    var message = string.IsNullOrWhiteSpace(stopResult.Error)
                        ? LanguageService.GetString("MsgWorkEndFailed")
                        : stopResult.Error!;
                    System.Windows.MessageBox.Show(message, LanguageService.GetString("DialogTitleTip"), MessageBoxButton.OK, MessageBoxImage.Warning);
                });
                return false;
            }
            _reportManager.Stop();
        }

        _isWorking = false;
        ApplyBreakState(false, false);
        InvokeOnUi(() =>
        {
            _startWorkItem.Enabled = true;
            _stopWorkItem.Enabled = false;
            _breakItem.Enabled = false;
            _reportItem.Enabled = false;
            _mainWindow.SetWorkingState(false);
        });
        UpdateStatus("已下班");
        return true;
    }

    private Task<ClientCheckoutPayload?> ShowCheckoutDialogAsync(CheckoutTemplate template)
    {
        var tcs = new TaskCompletionSource<ClientCheckoutPayload?>();
        InvokeOnUi(() =>
        {
            var dialog = new CheckoutFormWindow(template)
            {
                Owner = _mainWindow
            };
            var result = dialog.ShowDialog();
            if (result == true && dialog.Result != null)
            {
                tcs.TrySetResult(dialog.Result);
                return;
            }
            tcs.TrySetResult(null);
        });
        return tcs.Task;
    }

    private Task<string?> ShowWorkEndReasonDialogAsync(JsonElement? data)
    {
        var tcs = new TaskCompletionSource<string?>();
        InvokeOnUi(() =>
        {
            var messages = ExtractViolationMessages(data);
            var dialog = new WorkEndReasonWindow(messages)
            {
                Owner = _mainWindow
            };
            var result = dialog.ShowDialog();
            if (result == true && !string.IsNullOrWhiteSpace(dialog.Reason))
            {
                tcs.TrySetResult(dialog.Reason);
                return;
            }
            tcs.TrySetResult(null);
        });
        return tcs.Task;
    }

    private static List<string> ExtractViolationMessages(JsonElement? data)
    {
        var results = new List<string>();
        if (data.HasValue && data.Value.ValueKind == JsonValueKind.Object)
        {
            if (data.Value.TryGetProperty("violations", out var violations) && violations.ValueKind == JsonValueKind.Array)
            {
                foreach (var item in violations.EnumerateArray())
                {
                    if (item.TryGetProperty("message", out var msg))
                    {
                        var text = msg.GetString();
                        if (!string.IsNullOrWhiteSpace(text))
                        {
                            results.Add(text);
                        }
                    }
                }
            }
        }
        return results;
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
                UpdateStatus("已上班");
                return;
            }
            if (status == "已绑定")
            {
                UpdateStatus("连接中");
                return;
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


    private void ApplyBreakState(bool isBreaking, bool updateStatus)
    {
        _isBreaking = isBreaking;
        _reportManager?.SetBreakState(isBreaking);
        _breakItem.Text = isBreaking ? LanguageService.GetString("TrayBreakStop") : LanguageService.GetString("TrayBreakStart");
        _breakItem.Enabled = _isWorking;
        InvokeOnUi(() => _mainWindow.SetBreakState(isBreaking));
        if (updateStatus)
        {
            UpdateStatus(isBreaking ? "休息中" : "已上班");
        }
    }
    private void UpdateStatus(string status)
    {
        var displayStatus = status;
        if (_isBreaking && (status == "已上班" || status == "连接中" || status == "已上报"))
        {
            displayStatus = "休息中";
        }
        _currentStatusToken = displayStatus;
        InvokeOnUi(() =>
        {
            if (_statusItem.GetCurrentParent() == null)
            {
                return;
            }
            _statusItem.Text = LanguageService.Format("TrayStatusFormat", LanguageService.GetStatusDisplay(displayStatus));
            _mainWindow.UpdateStatus(displayStatus);
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
                InvokeOnUi(() => _mainWindow.SetUpdateProgress(LanguageService.GetString("UpdateProgressDownloading")));
                result = await _updateManager.DownloadUpdateAsync(_pendingUpdateUrl, progress, CancellationToken.None).ConfigureAwait(false);
                if (!result.Success)
                {
                    var errorText = LanguageService.TranslateUpdateError(result.Error ?? string.Empty);
                    var failMessage = LanguageService.Format("UpdateDownloadFailed", errorText);
                    if (_pendingUpdateForced)
                    {
                        InvokeOnUi(() => _mainWindow.SetUpdateProgress(failMessage));
                        await Task.Delay(1500).ConfigureAwait(false);
                        ExitInternal();
                        return;
                    }

                    InvokeOnUi(() => _mainWindow.ShowUpdatePrompt(false, _pendingUpdateVersion, failMessage));
                    return;
                }
            }

            _pendingUpdateReady = true;
            InvokeOnUi(() => _mainWindow.SetUpdateProgress(LanguageService.GetString("UpdateProgressApplying")));
            if (_updateManager.ApplyPendingUpdate())
            {
                InvokeOnUi(() => _mainWindow.SetUpdateProgress(LanguageService.GetString("UpdateProgressRestarting")));
                await Task.Delay(300).ConfigureAwait(false);
                Environment.Exit(0);
                return;
            }

            if (_pendingUpdateForced)
            {
                InvokeOnUi(() => _mainWindow.SetUpdateProgress(LanguageService.GetString("UpdateApplyFailedForced")));
                await Task.Delay(1500).ConfigureAwait(false);
                ExitInternal();
                return;
            }

            InvokeOnUi(() => _mainWindow.ShowUpdatePrompt(false, _pendingUpdateVersion, LanguageService.GetString("UpdateApplyFailed")));
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

    private async Task RequestExitAsync()
    {
        if (_isWorking)
        {
            var result = System.Windows.MessageBox.Show(LanguageService.GetString("MsgExitNeedStop"), LanguageService.GetString("DialogTitleTip"), MessageBoxButton.OKCancel, MessageBoxImage.Warning);
            if (result != MessageBoxResult.OK)
            {
                return;
            }

            var stopped = await StopWorkingAsync().ConfigureAwait(false);
            if (!stopped)
            {
                return;
            }
        }

        ExitInternal();
    }

    private void ExitInternal()
    {
        _allowExit = true;
        _reportManager?.Stop();
        _notifyIcon.Visible = false;
        _notifyIcon.Dispose();
        if (_mainWindow.IsVisible)
        {
            _mainWindow.Close();
        }
        System.Windows.Application.Current.Shutdown();
    }
}




