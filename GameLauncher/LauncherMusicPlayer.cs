using System.IO;
using System.Reflection;
using System.Windows.Media;

namespace GameLauncher;

public static class LauncherAudioPreferences
{
    public const double DefaultVolume = 0.35;

    public static double NormalizeVolume(double volume) =>
        double.IsFinite(volume) ? Math.Clamp(volume, 0d, 1d) : DefaultVolume;
}

internal sealed class LauncherMusicPlayer : IDisposable
{
    internal const string ResourceName = "GameLauncher.Audio.LauncherMainTheme.mp3";
    private const string CachedFileName = "launcher-main-theme.mp3";

    private readonly MediaPlayer player = new();
    private bool playWhenOpened;

    public event EventHandler<ExceptionEventArgs>? PlaybackFailed;

    public LauncherMusicPlayer(string settingsFolder)
    {
        string trackPath = ExtractEmbeddedTrack(settingsFolder);
        player.MediaOpened += Player_MediaOpened;
        player.MediaEnded += Player_MediaEnded;
        player.MediaFailed += Player_MediaFailed;
        player.Open(new Uri(trackPath, UriKind.Absolute));
    }

    public double Volume
    {
        get => player.Volume;
        set => player.Volume = LauncherAudioPreferences.NormalizeVolume(value);
    }

    public bool IsMuted
    {
        get => player.IsMuted;
        set => player.IsMuted = value;
    }

    public void Play()
    {
        playWhenOpened = true;
        player.Play();
    }

    private static string ExtractEmbeddedTrack(string settingsFolder)
    {
        string mediaFolder = Path.Combine(settingsFolder, "Media");
        string destination = Path.Combine(mediaFolder, CachedFileName);
        using Stream resource = Assembly.GetExecutingAssembly().GetManifestResourceStream(ResourceName)
            ?? throw new InvalidOperationException("The embedded launcher music resource is missing.");

        Directory.CreateDirectory(mediaFolder);
        if (File.Exists(destination) && new FileInfo(destination).Length == resource.Length)
            return destination;

        string temporaryPath = Path.Combine(mediaFolder, $"{CachedFileName}.{Environment.ProcessId}.tmp");
        try
        {
            using (FileStream output = new(temporaryPath, FileMode.Create, FileAccess.Write, FileShare.None))
                resource.CopyTo(output);
            File.Move(temporaryPath, destination, overwrite: true);
        }
        finally
        {
            if (File.Exists(temporaryPath)) File.Delete(temporaryPath);
        }

        return destination;
    }

    private void Player_MediaOpened(object? sender, EventArgs e)
    {
        if (playWhenOpened) player.Play();
    }

    private void Player_MediaEnded(object? sender, EventArgs e)
    {
        player.Position = TimeSpan.Zero;
        player.Play();
    }

    private void Player_MediaFailed(object? sender, ExceptionEventArgs e) => PlaybackFailed?.Invoke(this, e);

    public void Dispose()
    {
        player.Stop();
        player.Close();
        player.MediaOpened -= Player_MediaOpened;
        player.MediaEnded -= Player_MediaEnded;
        player.MediaFailed -= Player_MediaFailed;
    }
}
