using System;
using System.Reflection;

namespace WorkSentry.Client;

internal static class AppConstants
{
    public const string AppName = "WorkSentry";

    public static string ClientVersion
    {
        get
        {
            var version = Assembly.GetExecutingAssembly().GetCustomAttribute<AssemblyInformationalVersionAttribute>()?.InformationalVersion
                ?? Assembly.GetExecutingAssembly().GetName().Version?.ToString()
                ?? "1.0.0";

            var plusIndex = version.IndexOf('+');
            if (plusIndex > 0)
            {
                version = version[..plusIndex];
            }

            return version;
        }
    }
}
