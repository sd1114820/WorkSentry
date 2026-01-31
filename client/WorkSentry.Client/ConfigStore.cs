using System;
using System.IO;
using System.Text.Json;

namespace WorkSentry.Client;

internal sealed class ConfigStore
{
    private readonly JsonSerializerOptions _options = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
        WriteIndented = true
    };

    public string BaseDirectory { get; }
    public string ConfigPath { get; }

    public ConfigStore()
    {
        BaseDirectory = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.CommonApplicationData), "WorkSentry");
        ConfigPath = Path.Combine(BaseDirectory, "config.json");
    }

    public AppConfig Load()
    {
        Directory.CreateDirectory(BaseDirectory);
        if (!File.Exists(ConfigPath))
        {
            var config = new AppConfig();
            Save(config);
            return config;
        }

        try
        {
            var json = File.ReadAllText(ConfigPath);
            var config = JsonSerializer.Deserialize<AppConfig>(json, _options) ?? new AppConfig();
            if (string.IsNullOrWhiteSpace(config.ClientVersion))
            {
                config.ClientVersion = AppConstants.ClientVersion;
            }
            return config;
        }
        catch
        {
            var config = new AppConfig();
            Save(config);
            return config;
        }
    }

    public void Save(AppConfig config)
    {
        Directory.CreateDirectory(BaseDirectory);
        config.ClientVersion = AppConstants.ClientVersion;
        var json = JsonSerializer.Serialize(config, _options);
        File.WriteAllText(ConfigPath, json);
    }
}
