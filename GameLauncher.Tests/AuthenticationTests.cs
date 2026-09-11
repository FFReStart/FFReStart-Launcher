using System.Security.Cryptography;
using System.Security.Principal;
using System.Text;
using System.Text.Json;
using GameLauncher.Authentication;

namespace GameLauncher.Tests;

public sealed class AuthenticationTests : IDisposable
{
    private readonly string temporaryDirectory = Path.Combine(Path.GetTempPath(), "FFReStartLauncherTests", Guid.NewGuid().ToString("N"));

    [Fact]
    public void CanonicalAccountLoginAcceptsNormalizedUsernameAndRejectsWrongPassword()
    {
        string accountPath = CreateAccountDatabase("test-pilot", "correct horse battery staple", "save-slot-sensitive");
        var repository = new GameAccountRepository(accountPath);
        char[] correctPassword = "correct horse battery staple".ToCharArray();
        char[] wrongPassword = "incorrect-password".ToCharArray();
        try
        {
            AuthenticationResult accepted = repository.Authenticate(" TEST-PILOT ", correctPassword);
            AuthenticationResult rejected = repository.Authenticate("test-pilot", wrongPassword);
            Assert.True(accepted.Success);
            Assert.Equal("test-pilot", accepted.Username);
            Assert.Equal(AuthenticationFailure.InvalidCredentials, rejected.Failure);
        }
        finally
        {
            PasswordBuffer.Clear(correctPassword);
            PasswordBuffer.Clear(wrongPassword);
        }
    }

    [Fact]
    public void TicketIsOpaqueSignedAndExpires()
    {
        byte[] secret = RandomNumberGenerator.GetBytes(32);
        var clock = new TestTimeProvider(new DateTimeOffset(2026, 9, 11, 12, 0, 0, TimeSpan.Zero));
        var tickets = new AuthTicketService(clock);
        string ticket = tickets.Create(secret);
        Assert.DoesNotContain("test-pilot", ticket, StringComparison.OrdinalIgnoreCase);
        Assert.True(tickets.Validate(ticket, secret));
        Assert.False(tickets.Validate(ticket + "x", secret));
        clock.UtcNow = clock.UtcNow.Add(AuthTicketService.TicketLifetime).AddSeconds(1);
        Assert.False(tickets.Validate(ticket, secret));
        CryptographicOperations.ZeroMemory(secret);
    }

    [Fact]
    public void RememberedStateIsDpapiProtectedAndLogoutDeletesIt()
    {
        Directory.CreateDirectory(temporaryDirectory);
        string statePath = Path.Combine(temporaryDirectory, "auth-session.dat");
        string username = "security-user-427";
        string passwordMarker = "password-marker-831";
        string tokenMarker = "token-marker-619";
        string saveMarker = "save-marker-244";
        byte[] secret = SHA256.HashData(Encoding.UTF8.GetBytes(passwordMarker));
        var store = new SecureSessionStore(statePath, new DpapiDataProtector());
        using (var session = new AuthenticatedSession(username, secret))
        {
            store.Save(session);
        }

        string persistedText = Encoding.UTF8.GetString(File.ReadAllBytes(statePath));
        Assert.DoesNotContain(username, persistedText, StringComparison.Ordinal);
        Assert.DoesNotContain(passwordMarker, persistedText, StringComparison.Ordinal);
        Assert.DoesNotContain(tokenMarker, persistedText, StringComparison.Ordinal);
        Assert.DoesNotContain(saveMarker, persistedText, StringComparison.Ordinal);
        Assert.DoesNotContain(Convert.ToBase64String(SHA256.HashData(Encoding.UTF8.GetBytes(passwordMarker))), persistedText, StringComparison.Ordinal);
        Assert.True(new FileInfo(statePath).GetAccessControl().AreAccessRulesProtected);
        Assert.Equal(WindowsIdentity.GetCurrent().User, new FileInfo(statePath).GetAccessControl().GetOwner(typeof(SecurityIdentifier)));
        store.Delete();
        Assert.False(File.Exists(statePath));
    }

    [Fact]
    public void UncheckingRememberKeepsSessionButDeletesCredential()
    {
        string accountPath = CreateAccountDatabase("remember-user", "remember-password", null);
        string statePath = Path.Combine(temporaryDirectory, "auth-session.dat");
        using var service = CreateService(accountPath, statePath);
        char[] password = "remember-password".ToCharArray();
        try
        {
            Assert.True(service.Login("remember-user", password, remember: true).Success);
            Assert.True(File.Exists(statePath));
            service.ForgetRememberedSession();
            Assert.True(service.IsAuthenticated);
            Assert.False(File.Exists(statePath));
        }
        finally { PasswordBuffer.Clear(password); }
    }

    [Fact]
    public void LogoutClearsMemorySessionAndRememberedCredential()
    {
        string accountPath = CreateAccountDatabase("logout-user", "logout-password", null);
        string statePath = Path.Combine(temporaryDirectory, "auth-session.dat");
        using var service = CreateService(accountPath, statePath);
        char[] password = "logout-password".ToCharArray();
        try
        {
            Assert.True(service.Login("logout-user", password, remember: true).Success);
            service.Logout();
            Assert.False(service.IsAuthenticated);
            Assert.False(File.Exists(statePath));
        }
        finally { PasswordBuffer.Clear(password); }
    }

    [Fact]
    public void AuthFailureClearsRememberedCredential()
    {
        string accountPath = CreateAccountDatabase("remember-user", "remember-password", null);
        string statePath = Path.Combine(temporaryDirectory, "auth-session.dat");
        using var service = CreateService(accountPath, statePath);
        char[] correct = "remember-password".ToCharArray();
        char[] wrong = "wrong-password".ToCharArray();
        try
        {
            Assert.True(service.Login("remember-user", correct, remember: true).Success);
            Assert.False(service.Login("remember-user", wrong, remember: true).Success);
            Assert.False(File.Exists(statePath));
        }
        finally { PasswordBuffer.Clear(correct); PasswordBuffer.Clear(wrong); }
    }

    [Fact]
    public void PasswordChangeInvalidatesAndDeletesRememberedSession()
    {
        string accountPath = CreateAccountDatabase("changed-user", "first-password", null);
        string statePath = Path.Combine(temporaryDirectory, "auth-session.dat");
        using (var service = CreateService(accountPath, statePath))
        {
            char[] password = "first-password".ToCharArray();
            try { Assert.True(service.Login("changed-user", password, remember: true).Success); }
            finally { PasswordBuffer.Clear(password); }
        }
        CreateAccountDatabase("changed-user", "second-password", null, accountPath);
        using var restored = CreateService(accountPath, statePath);
        Assert.False(restored.RestoreSession().Success);
        Assert.False(File.Exists(statePath));
    }

    [Fact]
    public void AccountRemovalInvalidatesAndDeletesRememberedSession()
    {
        string accountPath = CreateAccountDatabase("removed-user", "removed-password", null);
        string statePath = Path.Combine(temporaryDirectory, "auth-session.dat");
        using (var service = CreateService(accountPath, statePath))
        {
            char[] password = "removed-password".ToCharArray();
            try { Assert.True(service.Login("removed-user", password, remember: true).Success); }
            finally { PasswordBuffer.Clear(password); }
        }

        File.WriteAllText(accountPath, "{\"Accounts\":[],\"LastUser\":null}");
        using var restored = CreateService(accountPath, statePath);
        Assert.False(restored.RestoreSession().Success);
        Assert.False(File.Exists(statePath));
    }

    [Fact]
    public void CorruptedProtectedStateIsDeletedWithoutThrowing()
    {
        Directory.CreateDirectory(temporaryDirectory);
        string statePath = Path.Combine(temporaryDirectory, "auth-session.dat");
        File.WriteAllBytes(statePath, [1, 2, 3, 4, 5]);
        var store = new SecureSessionStore(statePath, new DpapiDataProtector());
        Assert.Null(store.Load());
        Assert.False(File.Exists(statePath));
    }

    [Fact]
    public void ProtectedSettingsDoNotExposeConfiguredPath()
    {
        Directory.CreateDirectory(temporaryDirectory);
        string path = Path.Combine(temporaryDirectory, "settings.dat");
        string sensitivePath = @"C:\Users\representative-user\password-marker\token-marker\save-id-marker\FFReStart.exe";
        var store = new LauncherSettingsStore(path, new DpapiDataProtector());
        store.Save(new LauncherSettings { GameExecutablePath = sensitivePath });
        string bytesAsText = Encoding.UTF8.GetString(File.ReadAllBytes(path));
        Assert.DoesNotContain("representative-user", bytesAsText, StringComparison.OrdinalIgnoreCase);
        Assert.DoesNotContain("password-marker", bytesAsText, StringComparison.OrdinalIgnoreCase);
        Assert.DoesNotContain("token-marker", bytesAsText, StringComparison.OrdinalIgnoreCase);
        Assert.DoesNotContain("save-id-marker", bytesAsText, StringComparison.OrdinalIgnoreCase);
        Assert.Equal(sensitivePath, store.Load().GameExecutablePath);
    }

    private LauncherAuthenticationService CreateService(string accountPath, string statePath) =>
        new(new GameAccountRepository(accountPath), new SecureSessionStore(statePath, new DpapiDataProtector()), new AuthTicketService());

    private string CreateAccountDatabase(string username, string password, string? saveId, string? existingPath = null)
    {
        Directory.CreateDirectory(temporaryDirectory);
        string accountPath = existingPath ?? Path.Combine(temporaryDirectory, "accounts.json");
        byte[] salt = RandomNumberGenerator.GetBytes(16);
        byte[] hash = Rfc2898DeriveBytes.Pbkdf2(password, salt, GameAccountRepository.PasswordIterations, HashAlgorithmName.SHA256, 32);
        try
        {
            File.WriteAllText(accountPath, JsonSerializer.Serialize(new
            {
                Accounts = new[] { new { Username = username, SaveId = saveId, Salt = Convert.ToBase64String(salt), Hash = Convert.ToBase64String(hash) } },
                LastUser = (string?)null
            }));
            return accountPath;
        }
        finally { CryptographicOperations.ZeroMemory(salt); CryptographicOperations.ZeroMemory(hash); }
    }

    public void Dispose()
    {
        if (Directory.Exists(temporaryDirectory)) Directory.Delete(temporaryDirectory, recursive: true);
    }

    private sealed class TestTimeProvider(DateTimeOffset utcNow) : TimeProvider
    {
        public DateTimeOffset UtcNow { get; set; } = utcNow;
        public override DateTimeOffset GetUtcNow() => UtcNow;
    }
}
