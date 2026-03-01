using System;
using System.Collections.Generic;
using System.Threading;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Threading;

namespace WorkSentry.Client;

public partial class App : System.Windows.Application
{
    private TrayManager? _trayManager;
    private Mutex? _instanceMutex;

    protected override void OnStartup(StartupEventArgs e)
    {
        ApplyStartupLanguage();
        if (!EnsureSingleInstance())
        {
            return;
        }

        var configStore = new ConfigStore();
        var config = configStore.Load();
        var logger = new Logger(configStore.BaseDirectory);
        var errorQueue = new ClientErrorQueue(configStore.BaseDirectory);
        InstallGlobalErrorHandlers(logger, errorQueue, config);

        base.OnStartup(e);
        ShutdownMode = ShutdownMode.OnExplicitShutdown;
        _trayManager = new TrayManager(Dispatcher);
        _trayManager.Show();
    }

    protected override void OnExit(ExitEventArgs e)
    {
        _trayManager?.Dispose();
        _instanceMutex?.ReleaseMutex();
        _instanceMutex?.Dispose();
        base.OnExit(e);
    }

    private void ApplyStartupLanguage()
    {
        try
        {
            var config = new ConfigStore().Load();
            LanguageService.ApplyLanguage(LanguageService.ResolveLanguage(config));
        }
        catch
        {
            LanguageService.ApplyLanguage(LanguageService.DetectSystemLanguage());
        }
    }

    private bool EnsureSingleInstance()
    {
        _instanceMutex = new Mutex(true, "Local\\WorkSentry.Client.SingleInstance", out var createdNew);
        if (createdNew)
        {
            return true;
        }

        System.Windows.MessageBox.Show(LanguageService.GetString("MsgSingleInstance"), LanguageService.GetString("DialogTitleTip"), MessageBoxButton.OK, MessageBoxImage.Information);
        Shutdown();
        return false;
    }

    private void InstallGlobalErrorHandlers(Logger logger, ClientErrorQueue errorQueue, AppConfig config)
    {
        DispatcherUnhandledException += (_, args) =>
        {
            EnqueueException(logger, errorQueue, config, "ui_unhandled_exception", args.Exception);
            args.Handled = true;
        };

        AppDomain.CurrentDomain.UnhandledException += (_, args) =>
        {
            if (args.ExceptionObject is Exception ex)
            {
                EnqueueException(logger, errorQueue, config, "domain_unhandled_exception", ex);
            }
            else
            {
                logger.Error("domain_unhandled_exception");
            }
        };

        TaskScheduler.UnobservedTaskException += (_, args) =>
        {
            EnqueueException(logger, errorQueue, config, "task_unobserved_exception", args.Exception);
            args.SetObserved();
        };
    }

    private static void EnqueueException(Logger logger, ClientErrorQueue errorQueue, AppConfig config, string errorType, Exception ex)
    {
        try
        {
            logger.Error($"{errorType}: {ex}");
        }
        catch
        {
            // ignore
        }

        try
        {
            var context = new Dictionary<string, string>
            {
                ["serverUrl"] = config.ServerUrl ?? string.Empty,
                ["employeeCode"] = config.EmployeeCode ?? string.Empty,
                ["osVersion"] = Environment.OSVersion.VersionString,
                ["dotnet"] = Environment.Version.ToString(),
            };

            errorQueue.Enqueue(new ClientErrorReportRequest
            {
                OccurredAt = DateTime.UtcNow.ToString("O"),
                ErrorType = errorType,
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
    }
}
