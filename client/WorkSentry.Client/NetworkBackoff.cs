using System;

namespace WorkSentry.Client;

internal sealed class NetworkBackoff
{
    private static readonly TimeSpan[] Delays =
    {
        TimeSpan.FromSeconds(5),
        TimeSpan.FromSeconds(15),
        TimeSpan.FromSeconds(30),
        TimeSpan.FromMinutes(1),
        TimeSpan.FromMinutes(2),
        TimeSpan.FromMinutes(5)
    };

    private int _failureCount;
    private DateTime _nextAllowed = DateTime.MinValue;

    public bool CanSend => DateTime.UtcNow >= _nextAllowed;

    public void RegisterSuccess()
    {
        _failureCount = 0;
        _nextAllowed = DateTime.UtcNow;
    }

    public void RegisterFailure()
    {
        _failureCount++;
        var idx = Math.Min(_failureCount - 1, Delays.Length - 1);
        _nextAllowed = DateTime.UtcNow.Add(Delays[idx]);
    }
}
