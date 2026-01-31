using System;
using System.Linq;
using System.Management;
using System.Net.NetworkInformation;
using System.Security.Cryptography;
using System.Text;

namespace WorkSentry.Client;

internal static class FingerprintProvider
{
    public static string GetFingerprint(Logger logger)
    {
        var board = ReadWmi("SELECT SerialNumber FROM Win32_BaseBoard", "SerialNumber", logger);
        var cpu = ReadWmi("SELECT ProcessorId FROM Win32_Processor", "ProcessorId", logger);
        var mac = GetMacAddress();

        var raw = string.Join("|", new[] { board, cpu, mac }.Where(x => !string.IsNullOrWhiteSpace(x))).ToLowerInvariant();
        if (string.IsNullOrWhiteSpace(raw))
        {
            raw = Environment.MachineName.ToLowerInvariant();
        }

        var bytes = SHA256.HashData(Encoding.UTF8.GetBytes(raw));
        return Convert.ToHexString(bytes).ToLowerInvariant();
    }

    private static string ReadWmi(string query, string property, Logger logger)
    {
        try
        {
            using var searcher = new ManagementObjectSearcher(query);
            foreach (var obj in searcher.Get().Cast<ManagementObject>())
            {
                var value = obj[property]?.ToString();
                if (!string.IsNullOrWhiteSpace(value))
                {
                    return value.Trim();
                }
            }
        }
        catch (Exception ex)
        {
            logger.Warn($"读取硬件指纹失败: {ex.Message}");
        }

        return string.Empty;
    }

    private static string GetMacAddress()
    {
        try
        {
            var network = NetworkInterface.GetAllNetworkInterfaces()
                .FirstOrDefault(n => n.OperationalStatus == OperationalStatus.Up && n.NetworkInterfaceType != NetworkInterfaceType.Loopback);
            var address = network?.GetPhysicalAddress();
            return address == null ? string.Empty : address.ToString();
        }
        catch
        {
            return string.Empty;
        }
    }
}
