using System.Security.AccessControl;
using System.Security.Cryptography;
using System.Security.Principal;
using System.Text;
using System.Text.Json;

namespace GameLauncher.Authentication;

public interface IDataProtector
{
    byte[] Protect(byte[] plaintext);
    byte[] Unprotect(byte[] ciphertext);
}

public sealed class DpapiDataProtector : IDataProtector
{
    private static readonly byte[] Entropy = Encoding.UTF8.GetBytes("FFReStart.Launcher.ProtectedState.v1");

    public byte[] Protect(byte[] plaintext) =>
        ProtectedData.Protect(plaintext, Entropy, DataProtectionScope.CurrentUser);

    public byte[] Unprotect(byte[] ciphertext) =>
        ProtectedData.Unprotect(ciphertext, Entropy, DataProtectionScope.CurrentUser);
}

public sealed class ProtectedFileStore
{
    private readonly string path;
    private readonly IDataProtector protector;

    public ProtectedFileStore(string path, IDataProtector protector)
    {
        this.path = Path.GetFullPath(path);
        this.protector = protector;
    }

    public void Write(ReadOnlySpan<byte> value)
    {
        byte[] plaintext = value.ToArray();
        byte[]? ciphertext = null;
        string temporaryPath = path + ".tmp";
        try
        {
            ciphertext = protector.Protect(plaintext);
            string? directory = Path.GetDirectoryName(path);
            if (string.IsNullOrEmpty(directory))
            {
                throw new IOException("The protected data directory is invalid.");
            }

            Directory.CreateDirectory(directory);
            using (var stream = new FileStream(
                       temporaryPath,
                       FileMode.Create,
                       FileAccess.Write,
                       FileShare.None,
                       4096,
                       FileOptions.WriteThrough))
            {
                stream.Write(ciphertext);
                stream.Flush(flushToDisk: true);
            }

            RestrictToCurrentUser(temporaryPath);
            File.Move(temporaryPath, path, overwrite: true);
        }
        finally
        {
            CryptographicOperations.ZeroMemory(plaintext);
            if (ciphertext is not null)
            {
                CryptographicOperations.ZeroMemory(ciphertext);
            }

            TryDelete(temporaryPath);
        }
    }

    public byte[]? Read()
    {
        if (!File.Exists(path))
        {
            return null;
        }

        byte[] ciphertext;
        try
        {
            ciphertext = File.ReadAllBytes(path);
        }
        catch (IOException)
        {
            Delete();
            return null;
        }
        catch (UnauthorizedAccessException)
        {
            Delete();
            return null;
        }

        try
        {
            if (ciphertext.Length == 0)
            {
                Delete();
                return null;
            }

            return protector.Unprotect(ciphertext);
        }
        catch (Exception exception) when (exception is CryptographicException or PlatformNotSupportedException)
        {
            Delete();
            return null;
        }
        finally
        {
            CryptographicOperations.ZeroMemory(ciphertext);
        }
    }

    public void Delete()
    {
        TryDelete(path);
        TryDelete(path + ".tmp");
    }

    private static void RestrictToCurrentUser(string filePath)
    {
        if (!OperatingSystem.IsWindows())
        {
            throw new PlatformNotSupportedException("Protected launcher state requires Windows.");
        }

        SecurityIdentifier? user = WindowsIdentity.GetCurrent().User;
        if (user is null)
        {
            throw new CryptographicException("The current Windows identity is unavailable.");
        }

        var security = new FileSecurity();
        security.SetAccessRuleProtection(isProtected: true, preserveInheritance: false);
        security.SetOwner(user);
        security.AddAccessRule(new FileSystemAccessRule(user, FileSystemRights.FullControl, AccessControlType.Allow));
        new FileInfo(filePath).SetAccessControl(security);
    }

    private static void TryDelete(string filePath)
    {
        try
        {
            if (File.Exists(filePath))
            {
                File.Delete(filePath);
            }
        }
        catch (IOException)
        {
            // A locked protected blob is treated as absent and retried next startup.
        }
        catch (UnauthorizedAccessException)
        {
            // DPAPI remains the security boundary if cleanup is denied by the OS.
        }
    }
}

public sealed class SecureSessionStore
{
    private readonly ProtectedFileStore protectedFile;

    public SecureSessionStore(string path, IDataProtector protector)
    {
        protectedFile = new ProtectedFileStore(path, protector);
    }

    public void Save(AuthenticatedSession session)
    {
        byte[] plaintext = JsonSerializer.SerializeToUtf8Bytes(new StoredSession
        {
            Version = 1,
            Username = session.Username,
            AccountSecret = Convert.ToBase64String(session.AccountSecret)
        });

        try
        {
            protectedFile.Write(plaintext);
        }
        finally
        {
            CryptographicOperations.ZeroMemory(plaintext);
        }
    }

    public AuthenticatedSession? Load()
    {
        byte[]? plaintext = protectedFile.Read();
        if (plaintext is null)
        {
            return null;
        }

        try
        {
            StoredSession? stored = JsonSerializer.Deserialize<StoredSession>(plaintext);
            if (stored?.Version != 1 || string.IsNullOrWhiteSpace(stored.Username) ||
                string.IsNullOrWhiteSpace(stored.AccountSecret))
            {
                protectedFile.Delete();
                return null;
            }

            byte[] accountSecret = Convert.FromBase64String(stored.AccountSecret);
            if (accountSecret.Length != 32)
            {
                CryptographicOperations.ZeroMemory(accountSecret);
                protectedFile.Delete();
                return null;
            }

            return new AuthenticatedSession(stored.Username, accountSecret);
        }
        catch (Exception exception) when (exception is JsonException or FormatException)
        {
            protectedFile.Delete();
            return null;
        }
        finally
        {
            CryptographicOperations.ZeroMemory(plaintext);
        }
    }

    public void Delete() => protectedFile.Delete();

    private sealed class StoredSession
    {
        public int Version { get; set; }
        public string? Username { get; set; }
        public string? AccountSecret { get; set; }
    }
}

public sealed class AuthenticatedSession : IDisposable
{
    private bool disposed;

    public AuthenticatedSession(string username, byte[] accountSecret)
    {
        Username = username;
        AccountSecret = accountSecret.ToArray();
        CryptographicOperations.ZeroMemory(accountSecret);
    }

    public string Username { get; }
    public byte[] AccountSecret { get; }

    public void Dispose()
    {
        if (!disposed)
        {
            CryptographicOperations.ZeroMemory(AccountSecret);
            disposed = true;
        }
    }
}
