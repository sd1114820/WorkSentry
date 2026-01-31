using System.Threading;
using System.Windows;

namespace WorkSentry.Client;

public partial class App : System.Windows.Application
{
    private TrayManager? _trayManager;
    private Mutex? _instanceMutex;

    protected override void OnStartup(StartupEventArgs e)
    {
        if (!EnsureSingleInstance())
        {
            return;
        }

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

    private bool EnsureSingleInstance()
    {
        _instanceMutex = new Mutex(true, "Local\\WorkSentry.Client.SingleInstance", out var createdNew);
        if (createdNew)
        {
            return true;
        }

        System.Windows.MessageBox.Show("客户端已在运行，请勿重复启动。", "提示", MessageBoxButton.OK, MessageBoxImage.Information);
        Shutdown();
        return false;
    }
}

