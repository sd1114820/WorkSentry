using System;
using System.Diagnostics;
using System.IO;
using System.Runtime.InteropServices;
using System.Text;

namespace WorkSentry.Client;

internal static class Win32Interop
{
    public static SampleState CaptureSample(int idleThresholdSeconds)
    {
        var idleSeconds = GetIdleSeconds();
        var isIdle = idleSeconds >= idleThresholdSeconds;

        var windowTitle = string.Empty;
        var processName = string.Empty;

        var handle = GetForegroundWindow();
        if (handle != IntPtr.Zero)
        {
            windowTitle = GetWindowTitle(handle);
            processName = GetProcessName(handle);
        }

        processName = NormalizeProcessName(processName);
        return new SampleState(processName, windowTitle ?? string.Empty, idleSeconds, isIdle);
    }

    private static string NormalizeProcessName(string value)
    {
        value = (value ?? string.Empty).Trim().ToLowerInvariant();
        if (string.IsNullOrEmpty(value))
        {
            return string.Empty;
        }
        return value.EndsWith(".exe") ? value : value + ".exe";
    }

    private static string GetWindowTitle(IntPtr handle)
    {
        var length = GetWindowTextLength(handle);
        if (length <= 0)
        {
            return string.Empty;
        }
        var builder = new StringBuilder(length + 1);
        _ = GetWindowText(handle, builder, builder.Capacity);
        return builder.ToString();
    }

    private static string GetProcessName(IntPtr handle)
    {
        try
        {
            _ = GetWindowThreadProcessId(handle, out var pid);
            if (pid == 0)
            {
                return string.Empty;
            }
            using var process = Process.GetProcessById((int)pid);
            try
            {
                if (!string.IsNullOrWhiteSpace(process.MainModule?.FileName))
                {
                    return Path.GetFileName(process.MainModule.FileName);
                }
            }
            catch
            {
                // ignore main module access issue
            }
            return process.ProcessName;
        }
        catch
        {
            return string.Empty;
        }
    }

    private static int GetIdleSeconds()
    {
        var info = new LASTINPUTINFO();
        info.cbSize = (uint)Marshal.SizeOf(info);
        if (!GetLastInputInfo(ref info))
        {
            return 0;
        }
        var idleMs = (ulong)(GetTickCount64() - info.dwTime);
        if (idleMs < 0)
        {
            idleMs = 0;
        }
        return (int)(idleMs / 1000);
    }

    [DllImport("user32.dll")]
    private static extern IntPtr GetForegroundWindow();

    [DllImport("user32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern int GetWindowText(IntPtr hWnd, StringBuilder text, int count);

    [DllImport("user32.dll", SetLastError = true)]
    private static extern int GetWindowTextLength(IntPtr hWnd);

    [DllImport("user32.dll")]
    private static extern uint GetWindowThreadProcessId(IntPtr hWnd, out uint lpdwProcessId);

    [DllImport("user32.dll")]
    private static extern bool GetLastInputInfo(ref LASTINPUTINFO plii);

    [DllImport("kernel32.dll")]
    private static extern ulong GetTickCount64();

    [StructLayout(LayoutKind.Sequential)]
    private struct LASTINPUTINFO
    {
        public uint cbSize;
        public uint dwTime;
    }
}
