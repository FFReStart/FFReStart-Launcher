using System.Security.Cryptography;
using System.Text.Json;
using GameLauncher.Authentication;

namespace GameLauncher;

public sealed class LauncherSettingsStore
{
    private readonly ProtectedFileStore protectedFile;

    public LauncherSettingsStore(string path, IDataProtector protector)
    {
        protectedFile = new ProtectedFileStore(path, protector);
    }

    public LauncherSettings Load()
    {
        byte[]? plaintext = protectedFile.Read();
        if (plaintext is null)
        {
            return new LauncherSettings();
        }

        try
        {
            LauncherSettings? settings = JsonSerializer.Deserialize<LauncherSettings>(plaintext);
            if (settings?.Version != 1)
            {
                protectedFile.Delete();
                return new LauncherSettings();
            }

            return settings;
        }
        catch (JsonException)
        {
            protectedFile.Delete();
            return new LauncherSettings();
        }
        finally
        {
            CryptographicOperations.ZeroMemory(plaintext);
        }
    }

    public void Save(LauncherSettings settings)
    {
        byte[] plaintext = JsonSerializer.SerializeToUtf8Bytes(settings);
        try
        {
            protectedFile.Write(plaintext);
        }
        finally
        {
            CryptographicOperations.ZeroMemory(plaintext);
        }
    }
}

public sealed class LauncherSettings
{
    public int Version { get; set; } = 1;

    public string? InstallDirectory { get; set; }

    public double MusicVolume { get; set; } = LauncherAudioPreferences.DefaultVolume;

    public bool IsMusicMuted { get; set; }

    // Retained so settings written by the first auth-enabled launcher can be
    // migrated to an install root instead of silently forgetting the user's game.
    public string? GameExecutablePath { get; set; }
}
