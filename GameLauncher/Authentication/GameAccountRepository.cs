using System.Security.Cryptography;
using System.Text.Json;

namespace GameLauncher.Authentication;

public sealed class GameAccountRepository
{
    public const int PasswordIterations = 100_000;
    private const int SaltLength = 16;
    private const int HashLength = 32;

    private readonly string accountDatabasePath;

    public GameAccountRepository(string accountDatabasePath)
    {
        this.accountDatabasePath = Path.GetFullPath(accountDatabasePath);
    }

    public string AccountDatabasePath => accountDatabasePath;

    public static string GetDefaultAccountDatabasePath()
    {
        string localAppData = Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData);
        string localLow = Path.GetFullPath(Path.Combine(localAppData, "..", "LocalLow"));
        return Path.Combine(localLow, "FFReStart-Unity", "FFReStart-Dev-Build", "Accounts", "accounts.json");
    }

    public AuthenticationResult Authenticate(string username, char[] password)
    {
        string normalizedUsername = NormalizeUsername(username);
        if (string.IsNullOrEmpty(normalizedUsername) || password.Length == 0)
        {
            return AuthenticationResult.InvalidCredentials();
        }

        AccountDatabase? database;
        try
        {
            database = Load();
        }
        catch (IOException)
        {
            return AuthenticationResult.AccountStoreUnavailable();
        }
        catch (UnauthorizedAccessException)
        {
            return AuthenticationResult.AccountStoreUnavailable();
        }
        catch (JsonException)
        {
            return AuthenticationResult.AccountStoreInvalid();
        }

        AccountRecord? account = database.Accounts.FirstOrDefault(candidate =>
            string.Equals(candidate.Username, normalizedUsername, StringComparison.OrdinalIgnoreCase));

        if (account is null || !TryDecodeAccountSecret(account, out byte[] salt, out byte[] expectedHash))
        {
            return AuthenticationResult.InvalidCredentials();
        }

        byte[] actualHash = Rfc2898DeriveBytes.Pbkdf2(
            password.AsSpan(),
            salt,
            PasswordIterations,
            HashAlgorithmName.SHA256,
            HashLength);

        try
        {
            if (!CryptographicOperations.FixedTimeEquals(actualHash, expectedHash))
            {
                return AuthenticationResult.InvalidCredentials();
            }

            byte[] ticketKey = AuthTicketService.DeriveSigningKey(expectedHash);
            try
            {
                return AuthenticationResult.Succeeded(normalizedUsername, ticketKey);
            }
            finally
            {
                CryptographicOperations.ZeroMemory(ticketKey);
            }
        }
        finally
        {
            CryptographicOperations.ZeroMemory(actualHash);
            CryptographicOperations.ZeroMemory(salt);
            CryptographicOperations.ZeroMemory(expectedHash);
        }
    }

    public AuthenticationResult ValidateRememberedSession(string username, ReadOnlySpan<byte> accountSecret)
    {
        try
        {
            string normalizedUsername = NormalizeUsername(username);
            AccountRecord? account = Load().Accounts.FirstOrDefault(candidate =>
                string.Equals(candidate.Username, normalizedUsername, StringComparison.OrdinalIgnoreCase));

            if (account is null || !TryDecodeAccountSecret(account, out byte[] salt, out byte[] currentSecret))
            {
                return AuthenticationResult.SessionRejected();
            }

            byte[] ticketKey = AuthTicketService.DeriveSigningKey(currentSecret);
            try
            {
                return CryptographicOperations.FixedTimeEquals(ticketKey, accountSecret)
                    ? AuthenticationResult.Succeeded(normalizedUsername, ticketKey)
                    : AuthenticationResult.SessionRejected();
            }
            finally
            {
                CryptographicOperations.ZeroMemory(salt);
                CryptographicOperations.ZeroMemory(currentSecret);
                CryptographicOperations.ZeroMemory(ticketKey);
            }
        }
        catch (IOException)
        {
            return AuthenticationResult.AccountStoreUnavailable();
        }
        catch (UnauthorizedAccessException)
        {
            return AuthenticationResult.AccountStoreUnavailable();
        }
        catch (JsonException)
        {
            return AuthenticationResult.AccountStoreInvalid();
        }
    }

    private AccountDatabase Load()
    {
        if (!File.Exists(accountDatabasePath))
        {
            throw new FileNotFoundException("The local game account database was not found.", accountDatabasePath);
        }

        string json = File.ReadAllText(accountDatabasePath);
        AccountDatabase? database = JsonSerializer.Deserialize<AccountDatabase>(json, JsonOptions);
        return database ?? throw new JsonException("The local game account database was empty.");
    }

    private static bool TryDecodeAccountSecret(AccountRecord account, out byte[] salt, out byte[] hash)
    {
        salt = Array.Empty<byte>();
        hash = Array.Empty<byte>();
        try
        {
            salt = Convert.FromBase64String(account.Salt ?? string.Empty);
            hash = Convert.FromBase64String(account.Hash ?? string.Empty);
            if (salt.Length == SaltLength && hash.Length == HashLength)
            {
                return true;
            }

            CryptographicOperations.ZeroMemory(salt);
            CryptographicOperations.ZeroMemory(hash);
            salt = Array.Empty<byte>();
            hash = Array.Empty<byte>();
            return false;
        }
        catch (FormatException)
        {
            CryptographicOperations.ZeroMemory(salt);
            CryptographicOperations.ZeroMemory(hash);
            salt = Array.Empty<byte>();
            hash = Array.Empty<byte>();
            return false;
        }
    }

    private static string NormalizeUsername(string username) =>
        string.IsNullOrWhiteSpace(username) ? string.Empty : username.Trim().ToLowerInvariant();

    private static readonly JsonSerializerOptions JsonOptions = new() { PropertyNameCaseInsensitive = true };

    private sealed class AccountDatabase
    {
        public List<AccountRecord> Accounts { get; set; } = [];
    }

    private sealed class AccountRecord
    {
        public string? Username { get; set; }
        public string? Salt { get; set; }
        public string? Hash { get; set; }
    }
}

public sealed class AuthenticationResult
{
    private AuthenticationResult(bool success, string? username, byte[]? accountSecret, AuthenticationFailure failure)
    {
        Success = success;
        Username = username;
        AccountSecret = accountSecret;
        Failure = failure;
    }

    public bool Success { get; }
    public string? Username { get; }
    public byte[]? AccountSecret { get; }
    public AuthenticationFailure Failure { get; }

    public static AuthenticationResult Succeeded(string username, ReadOnlySpan<byte> accountSecret) =>
        new(true, username, accountSecret.ToArray(), AuthenticationFailure.None);

    public static AuthenticationResult InvalidCredentials() => new(false, null, null, AuthenticationFailure.InvalidCredentials);
    public static AuthenticationResult SessionRejected() => new(false, null, null, AuthenticationFailure.SessionRejected);
    public static AuthenticationResult AccountStoreUnavailable() => new(false, null, null, AuthenticationFailure.AccountStoreUnavailable);
    public static AuthenticationResult AccountStoreInvalid() => new(false, null, null, AuthenticationFailure.AccountStoreInvalid);
}

public enum AuthenticationFailure
{
    None,
    InvalidCredentials,
    SessionRejected,
    AccountStoreUnavailable,
    AccountStoreInvalid
}
