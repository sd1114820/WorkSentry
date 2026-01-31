using System;
using System.Linq;

namespace WorkSentry.Client;

internal static class VersionHelper
{
    public static bool IsOutdated(string current, string latest)
    {
        if (string.IsNullOrWhiteSpace(latest))
        {
            return false;
        }
        if (string.IsNullOrWhiteSpace(current))
        {
            return true;
        }

        if (TryParseVersion(current, out var currentVer) && TryParseVersion(latest, out var latestVer))
        {
            return currentVer < latestVer;
        }

        return !string.Equals(current.Trim(), latest.Trim(), StringComparison.OrdinalIgnoreCase);
    }

    private static bool TryParseVersion(string value, out Version version)
    {
        var normalized = string.Join('.', value.Split('.', StringSplitOptions.RemoveEmptyEntries)
            .Select(part => int.TryParse(part, out var num) ? num.ToString() : "0"));
        return Version.TryParse(normalized, out version!);
    }
}
