using System;
using System.Globalization;
using System.IO;
using System.Linq;
using System.Text;

namespace WorkSentry.Client;

internal sealed class Logger
{
    private readonly string _logDirectory;
    private readonly object _lock = new();

    public Logger(string baseDirectory)
    {
        _logDirectory = Path.Combine(baseDirectory, "logs");
        Directory.CreateDirectory(_logDirectory);
        CleanupOldLogs();
    }

    public void Info(string message) => Write("INFO", message);
    public void Warn(string message) => Write("WARN", message);
    public void Error(string message) => Write("ERROR", message);

    private void Write(string level, string message)
    {
        var now = DateTime.Now;
        var file = Path.Combine(_logDirectory, $"{now:yyyy-MM-dd}.log");
        var line = $"[{now:yyyy-MM-dd HH:mm:ss}] [{level}] {message}{Environment.NewLine}";
        lock (_lock)
        {
            File.AppendAllText(file, line, Encoding.UTF8);
        }
    }

    private void CleanupOldLogs()
    {
        try
        {
            var cutoff = DateTime.Now.Date.AddDays(-7);
            foreach (var file in Directory.GetFiles(_logDirectory, "*.log"))
            {
                var name = Path.GetFileNameWithoutExtension(file);
                if (DateTime.TryParseExact(name, "yyyy-MM-dd", CultureInfo.InvariantCulture, DateTimeStyles.None, out var date))
                {
                    if (date < cutoff)
                    {
                        File.Delete(file);
                    }
                }
            }
        }
        catch
        {
            // ignore cleanup errors
        }
    }
}
