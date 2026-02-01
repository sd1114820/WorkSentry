using System;
using System.Diagnostics;
using System.IO;
using System.Net.Http;
using System.Text;
using System.Threading;
using System.Threading.Tasks;

namespace WorkSentry.Client;

internal sealed class UpdateManager
{
    private readonly Logger _logger;
    private readonly string _updateDirectory;
    private readonly string _updateScriptPath;
    private readonly string _downloadPath;
    private readonly string _pendingPath;
    private readonly HttpClient _httpClient;

    public UpdateManager(string baseDirectory, Logger logger)
    {
        _logger = logger;
        _updateDirectory = Path.Combine(baseDirectory, "updates");
        _updateScriptPath = Path.Combine(_updateDirectory, "apply_update.cmd");
        _downloadPath = Path.Combine(_updateDirectory, "WorkSentry.Client.download");
        _pendingPath = Path.Combine(_updateDirectory, "WorkSentry.Client.new.exe");
        _httpClient = new HttpClient
        {
            Timeout = TimeSpan.FromMinutes(5)
        };
    }

    public void PrepareWorkspace()
    {
        Directory.CreateDirectory(_updateDirectory);
        if (!File.Exists(_updateScriptPath))
        {
            File.WriteAllText(_updateScriptPath, BuildScript(), new UTF8Encoding(false));
        }

        if (!File.Exists(_pendingPath))
        {
            File.WriteAllBytes(_pendingPath, Array.Empty<byte>());
        }
    }

    public bool HasPendingUpdate()
    {
        if (!File.Exists(_pendingPath))
        {
            return false;
        }

        var info = new FileInfo(_pendingPath);
        return info.Length >= 5_000_000;
    }

    public async Task<bool> DownloadUpdateAsync(string url, CancellationToken ct)
    {
        if (string.IsNullOrWhiteSpace(url))
        {
            return false;
        }

        try
        {
            PrepareWorkspace();
            using var response = await _httpClient.GetAsync(url, HttpCompletionOption.ResponseHeadersRead, ct).ConfigureAwait(false);
            response.EnsureSuccessStatusCode();
            await using var stream = await response.Content.ReadAsStreamAsync(ct).ConfigureAwait(false);
            await using var fileStream = new FileStream(_downloadPath, FileMode.Create, FileAccess.Write, FileShare.None);
            await stream.CopyToAsync(fileStream, ct).ConfigureAwait(false);

            var info = new FileInfo(_downloadPath);
            if (info.Length < 5_000_000)
            {
                _logger.Warn($"更新文件体积异常: {info.Length} bytes");
                return false;
            }

            File.Copy(_downloadPath, _pendingPath, true);
            File.Delete(_downloadPath);
            _logger.Info("更新文件下载完成");
            return true;
        }
        catch (Exception ex)
        {
            _logger.Warn($"更新下载失败: {ex.Message}");
            return false;
        }
    }

    public bool ApplyPendingUpdate()
    {
        try
        {
            if (!HasPendingUpdate())
            {
                _logger.Warn("未找到有效的更新文件");
                return false;
            }

            var currentExe = Process.GetCurrentProcess().MainModule?.FileName;
            if (string.IsNullOrWhiteSpace(currentExe))
            {
                _logger.Warn("无法获取当前程序路径");
                return false;
            }

            var pid = Process.GetCurrentProcess().Id;
            var arguments = $"/c \"\"{_updateScriptPath}\" \"{_pendingPath}\" \"{currentExe}\" {pid}\"";
            Process.Start(new ProcessStartInfo
            {
                FileName = "cmd.exe",
                Arguments = arguments,
                CreateNoWindow = true,
                UseShellExecute = false,
                WorkingDirectory = _updateDirectory
            });
            return true;
        }
        catch (Exception ex)
        {
            _logger.Warn($"启动更新失败: {ex.Message}");
            return false;
        }
    }

    private static string BuildScript()
    {
        var nl = Environment.NewLine;
        return string.Join(nl, new[]
        {
            "@echo off",
            "setlocal enabledelayedexpansion",
            "set NEW=%~1",
            "set TARGET=%~2",
            "set PID=%~3",
            "if \"%NEW%\"==\"\" exit /b 1",
            "if \"%TARGET%\"==\"\" exit /b 1",
            "if \"%PID%\"==\"\" set PID=0",
            ":wait",
            "tasklist /FI \"PID eq %PID%\" | find /I \"%PID%\" >nul",
            "if %errorlevel%==0 (",
            "  timeout /t 1 /nobreak >nul",
            "  goto wait",
            ")",
            "timeout /t 1 /nobreak >nul",
            "copy /Y \"%NEW%\" \"%TARGET%\" >nul",
            "del /f /q \"%NEW%\" >nul",
            "start \"\" \"%TARGET%\"",
            "exit /b 0"
        });
    }
}
