using System;
using System.Reflection;

namespace WorkSentry.Client;

internal static class AppConstants
{
    public const string AppName = "WorkSentry";
    public static string ClientVersion =>
        Assembly.GetExecutingAssembly().GetCustomAttribute<AssemblyInformationalVersionAttribute>()?.InformationalVersion
        ?? Assembly.GetExecutingAssembly().GetName().Version?.ToString()
        ?? "1.0.0";
}
