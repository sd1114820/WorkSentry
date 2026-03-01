using System;
using System.Collections.Generic;
using System.IO;
using System.Text;
using System.Text.Json;
using System.Threading;
using System.Threading.Tasks;

namespace WorkSentry.Client;

internal sealed class ClientErrorQueue
{
    private readonly string _queueFile;
    private readonly object _lock = new();
    private readonly JsonSerializerOptions _options = new() { PropertyNamingPolicy = JsonNamingPolicy.CamelCase };

    public ClientErrorQueue(string baseDirectory)
    {
        var dir = Path.Combine(baseDirectory, "errors");
        Directory.CreateDirectory(dir);
        _queueFile = Path.Combine(dir, "pending.jsonl");
    }

    public void Enqueue(ClientErrorReportRequest report)
    {
        if (report == null)
        {
            return;
        }

        var line = JsonSerializer.Serialize(report, _options);
        lock (_lock)
        {
            File.AppendAllText(_queueFile, line + Environment.NewLine, Encoding.UTF8);
        }
    }

    public async Task FlushAsync(ApiClient apiClient, string token, Logger logger, CancellationToken ct)
    {
        if (apiClient == null)
        {
            return;
        }
        if (string.IsNullOrWhiteSpace(token))
        {
            return;
        }

        string[] lines;
        lock (_lock)
        {
            if (!File.Exists(_queueFile))
            {
                return;
            }
            lines = File.ReadAllLines(_queueFile, Encoding.UTF8);
        }

        if (lines.Length == 0)
        {
            return;
        }

        var remaining = new List<string>(lines.Length);
        for (var i = 0; i < lines.Length; i++)
        {
            ct.ThrowIfCancellationRequested();

            var line = lines[i];
            if (string.IsNullOrWhiteSpace(line))
            {
                continue;
            }

            try
            {
                var report = JsonSerializer.Deserialize<ClientErrorReportRequest>(line, _options);
                if (report == null)
                {
                    continue;
                }

                await apiClient.ReportErrorAsync(report, token, ct).ConfigureAwait(false);
            }
            catch (ApiException ex) when ((int)ex.StatusCode == 404 || (int)ex.StatusCode == 405)
            {
                logger.Warn("服务端暂不支持错误上报接口，已跳过刷新。");
                remaining.Add(line);
                for (var j = i + 1; j < lines.Length; j++)
                {
                    if (!string.IsNullOrWhiteSpace(lines[j]))
                    {
                        remaining.Add(lines[j]);
                    }
                }
                break;
            }
            catch (Exception ex)
            {
                logger.Warn($"刷新错误上报队列失败: {ex.Message}");
                remaining.Add(line);
            }
        }

        lock (_lock)
        {
            if (remaining.Count == 0)
            {
                try
                {
                    File.Delete(_queueFile);
                }
                catch
                {
                    // ignore
                }
                return;
            }

            File.WriteAllLines(_queueFile, remaining, Encoding.UTF8);
        }
    }
}
