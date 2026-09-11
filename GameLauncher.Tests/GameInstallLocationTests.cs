namespace GameLauncher.Tests;

public sealed class GameInstallLocationTests : IDisposable
{
    private readonly string temporaryDirectory = Path.Combine(
        Path.GetTempPath(),
        "FFReStartInstallLocationTests",
        Guid.NewGuid().ToString("N"));

    [Fact]
    public void CustomRootOwnsVersionDownloadExtractionAndExpectedExecutablePaths()
    {
        string customRoot = Path.Combine(temporaryDirectory, "Custom Games", "FFReStart");
        var location = new GameInstallLocation(customRoot);

        Assert.Equal(Path.GetFullPath(customRoot), location.RootDirectory);
        Assert.Equal(Path.Combine(customRoot, "Version.txt"), location.VersionFilePath);
        Assert.Equal(Path.Combine(customRoot, "FFReStart-Dev-Build.zip"), location.DownloadArchivePath);
        Assert.Equal(
            Path.Combine(customRoot, "FFReStart-Dev-Build", "FFReStart-Dev-Build.exe"),
            location.ExpectedGameExecutablePath);
    }

    [Fact]
    public void PersistedCustomRootTakesPriorityOverLegacyExecutable()
    {
        string defaultRoot = Path.Combine(temporaryDirectory, "Default");
        string customRoot = Path.Combine(temporaryDirectory, "Custom");
        string legacyExecutable = CreateGameExecutable(Path.Combine(temporaryDirectory, "Legacy", "Game.exe"));

        GameInstallLocation location = GameInstallLocation.FromSettings(defaultRoot, customRoot, legacyExecutable);

        Assert.Equal(Path.GetFullPath(customRoot), location.RootDirectory);
    }

    [Fact]
    public void LegacyBuildExecutableMigratesToItsInstallRoot()
    {
        string defaultRoot = Path.Combine(temporaryDirectory, "Default");
        string legacyRoot = Path.Combine(temporaryDirectory, "Existing Install");
        string executable = CreateGameExecutable(Path.Combine(
            legacyRoot,
            "FFReStart-Dev-Build",
            "FFReStart-Dev-Build.exe"));

        GameInstallLocation location = GameInstallLocation.FromSettings(defaultRoot, null, executable);

        Assert.Equal(Path.GetFullPath(legacyRoot), location.RootDirectory);
    }

    [Fact]
    public void MissingLegacyExecutableFallsBackToLocalAppDataDefault()
    {
        string defaultRoot = Path.Combine(temporaryDirectory, "Default");
        string missingExecutable = Path.Combine(temporaryDirectory, "Missing", "Game.exe");

        GameInstallLocation location = GameInstallLocation.FromSettings(defaultRoot, null, missingExecutable);

        Assert.Equal(Path.GetFullPath(defaultRoot), location.RootDirectory);
    }

    [Fact]
    public void RelativePersistedFolderFallsBackToLocalAppDataDefault()
    {
        string defaultRoot = Path.Combine(temporaryDirectory, "Default");

        GameInstallLocation location = GameInstallLocation.FromSettings(defaultRoot, @"relative\game", null);

        Assert.Equal(Path.GetFullPath(defaultRoot), location.RootDirectory);
    }

    [Fact]
    public void WritableCheckCreatesSelectedFolderWithoutLeavingProbeFiles()
    {
        string customRoot = Path.Combine(temporaryDirectory, "Writable");
        var location = new GameInstallLocation(customRoot);

        location.EnsureWritable();

        Assert.True(Directory.Exists(customRoot));
        Assert.Empty(Directory.EnumerateFiles(customRoot, ".ffrestart-write-*.tmp"));
    }

    [Fact]
    public void ExecutableDiscoveryUsesTheSelectedCustomRoot()
    {
        string customRoot = Path.Combine(temporaryDirectory, "Second Drive", "FFReStart");
        string expectedExecutable = CreateGameExecutable(Path.Combine(
            customRoot,
            GameInstallLocation.BuildDirectoryName,
            GameInstallLocation.GameExecutableName));

        var location = new GameInstallLocation(customRoot);
        var locator = new GameExecutableLocator(location.RootDirectory);

        Assert.Equal(expectedExecutable, locator.Find(null));
    }

    private static string CreateGameExecutable(string path)
    {
        Directory.CreateDirectory(Path.GetDirectoryName(path)!);
        File.WriteAllBytes(path, []);
        return path;
    }

    public void Dispose()
    {
        if (Directory.Exists(temporaryDirectory))
            Directory.Delete(temporaryDirectory, recursive: true);
    }
}
