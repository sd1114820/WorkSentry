using System;
using System.Globalization;
using System.Linq;
using System.Windows;

namespace WorkSentry.Client;

internal static class LanguageService
{
    public const string Auto = "auto";
    public const string ZhCn = "zh-CN";
    public const string EnUs = "en-US";
    public const string ViVn = "vi-VN";

    public static event Action? LanguageChanged;

    public static string CurrentLanguage { get; private set; } = ZhCn;

    public static string ResolveLanguage(AppConfig config)
    {
        var overrideCode = (config.LanguageOverride ?? string.Empty).Trim();
        if (!string.IsNullOrWhiteSpace(overrideCode) && !string.Equals(overrideCode, Auto, StringComparison.OrdinalIgnoreCase))
        {
            return Normalize(overrideCode);
        }
        return DetectSystemLanguage();
    }

    public static string DetectSystemLanguage()
    {
        try
        {
            var name = CultureInfo.CurrentUICulture.Name;
            if (name.StartsWith("vi", StringComparison.OrdinalIgnoreCase))
            {
                return ViVn;
            }
            if (name.StartsWith("en", StringComparison.OrdinalIgnoreCase))
            {
                return EnUs;
            }
            if (name.StartsWith("zh", StringComparison.OrdinalIgnoreCase))
            {
                return ZhCn;
            }
        }
        catch
        {
            // ignore
        }
        return ZhCn;
    }

    public static void ApplyLanguage(string languageCode)
    {
        var resolved = Normalize(languageCode);
        if (string.Equals(CurrentLanguage, resolved, StringComparison.OrdinalIgnoreCase) && HasLanguageDictionary())
        {
            return;
        }

        var dict = new ResourceDictionary
        {
            Source = new Uri($"Resources/Strings.{resolved}.xaml", UriKind.Relative)
        };

        var merged = System.Windows.Application.Current.Resources.MergedDictionaries;
        var existing = merged.FirstOrDefault(d => d.Source != null && d.Source.OriginalString.StartsWith("Resources/Strings.", StringComparison.OrdinalIgnoreCase));
        if (existing != null)
        {
            var index = merged.IndexOf(existing);
            merged.RemoveAt(index);
            merged.Insert(index, dict);
        }
        else
        {
            merged.Add(dict);
        }

        CurrentLanguage = resolved;
        LanguageChanged?.Invoke();
    }

    public static string GetString(string key)
    {
        if (System.Windows.Application.Current == null)
        {
            return key;
        }
        return System.Windows.Application.Current.TryFindResource(key) as string ?? key;
    }

    public static string Format(string key, params object[] args)
    {
        var template = GetString(key);
        try
        {
            return string.Format(GetCulture(CurrentLanguage), template, args);
        }
        catch
        {
            return template;
        }
    }

    public static string GetStatusDisplay(string statusToken)
    {
        return statusToken switch
        {
            "已上班" => GetString("StatusWorking"),
            "待上班" => GetString("StatusIdle"),
            "已下班" => GetString("StatusOff"),
            "连接中" => GetString("StatusConnecting"),
            "网络异常" => GetString("StatusNetwork"),
            "需要更新" => GetString("StatusNeedUpdate"),
            "配置已保存" => GetString("StatusConfigSaved"),
            _ => statusToken
        };
    }

    public static string TranslateUpdateStage(string stage)
    {
        return stage switch
        {
            "连接中" => GetString("UpdateStageConnecting"),
            "下载中" => GetString("UpdateStageDownloading"),
            "下载完成" => GetString("UpdateStageDownloaded"),
            "解压中" => GetString("UpdateStageExtracting"),
            _ => stage
        };
    }

    public static string TranslateUpdateError(string error)
    {
        if (string.IsNullOrWhiteSpace(error))
        {
            return error;
        }

        if (error.StartsWith("更新地址为空", StringComparison.OrdinalIgnoreCase))
        {
            return GetString("UpdateErrorUrlEmpty");
        }
        if (error.StartsWith("下载地址不可用", StringComparison.OrdinalIgnoreCase))
        {
            return GetString("UpdateErrorUrlInvalid");
        }
        if (error.StartsWith("下载失败:", StringComparison.OrdinalIgnoreCase))
        {
            var detail = error.Substring("下载失败:".Length).Trim();
            return string.Format(GetCulture(CurrentLanguage), GetString("UpdateErrorDownloadFailed"), detail);
        }
        if (error.StartsWith("下载内容不是更新文件", StringComparison.OrdinalIgnoreCase))
        {
            return GetString("UpdateErrorNotPackage");
        }
        if (error.StartsWith("压缩包内未找到客户端程序", StringComparison.OrdinalIgnoreCase))
        {
            return GetString("UpdateErrorExeNotFound");
        }
        if (error.StartsWith("更新文件不是可执行程序", StringComparison.OrdinalIgnoreCase))
        {
            return GetString("UpdateErrorNotExe");
        }
        if (error.StartsWith("更新文件格式不受支持", StringComparison.OrdinalIgnoreCase))
        {
            return GetString("UpdateErrorUnsupported");
        }

        return error;
    }

    private static string Normalize(string code)
    {
        if (string.IsNullOrWhiteSpace(code) || string.Equals(code, Auto, StringComparison.OrdinalIgnoreCase))
        {
            return DetectSystemLanguage();
        }

        if (code.StartsWith("vi", StringComparison.OrdinalIgnoreCase))
        {
            return ViVn;
        }
        if (code.StartsWith("en", StringComparison.OrdinalIgnoreCase))
        {
            return EnUs;
        }
        if (code.StartsWith("zh", StringComparison.OrdinalIgnoreCase))
        {
            return ZhCn;
        }

        return ZhCn;
    }

    private static CultureInfo GetCulture(string code)
    {
        try
        {
            return CultureInfo.GetCultureInfo(code);
        }
        catch
        {
            return CultureInfo.InvariantCulture;
        }
    }

    private static bool HasLanguageDictionary()
    {
        if (System.Windows.Application.Current == null)
        {
            return false;
        }
        return System.Windows.Application.Current.Resources.MergedDictionaries.Any(d => d.Source != null && d.Source.OriginalString.StartsWith("Resources/Strings.", StringComparison.OrdinalIgnoreCase));
    }
}

