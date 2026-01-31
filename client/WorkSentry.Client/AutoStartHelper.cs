using Microsoft.Win32;
using System;
using System.Windows.Forms;

namespace WorkSentry.Client;

internal static class AutoStartHelper
{
    public static void EnsureAutoStart(Logger logger)
    {
        try
        {
            using var key = Registry.CurrentUser.OpenSubKey("SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Run", true);
            if (key == null)
            {
                return;
            }
            var exePath = Application.ExecutablePath;
            key.SetValue(AppConstants.AppName, $"\"{exePath}\"");
        }
        catch (Exception ex)
        {
            logger.Warn($"设置开机自启失败: {ex.Message}");
        }
    }
}
