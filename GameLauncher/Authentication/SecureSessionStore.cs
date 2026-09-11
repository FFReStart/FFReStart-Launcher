using System.Security.AccessControl;
using System.Security.Cryptography;
using System.Security.Principal;
using System.Buffers.Binary;
using System.Text;

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

            BestEffortDelete(temporaryPath);
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
            DeleteRequired(path);
            return null;
        }
        catch (UnauthorizedAccessException)
        {
            DeleteRequired(path);
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
        DeleteRequired(path);
        DeleteRequired(path + ".tmp");
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

    private static void DeleteRequired(string filePath)
    {
        if (!File.Exists(filePath)) return;
        try
        {
            File.Delete(filePath);
        }
        catch (Exception exception) when (exception is IOException or UnauthorizedAccessException)
        {
            throw new IOException("Protected launcher state could not be removed.", exception);
        }
    }

    private static void BestEffortDelete(string filePath)
    {
        try
        {
            if (File.Exists(filePath)) File.Delete(filePath);
        }
        catch (Exception exception) when (exception is IOException or UnauthorizedAccessException) { }
    }
}

public sealed class SecureSessionStore
{
    private static readonly UTF8Encoding StrictUtf8 = new(false, true);
    private readonly ProtectedFileStore protectedFile;

    public SecureSessionStore(string path, IDataProtector protector)
    {
        protectedFile = new ProtectedFileStore(path, protector);
    }

    public void Save(AuthenticatedSession session)
    {
        byte[] username = StrictUtf8.GetBytes(session.Username);
        if (username.Length is 0 or > 1024 || session.AccountSecret.Length != 32)
        {
            CryptographicOperations.ZeroMemory(username);
            throw new CryptographicException("The session payload is invalid.");
        }

        byte[] plaintext = new byte[1 + sizeof(ushort) + username.Length + session.AccountSecret.Length];
        plaintext[0] = 1;
        BinaryPrimitives.WriteUInt16BigEndian(plaintext.AsSpan(1, sizeof(ushort)), checked((ushort)username.Length));
        username.CopyTo(plaintext.AsSpan(3));
        session.AccountSecret.CopyTo(plaintext.AsSpan(3 + username.Length));

        try
        {
            protectedFile.Write(plaintext);
        }
        finally
        {
            CryptographicOperations.ZeroMemory(username);
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
            if (plaintext.Length < 1 + sizeof(ushort) + 32 || plaintext[0] != 1)
            {
                protectedFile.Delete();
                return null;
            }

            int usernameLength = BinaryPrimitives.ReadUInt16BigEndian(plaintext.AsSpan(1, sizeof(ushort)));
            if (usernameLength is 0 or > 1024 || plaintext.Length != 3 + usernameLength + 32)
            {
                protectedFile.Delete();
                return null;
            }

            string username = StrictUtf8.GetString(plaintext, 3, usernameLength);
            if (string.IsNullOrWhiteSpace(username))
            {
                protectedFile.Delete();
                return null;
            }

            byte[] accountSecret = plaintext.AsSpan(3 + usernameLength, 32).ToArray();
            return new AuthenticatedSession(username, accountSecret);
        }
        catch (Exception exception) when (exception is DecoderFallbackException or ArgumentException)
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
