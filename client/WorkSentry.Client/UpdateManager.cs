using System;
using System.Diagnostics;
using System.IO;
using System.IO.Compression;
using System.Net;
using System.Net.Http;
using System.Net.Http.Headers;
using System.Text;
using System.Threading;
using System.Threading.Tasks;

namespace WorkSentry.Client;

internal sealed class UpdateProgress
{
    public string Stage { get; }
    public long ReceivedBytes { get; }
    public long? TotalBytes { get; }

    public UpdateProgress(string stage, long receivedBytes, long? totalBytes)
    {
        Stage = stage;
        ReceivedBytes = receivedBytes;
        TotalBytes = totalBytes;
    }
}

internal sealed class UpdateDownloadResult
{
    public bool Success { get; }
    public string Error { get; }
    public UpdatePackageType PackageType { get; }

    private UpdateDownloadResult(bool success, string error, UpdatePackageType packageType)
    {
        Success = success;
        Error = error;
        PackageType = packageType;
    }

    public static UpdateDownloadResult Ok(UpdatePackageType packageType) => new(true, string.Empty, packageType);

    public static UpdateDownloadResult Fail(string error) => new(false, error, UpdatePackageType.Unknown);
}

internal enum UpdatePackageType
{
    Unknown,
    Exe,
    Zip
}

internal sealed class UpdateManager
{
    private const int ProgressReportIntervalMs = 200;
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
        if (info.Length < 100_000)
        {
            return false;
        }

        return DetectPackageType(_pendingPath) == UpdatePackageType.Exe;
    }

    public async Task<UpdateDownloadResult> DownloadUpdateAsync(string url, IProgress<UpdateProgress>? progress, CancellationToken ct)
    {
        if (string.IsNullOrWhiteSpace(url))
        {
            return UpdateDownloadResult.Fail("更新地址为空");
        }

        PrepareWorkspace();

        var preflight = await PreflightAsync(url, ct).ConfigureAwait(false);
        if (!preflight.Ok)
        {
            return UpdateDownloadResult.Fail(preflight.Error ?? "下载地址不可用");
        }

        progress?.Report(new UpdateProgress("连接中", 0, preflight.TotalBytes));

        try
        {
            using var response = await _httpClient.GetAsync(url, HttpCompletionOption.ResponseHeadersRead, ct).ConfigureAwait(false);
            if (!response.IsSuccessStatusCode)
            {
                return UpdateDownloadResult.Fail($"下载失败: {(int)response.StatusCode}");
            }

            var totalBytes = response.Content.Headers.ContentLength ?? preflight.TotalBytes;
            var hintType = InferPackageType(url, response);

            if (File.Exists(_downloadPath))
            {
                File.Delete(_downloadPath);
            }

            long totalRead = 0;
            var lastReportAt = Environment.TickCount64;

            await using (var stream = await response.Content.ReadAsStreamAsync(ct).ConfigureAwait(false))
            await using (var fileStream = new FileStream(_downloadPath, FileMode.Create, FileAccess.Write, FileShare.Read))
            {
                var buffer = new byte[81920];
                while (true)
                {
                    var read = await stream.ReadAsync(buffer.AsMemory(0, buffer.Length), ct).ConfigureAwait(false);
                    if (read <= 0)
                    {
                        break;
                    }

                    await fileStream.WriteAsync(buffer.AsMemory(0, read), ct).ConfigureAwait(false);
                    totalRead += read;

                    var now = Environment.TickCount64;
                    if (now - lastReportAt >= ProgressReportIntervalMs)
                    {
                        progress?.Report(new UpdateProgress("下载中", totalRead, totalBytes));
                        lastReportAt = now;
                    }
                }
            }

            progress?.Report(new UpdateProgress("下载完成", totalRead, totalBytes));

            if (IsLikelyHtml(_downloadPath))
            {
                return UpdateDownloadResult.Fail("下载内容不是更新文件，请检查下载链接");
            }

            var packageType = DetectPackageType(_downloadPath);
            if (packageType == UpdatePackageType.Unknown)
            {
                packageType = hintType;
            }

            if (packageType == UpdatePackageType.Zip)
            {
                progress?.Report(new UpdateProgress("解压中", totalRead, totalBytes));
                if (!ExtractExeFromZip(_downloadPath, _pendingPath))
                {
                    return UpdateDownloadResult.Fail("压缩包内未找到客户端程序");
                }
            }
            else if (packageType == UpdatePackageType.Exe)
            {
                if (DetectPackageType(_downloadPath) != UpdatePackageType.Exe)
                {
                    return UpdateDownloadResult.Fail("更新文件不是可执行程序");
                }
                File.Copy(_downloadPath, _pendingPath, true);
            }
            else
            {
                if (ExtractExeFromZip(_downloadPath, _pendingPath))
                {
                    packageType = UpdatePackageType.Zip;
                }
                else
                {
                    return UpdateDownloadResult.Fail("更新文件格式不受支持");
                }
            }

            if (File.Exists(_downloadPath))
            {
                File.Delete(_downloadPath);
            }

            _logger.Info("更新文件下载完成");
            return UpdateDownloadResult.Ok(packageType);
        }
        catch (Exception ex)
        {
            _logger.Warn($"更新下载失败: {ex.Message}");
            return UpdateDownloadResult.Fail(ex.Message);
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
            var process = Process.Start(new ProcessStartInfo
            {
                FileName = "cmd.exe",
                Arguments = arguments,
                CreateNoWindow = true,
                UseShellExecute = false,
                WorkingDirectory = _updateDirectory
            });

            if (process == null)
            {
                _logger.Warn("启动更新脚本失败");
                return false;
            }

            return true;
        }
        catch (Exception ex)
        {
            _logger.Warn($"启动更新失败: {ex.Message}");
            return false;
        }
    }

    private static bool IsLikelyHtml(string path)
    {
        try
        {
            using var stream = File.OpenRead(path);
            var buffer = new byte[256];
            var read = stream.Read(buffer, 0, buffer.Length);
            if (read <= 0)
            {
                return false;
            }
            var text = Encoding.UTF8.GetString(buffer, 0, read).TrimStart();
            return text.StartsWith("<!DOCTYPE", StringComparison.OrdinalIgnoreCase)
                   || text.StartsWith("<html", StringComparison.OrdinalIgnoreCase)
                   || text.StartsWith("<!doctype", StringComparison.OrdinalIgnoreCase);
        }
        catch
        {
            return false;
        }
    }

    private static UpdatePackageType InferPackageType(string url, HttpResponseMessage response)
    {
        var path = response.RequestMessage?.RequestUri?.AbsolutePath ?? url;
        if (path.EndsWith(".zip", StringComparison.OrdinalIgnoreCase))
        {
            return UpdatePackageType.Zip;
        }
        if (path.EndsWith(".exe", StringComparison.OrdinalIgnoreCase))
        {
            return UpdatePackageType.Exe;
        }

        var contentType = response.Content.Headers.ContentType?.MediaType ?? string.Empty;
        if (contentType.Contains("zip", StringComparison.OrdinalIgnoreCase))
        {
            return UpdatePackageType.Zip;
        }
        if (contentType.Contains("octet-stream", StringComparison.OrdinalIgnoreCase)
            || contentType.Contains("msdownload", StringComparison.OrdinalIgnoreCase)
            || contentType.Contains("application/exe", StringComparison.OrdinalIgnoreCase))
        {
            return UpdatePackageType.Exe;
        }

        return UpdatePackageType.Unknown;
    }

    private static UpdatePackageType DetectPackageType(string path)
    {
        try
        {
            using var stream = File.OpenRead(path);
            var header = new byte[4];
            var read = stream.Read(header, 0, header.Length);
            if (read >= 2)
            {
                if (header[0] == 0x4D && header[1] == 0x5A)
                {
                    return UpdatePackageType.Exe;
                }
                if (header[0] == 0x50 && header[1] == 0x4B)
                {
                    return UpdatePackageType.Zip;
                }
            }
        }
        catch
        {
            // ignore
        }
        return UpdatePackageType.Unknown;
    }

    private static bool ExtractExeFromZip(string zipPath, string outputPath)
    {
        for (var attempt = 0; attempt < 5; attempt++)
        {
            try
            {
                using var zipStream = new FileStream(zipPath, FileMode.Open, FileAccess.Read, FileShare.ReadWrite);
                using var archive = new ZipArchive(zipStream, ZipArchiveMode.Read);
                ZipArchiveEntry? entry = null;
                foreach (var item in archive.Entries)
                {
                    if (item.Name.Equals("WorkSentry.Client.exe", StringComparison.OrdinalIgnoreCase))
                    {
                        entry = item;
                        break;
                    }
                }
                if (entry == null)
                {
                    foreach (var item in archive.Entries)
                    {
                        if (item.Name.EndsWith(".exe", StringComparison.OrdinalIgnoreCase))
                        {
                            entry = item;
                            break;
                        }
                    }
                }
                if (entry == null)
                {
                    return false;
                }

                using var entryStream = entry.Open();
                using var output = new FileStream(outputPath, FileMode.Create, FileAccess.Write, FileShare.None);
                entryStream.CopyTo(output);
                return true;
            }
            catch (IOException) when (attempt < 4)
            {
                Thread.Sleep(300);
            }
            catch (InvalidDataException)
            {
                return false;
            }
            catch
            {
                return false;
            }
        }
        return false;
    }
    private async Task<(bool Ok, string? Error, long? TotalBytes)> PreflightAsync(string url, CancellationToken ct)
    {
        try
        {
            using var head = new HttpRequestMessage(HttpMethod.Head, url);
            using var resp = await _httpClient.SendAsync(head, HttpCompletionOption.ResponseHeadersRead, ct).ConfigureAwait(false);
            if (resp.IsSuccessStatusCode)
            {
                return (true, null, resp.Content.Headers.ContentLength);
            }
        }
        catch
        {
            // ignore head failures
        }

        try
        {
            using var range = new HttpRequestMessage(HttpMethod.Get, url);
            range.Headers.Range = new RangeHeaderValue(0, 0);
            using var resp = await _httpClient.SendAsync(range, HttpCompletionOption.ResponseHeadersRead, ct).ConfigureAwait(false);
            if (resp.IsSuccessStatusCode || resp.StatusCode == HttpStatusCode.PartialContent)
            {
                var total = resp.Content.Headers.ContentRange?.Length ?? resp.Content.Headers.ContentLength;
                return (true, null, total);
            }

            return (false, $"下载地址不可用: {(int)resp.StatusCode}", null);
        }
        catch (Exception ex)
        {
            return (false, $"下载地址不可用: {ex.Message}", null);
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

