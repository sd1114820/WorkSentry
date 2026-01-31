using System;
using System.IO;
using System.Security.Cryptography;
using System.Text;

namespace WorkSentry.Client;

internal sealed class TokenStore
{
    private readonly string _tokenPath;

    public TokenStore(string baseDirectory)
    {
        _tokenPath = Path.Combine(baseDirectory, "token.dat");
    }

    public string? LoadToken()
    {
        if (!File.Exists(_tokenPath))
        {
            return null;
        }

        try
        {
            var protectedBytes = File.ReadAllBytes(_tokenPath);
            var bytes = ProtectedData.Unprotect(protectedBytes, null, DataProtectionScope.CurrentUser);
            var token = Encoding.UTF8.GetString(bytes);
            return string.IsNullOrWhiteSpace(token) ? null : token;
        }
        catch
        {
            return null;
        }
    }

    public void SaveToken(string token)
    {
        var bytes = Encoding.UTF8.GetBytes(token);
        var protectedBytes = ProtectedData.Protect(bytes, null, DataProtectionScope.CurrentUser);
        File.WriteAllBytes(_tokenPath, protectedBytes);
    }

    public void ClearToken()
    {
        if (File.Exists(_tokenPath))
        {
            File.Delete(_tokenPath);
        }
    }
}
