using System.Globalization;
using System.Security.Cryptography;
using System.Text;

namespace GameLauncher.Authentication;

public sealed class AuthTicketService
{
    public static readonly TimeSpan TicketLifetime = TimeSpan.FromMinutes(5);
    private const string Version = "v1";
    private readonly TimeProvider timeProvider;

    public AuthTicketService(TimeProvider? timeProvider = null)
    {
        this.timeProvider = timeProvider ?? TimeProvider.System;
    }

    public string Create(ReadOnlySpan<byte> accountSecret)
    {
        DateTimeOffset issuedAt = timeProvider.GetUtcNow();
        long issuedAtSeconds = issuedAt.ToUnixTimeSeconds();
        long expiresAtSeconds = issuedAt.Add(TicketLifetime).ToUnixTimeSeconds();
        string nonce = Base64UrlEncode(RandomNumberGenerator.GetBytes(16));
        byte[] subjectBytes = HMACSHA256.HashData(accountSecret, Encoding.UTF8.GetBytes("FFReStart.AuthTicket.Subject.v1"));
        byte[]? signature = null;
        try
        {
            string subject = Base64UrlEncode(subjectBytes.AsSpan(0, 16));
            string payload = string.Join('.',
                Version,
                subject,
                issuedAtSeconds.ToString(CultureInfo.InvariantCulture),
                expiresAtSeconds.ToString(CultureInfo.InvariantCulture),
                nonce);
            signature = HMACSHA256.HashData(accountSecret, Encoding.UTF8.GetBytes(payload));
            return payload + "." + Base64UrlEncode(signature);
        }
        finally
        {
            CryptographicOperations.ZeroMemory(subjectBytes);
            if (signature is not null)
            {
                CryptographicOperations.ZeroMemory(signature);
            }
        }
    }

    public bool Validate(string ticket, ReadOnlySpan<byte> accountSecret)
    {
        string[] parts = ticket.Split('.');
        if (parts.Length != 6 || parts[0] != Version)
        {
            return false;
        }

        try
        {
            byte[] expectedSubjectBytes = HMACSHA256.HashData(accountSecret, Encoding.UTF8.GetBytes("FFReStart.AuthTicket.Subject.v1"));
            string expectedSubject = Base64UrlEncode(expectedSubjectBytes.AsSpan(0, 16));
            CryptographicOperations.ZeroMemory(expectedSubjectBytes);
            if (!string.Equals(parts[1], expectedSubject, StringComparison.Ordinal))
            {
                return false;
            }

            if (!long.TryParse(parts[2], NumberStyles.None, CultureInfo.InvariantCulture, out long issuedAt) ||
                !long.TryParse(parts[3], NumberStyles.None, CultureInfo.InvariantCulture, out long expiresAt) ||
                expiresAt - issuedAt != (long)TicketLifetime.TotalSeconds)
            {
                return false;
            }

            long now = timeProvider.GetUtcNow().ToUnixTimeSeconds();
            if (issuedAt > now + 30 || expiresAt <= now)
            {
                return false;
            }

            byte[] suppliedSignature = Base64UrlDecode(parts[5]);
            byte[] expectedSignature = HMACSHA256.HashData(
                accountSecret,
                Encoding.UTF8.GetBytes(string.Join('.', parts.Take(5))));
            try
            {
                return CryptographicOperations.FixedTimeEquals(suppliedSignature, expectedSignature);
            }
            finally
            {
                CryptographicOperations.ZeroMemory(suppliedSignature);
                CryptographicOperations.ZeroMemory(expectedSignature);
            }
        }
        catch (FormatException)
        {
            return false;
        }
    }

    private static string Base64UrlEncode(ReadOnlySpan<byte> value) =>
        Convert.ToBase64String(value).TrimEnd('=').Replace('+', '-').Replace('/', '_');

    private static byte[] Base64UrlDecode(string value)
    {
        string padded = value.Replace('-', '+').Replace('_', '/');
        padded = padded.PadRight(padded.Length + ((4 - padded.Length % 4) % 4), '=');
        return Convert.FromBase64String(padded);
    }
}
