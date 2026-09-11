namespace GameLauncher;

/// <summary>
/// Defines every path owned by a game installation. Launcher settings and
/// authentication data deliberately remain in the launcher's LocalAppData folder.
/// </summary>
public sealed class GameInstallLocation
{
    public const string BuildDirectoryName = "FFReStart-Dev-Build";
    public const string GameExecutableName = "FFReStart-Dev-Build.exe";
    public const string VersionFileName = "Version.txt";
    public const string DownloadArchiveName = "FFReStart-Dev-Build.zip";

    public GameInstallLocation(string rootDirectory)
    {
        if (string.IsNullOrWhiteSpace(rootDirectory))
            throw new ArgumentException("An install folder is required.", nameof(rootDirectory));

        RootDirectory = Normalize(rootDirectory);
    }

    public string RootDirectory { get; }
    public string VersionFilePath => Path.Combine(RootDirectory, VersionFileName);
    public string DownloadArchivePath => Path.Combine(RootDirectory, DownloadArchiveName);
    public string ExpectedGameExecutablePath =>
        Path.Combine(RootDirectory, BuildDirectoryName, GameExecutableName);

    public static GameInstallLocation FromSettings(
        string defaultRootDirectory,
        string? configuredInstallDirectory,
        string? legacyGameExecutablePath)
    {
        if (TryCreate(configuredInstallDirectory, out GameInstallLocation? configured))
            return configured!;

        string? inferredLegacyRoot = InferRootFromExecutable(legacyGameExecutablePath);
        if (TryCreate(inferredLegacyRoot, out GameInstallLocation? migrated))
            return migrated!;

        return new GameInstallLocation(defaultRootDirectory);
    }

    public static string? InferRootFromExecutable(string? executablePath)
    {
        if (!GameExecutableLocator.IsUsableGameExecutable(executablePath)) return null;

        try
        {
            string? executableDirectory = Path.GetDirectoryName(Path.GetFullPath(executablePath!));
            if (string.IsNullOrEmpty(executableDirectory)) return null;

            string directoryName = Path.GetFileName(executableDirectory.TrimEnd(Path.DirectorySeparatorChar));
            if (directoryName.StartsWith("FFReStart-", StringComparison.OrdinalIgnoreCase))
                return Directory.GetParent(executableDirectory)?.FullName ?? executableDirectory;

            return executableDirectory;
        }
        catch (Exception exception) when (exception is ArgumentException or NotSupportedException or PathTooLongException)
        {
            return null;
        }
    }

    public void EnsureWritable()
    {
        Directory.CreateDirectory(RootDirectory);
        string probePath = Path.Combine(RootDirectory, $".ffrestart-write-{Guid.NewGuid():N}.tmp");
        using var probe = new FileStream(
            probePath,
            FileMode.CreateNew,
            FileAccess.Write,
            FileShare.None,
            1,
            FileOptions.DeleteOnClose);
        probe.WriteByte(0);
    }

    private static bool TryCreate(string? path, out GameInstallLocation? location)
    {
        location = null;
        if (string.IsNullOrWhiteSpace(path)) return false;

        try
        {
            if (!Path.IsPathFullyQualified(path)) return false;
            location = new GameInstallLocation(path);
            return true;
        }
        catch (Exception exception) when (exception is ArgumentException or NotSupportedException or PathTooLongException)
        {
            return false;
        }
    }

    private static string Normalize(string path)
    {
        string fullPath = Path.GetFullPath(path);
        string? pathRoot = Path.GetPathRoot(fullPath);
        return string.Equals(fullPath, pathRoot, StringComparison.OrdinalIgnoreCase)
            ? fullPath
            : fullPath.TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar);
    }
}
