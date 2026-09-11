namespace GameLauncher;

public sealed class GameExecutableLocator
{
    private readonly string installRoot;

    public GameExecutableLocator(string installRoot)
    {
        this.installRoot = Path.GetFullPath(installRoot);
    }

    public string? Find(string? configuredPath)
    {
        if (IsUsableGameExecutable(configuredPath))
        {
            return Path.GetFullPath(configuredPath!);
        }

        string expected = Path.Combine(installRoot, "FFReStart-Dev-Build", "FFReStart-Dev-Build.exe");
        if (IsUsableGameExecutable(expected))
        {
            return expected;
        }

        if (!Directory.Exists(installRoot))
        {
            return null;
        }

        try
        {
            return Directory.EnumerateFiles(installRoot, "*.exe", SearchOption.AllDirectories)
                .Where(IsUsableGameExecutable)
                .OrderByDescending(path => Path.GetFileNameWithoutExtension(path)
                    .Contains("FFReStart", StringComparison.OrdinalIgnoreCase))
                .ThenBy(path => path.Count(character => character == Path.DirectorySeparatorChar))
                .ThenBy(path => path, StringComparer.OrdinalIgnoreCase)
                .FirstOrDefault();
        }
        catch (UnauthorizedAccessException)
        {
            return null;
        }
        catch (IOException)
        {
            return null;
        }
    }

    public static bool IsUsableGameExecutable(string? path)
    {
        if (string.IsNullOrWhiteSpace(path) || !File.Exists(path) ||
            !string.Equals(Path.GetExtension(path), ".exe", StringComparison.OrdinalIgnoreCase))
        {
            return false;
        }

        string fileName = Path.GetFileName(path);
        return !fileName.Contains("UnityCrashHandler", StringComparison.OrdinalIgnoreCase) &&
               !fileName.Contains("Launcher", StringComparison.OrdinalIgnoreCase);
    }
}
