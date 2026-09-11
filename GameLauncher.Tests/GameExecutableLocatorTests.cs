namespace GameLauncher.Tests;

public sealed class GameExecutableLocatorTests : IDisposable
{
    private readonly string temporaryDirectory = Path.Combine(Path.GetTempPath(), "FFReStartLocatorTests", Guid.NewGuid().ToString("N"));

    [Fact]
    public void PrefersConfiguredExecutableAndRejectsCrashHandler()
    {
        string installRoot = Path.Combine(temporaryDirectory, "Install");
        string configured = Path.Combine(temporaryDirectory, "Custom", "FFReStart.exe");
        string crashHandler = Path.Combine(installRoot, "UnityCrashHandler64.exe");
        Directory.CreateDirectory(Path.GetDirectoryName(configured)!);
        Directory.CreateDirectory(installRoot);
        File.WriteAllBytes(configured, []);
        File.WriteAllBytes(crashHandler, []);
        var locator = new GameExecutableLocator(installRoot);
        Assert.Equal(Path.GetFullPath(configured), locator.Find(configured));
        Assert.False(GameExecutableLocator.IsUsableGameExecutable(crashHandler));
    }

    [Fact]
    public void DiscoversRenamedGameBuildBelowInstallRoot()
    {
        string installRoot = Path.Combine(temporaryDirectory, "Install");
        string game = Path.Combine(installRoot, "FFReStart-New-Build", "FFReStart-New-Build.exe");
        Directory.CreateDirectory(Path.GetDirectoryName(game)!);
        File.WriteAllBytes(game, []);
        Assert.Equal(game, new GameExecutableLocator(installRoot).Find(null));
    }

    public void Dispose()
    {
        if (Directory.Exists(temporaryDirectory)) Directory.Delete(temporaryDirectory, recursive: true);
    }
}
