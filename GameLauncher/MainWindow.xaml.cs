using System;
using System.Diagnostics;
using System.IO;
using System.IO.Compression;
using System.Net.Http;
using System.Security.Cryptography;
using System.Threading;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Input;
using System.Windows.Media;
using GameLauncher.Authentication;
using Microsoft.Win32;

namespace GameLauncher
{
    internal enum LauncherStatus
    {
        Checking,
        Ready,
        OfflineReady,
        Failed,
        DownloadingGame,
        DownloadingUpdate,
        Installing
    }

    public enum LauncherLinkDestination
    {
        Community,
        Support
    }

    public static class LauncherLinks
    {
        public const string CommunityDiscord = "https://discord.gg/Q5je3v9Bjg";
        public const string Support = "https://discord.gg/VNVjmPn2Fn";

        public static string GetUrl(LauncherLinkDestination destination) => destination switch
        {
            LauncherLinkDestination.Community => CommunityDiscord,
            LauncherLinkDestination.Support => Support,
            _ => throw new ArgumentOutOfRangeException(nameof(destination))
        };

        public static ProcessStartInfo CreateStartInfo(LauncherLinkDestination destination) =>
            new ProcessStartInfo(GetUrl(destination)) { UseShellExecute = true };
    }

    public partial class MainWindow : Window
    {
        private readonly string rootPath;
        private readonly string versionFile;
        private readonly string gameZip;
        private readonly string settingsFolder;
        private readonly LauncherSettingsStore launcherSettingsStore;
        private readonly GameExecutableLocator gameExecutableLocator;
        private readonly LauncherAuthenticationService authenticationService;
        private LauncherSettings launcherSettings;
        private static readonly HttpClient HttpClient = new HttpClient { Timeout = TimeSpan.FromMinutes(10) };
        private readonly CancellationTokenSource shutdown = new CancellationTokenSource();
        private LauncherStatus status;

        private const string VersionUrl = "https://github.com/FFReStart/FFReStart_Builds/releases/latest/download/version.txt";
        private const string GameZipUrl = "https://github.com/FFReStart/FFReStart_Builds/releases/latest/download/FFReStart-Dev-Build.zip";
        private const string ApplicationFolderName = "FFReStart";

        internal LauncherStatus Status
        {
            get => status;
            set
            {
                status = value;
                ApplyStatusPresentation(value);
            }
        }

        public MainWindow()
        {
            InitializeComponent();

            rootPath = Path.Combine(
                Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
                ApplicationFolderName
            );
            Directory.CreateDirectory(rootPath);

            versionFile = Path.Combine(rootPath, "Version.txt");
            gameZip = Path.Combine(rootPath, "FFReStart-Dev-Build.zip");
            settingsFolder = Path.Combine(rootPath, "Launcher");
            var dataProtector = new DpapiDataProtector();
            launcherSettingsStore = new LauncherSettingsStore(Path.Combine(settingsFolder, "settings.dat"), dataProtector);
            try
            {
                launcherSettings = launcherSettingsStore.Load();
            }
            catch (Exception exception) when (exception is IOException or UnauthorizedAccessException or CryptographicException or PlatformNotSupportedException)
            {
                launcherSettings = new LauncherSettings();
                AuthStatusText.Text = "Protected launcher settings could not be recovered. Defaults are in use.";
            }
            gameExecutableLocator = new GameExecutableLocator(rootPath);
            string sessionPath = Path.Combine(settingsFolder, "auth-session.dat");
            bool hadRememberedSession = File.Exists(sessionPath);
            authenticationService = new LauncherAuthenticationService(
                new GameAccountRepository(GameAccountRepository.GetDefaultAccountDatabasePath()),
                new SecureSessionStore(sessionPath, dataProtector),
                new AuthTicketService());

            AuthenticationResult? restored = null;
            try
            {
                restored = authenticationService.RestoreSession();
            }
            catch (Exception exception) when (exception is IOException or UnauthorizedAccessException or CryptographicException or PlatformNotSupportedException)
            {
                AuthStatusText.Text = "The protected remembered session could not be recovered or removed. Close other launcher instances and sign in again.";
            }

            if (hadRememberedSession && restored is { Success: false })
            {
                AuthStatusText.Text = restored.Failure switch
                {
                    AuthenticationFailure.AccountStoreUnavailable => "Your local game account could not be checked. Sign in after it is available.",
                    AuthenticationFailure.AccountStoreInvalid => "The local account database could not be read. Open the game to repair it.",
                    _ => "Your remembered session is no longer valid. Please sign in again."
                };
            }

            RefreshAuthenticationUi();
            RefreshGameLocationText();
            Status = LauncherStatus.Checking;
        }

        private async void Window_ContentRendered(object sender, EventArgs e)
        {
            if (Environment.GetCommandLineArgs().Length > 1 &&
                Array.Exists(Environment.GetCommandLineArgs(), argument =>
                    string.Equals(argument, "--preview", StringComparison.OrdinalIgnoreCase)))
            {
                VersionText.Text = "PREVIEW";
                Status = LauncherStatus.Ready;
                StatusDetailText.Text = "Preview mode — network and game launch are disabled.";
                PlayButton.IsEnabled = false;
                return;
            }

            await CheckForUpdatesAsync();
        }

        private void Window_Closed(object sender, EventArgs e)
        {
            shutdown.Cancel();
            shutdown.Dispose();
            authenticationService.Dispose();
        }

        private void ApplyStatusPresentation(LauncherStatus currentStatus)
        {
            DownloadProgress.Visibility = Visibility.Collapsed;
            DownloadProgress.IsIndeterminate = false;
            PlayButton.IsEnabled = false;

            switch (currentStatus)
            {
                case LauncherStatus.Checking:
                    StatusTitleText.Text = "CHECKING GAME FILES";
                    StatusDetailText.Text = "Contacting the update service…";
                    StatusDot.Fill = FindBrush("CyanBrush");
                    PlayButton.Content = "CHECKING…";
                    break;

                case LauncherStatus.Ready:
                    StatusTitleText.Text = "READY FOR DEPLOYMENT";
                    StatusDetailText.Text = "Your game is current. Jump back into the fight.";
                    StatusDot.Fill = FindBrush("NanoGreenBrush");
                    PlayButton.Content = authenticationService.IsAuthenticated ? "PLAY FFReSTART" : "SIGN IN TO PLAY";
                    PlayButton.IsEnabled = true;
                    break;

                case LauncherStatus.OfflineReady:
                    StatusTitleText.Text = "OFFLINE MODE AVAILABLE";
                    StatusDetailText.Text = "The update service is unavailable. You can launch the installed game or try again later.";
                    StatusDot.Fill = FindBrush("CyanBrush");
                    PlayButton.Content = authenticationService.IsAuthenticated ? "PLAY INSTALLED BUILD" : "SIGN IN TO PLAY";
                    PlayButton.IsEnabled = true;
                    break;

                case LauncherStatus.Failed:
                    StatusTitleText.Text = "UPDATE NEEDS ATTENTION";
                    StatusDot.Fill = FindBrush("DangerBrush");
                    PlayButton.Content = "TRY AGAIN";
                    PlayButton.IsEnabled = true;
                    break;

                case LauncherStatus.DownloadingGame:
                    ShowDownloadState("INSTALLING GAME", "Preparing your first deployment…", "INSTALLING…");
                    break;

                case LauncherStatus.DownloadingUpdate:
                    ShowDownloadState("DOWNLOADING UPDATE", "Retrieving the latest mission files…", "UPDATING…");
                    break;

                case LauncherStatus.Installing:
                    StatusTitleText.Text = "APPLYING GAME FILES";
                    StatusDetailText.Text = "Finishing the installation. Keep the launcher open.";
                    StatusDot.Fill = FindBrush("NanoGreenBrush");
                    DownloadProgress.Visibility = Visibility.Visible;
                    DownloadProgress.IsIndeterminate = true;
                    PlayButton.Content = "FINALIZING…";
                    break;
            }
        }

        private Brush FindBrush(string resourceName) => (Brush)FindResource(resourceName);

        private void ShowDownloadState(string title, string detail, string buttonText)
        {
            StatusTitleText.Text = title;
            StatusDetailText.Text = detail;
            StatusDot.Fill = FindBrush("NanoGreenBrush");
            DownloadProgress.Value = 0;
            DownloadProgress.Visibility = Visibility.Visible;
            PlayButton.Content = buttonText;
        }

        private async Task CheckForUpdatesAsync()
        {
            Status = LauncherStatus.Checking;

            try
            {
                LauncherVersion onlineVersion = await GetOnlineVersionAsync(shutdown.Token);

                if (File.Exists(versionFile))
                {
                    LauncherVersion localVersion = new LauncherVersion(File.ReadAllText(versionFile).Trim());
                    VersionText.Text = $"v{localVersion}";

                    if (onlineVersion.IsDifferentThan(localVersion))
                    {
                        await InstallGameFilesAsync(true, onlineVersion, shutdown.Token);
                    }
                    else
                    {
                        Status = LauncherStatus.Ready;
                    }
                }
                else
                {
                    VersionText.Text = "NEW INSTALL";
                    await InstallGameFilesAsync(false, onlineVersion, shutdown.Token);
                }
            }
            catch (OperationCanceledException) when (shutdown.IsCancellationRequested)
            {
                // Closing the launcher intentionally cancels in-flight network and file work.
            }
            catch (Exception)
            {
                if (gameExecutableLocator.Find(launcherSettings.GameExecutablePath) is not null)
                {
                    Status = LauncherStatus.OfflineReady;
                }
                else
                {
                    ShowFailure("We couldn't reach the update service and no installed game was found. Check your connection, then try again.");
                }
            }
        }

        private static async Task<LauncherVersion> GetOnlineVersionAsync(CancellationToken cancellationToken)
        {
            string versionText = (await HttpClient.GetStringAsync(VersionUrl, cancellationToken)).Trim();
            return new LauncherVersion(versionText);
        }

        private async Task InstallGameFilesAsync(bool isUpdate, LauncherVersion onlineVersion, CancellationToken cancellationToken)
        {
            try
            {
                Status = isUpdate ? LauncherStatus.DownloadingUpdate : LauncherStatus.DownloadingGame;

                if (File.Exists(gameZip))
                {
                    File.Delete(gameZip);
                }

                using HttpResponseMessage response = await HttpClient.GetAsync(
                    GameZipUrl,
                    HttpCompletionOption.ResponseHeadersRead,
                    cancellationToken);
                response.EnsureSuccessStatusCode();

                long totalBytes = response.Content.Headers.ContentLength ?? -1;
                using Stream input = await response.Content.ReadAsStreamAsync(cancellationToken);
                using FileStream output = new FileStream(gameZip, FileMode.CreateNew, FileAccess.Write, FileShare.None, 81920, true);
                byte[] buffer = new byte[81920];
                long receivedBytes = 0;
                int bytesRead;

                while ((bytesRead = await input.ReadAsync(buffer, cancellationToken)) > 0)
                {
                    await output.WriteAsync(buffer.AsMemory(0, bytesRead), cancellationToken);
                    receivedBytes += bytesRead;
                    UpdateDownloadProgress(receivedBytes, totalBytes);
                }

                Status = LauncherStatus.Installing;
                await Task.Run(() => ExtractZipToDirectorySkippingUnchangedFiles(gameZip, rootPath), cancellationToken);
                File.Delete(gameZip);
                File.WriteAllText(versionFile, onlineVersion.ToString());

                VersionText.Text = $"v{onlineVersion}";
                RefreshGameLocationText();
                Status = LauncherStatus.Ready;
            }
            catch (OperationCanceledException) when (shutdown.IsCancellationRequested)
            {
                // The window is closing.
            }
            catch (Exception)
            {
                ShowFailure("The update couldn't be completed. Check your connection, then try again; existing files are preserved.");
            }
        }

        private void UpdateDownloadProgress(long receivedBytes, long totalBytes)
        {
            if (totalBytes > 0)
            {
                double percentage = receivedBytes * 100d / totalBytes;
                DownloadProgress.IsIndeterminate = false;
                DownloadProgress.Value = percentage;
                StatusDetailText.Text = $"{FormatBytes(receivedBytes)} of {FormatBytes(totalBytes)} • {percentage:0}%";
            }
            else
            {
                DownloadProgress.IsIndeterminate = true;
                StatusDetailText.Text = $"{FormatBytes(receivedBytes)} downloaded";
            }
        }

        private void ShowFailure(string playerMessage)
        {
            Status = LauncherStatus.Failed;
            StatusDetailText.Text = playerMessage;
            StatusDetailText.ToolTip = null;
        }

        private async void PlayButton_Click(object sender, RoutedEventArgs e)
        {
            if (Status is LauncherStatus.Ready or LauncherStatus.OfflineReady)
            {
                if (!authenticationService.IsAuthenticated)
                {
                    AuthStatusText.Text = "Sign in with your local game account before launching.";
                    UsernameTextBox.Focus();
                    return;
                }

                TicketResult ticket = authenticationService.CreateLaunchTicket();
                if (!ticket.Success || string.IsNullOrEmpty(ticket.Token))
                {
                    AuthStatusText.Text = ticket.ErrorMessage ?? "Your session could not be validated. Please sign in again.";
                    RefreshAuthenticationUi();
                    return;
                }

                string? gameExecutable = gameExecutableLocator.Find(launcherSettings.GameExecutablePath);
                if (string.IsNullOrEmpty(gameExecutable))
                {
                    ShowFailure("The game executable is missing. Repair the installation or choose its location below.");
                    return;
                }

                ProcessStartInfo startInfo = GameLaunchCommand.Create(gameExecutable, ticket.Token);
                try
                {
                    Process.Start(startInfo);
                    startInfo.ArgumentList.Clear();
                    Close();
                }
                catch
                {
                    startInfo.ArgumentList.Clear();
                    ShowFailure("Windows could not start the selected game executable. Check the game location and try again.");
                }
            }
            else if (Status == LauncherStatus.Failed)
            {
                await CheckForUpdatesAsync();
            }
        }

        private async void LoginButton_Click(object sender, RoutedEventArgs e) => await SignInAsync();

        private async void LoginField_KeyDown(object sender, KeyEventArgs e)
        {
            if (e.Key != Key.Enter) return;
            e.Handled = true;
            await SignInAsync();
        }

        private async Task SignInAsync()
        {
            string username = UsernameTextBox.Text;
            using var securePassword = PasswordInput.SecurePassword;
            if (string.IsNullOrWhiteSpace(username) || securePassword.Length == 0)
            {
                authenticationService.ForgetRememberedSession();
                PasswordInput.Clear();
                AuthStatusText.Text = "Enter your username and password.";
                return;
            }

            char[] password = PasswordBuffer.CopyFrom(securePassword);
            PasswordInput.Clear();
            SetAuthenticationControlsEnabled(false);
            AuthStatusText.Text = "Signing in…";
            try
            {
                bool remember = RememberSessionCheckBox.IsChecked == true;
                AuthenticationResult result = await Task.Run(() => authenticationService.Login(username, password, remember));
                AuthStatusText.Text = result.Success ? string.Empty : GetAuthenticationError(result.Failure);
            }
            catch (Exception exception) when (exception is IOException or UnauthorizedAccessException or CryptographicException or PlatformNotSupportedException)
            {
                AuthStatusText.Text = "The session could not be protected for this Windows user. Nothing was remembered.";
            }
            finally
            {
                PasswordBuffer.Clear(password);
                PasswordInput.Clear();
                SetAuthenticationControlsEnabled(true);
                RefreshAuthenticationUi();
            }
        }

        private void RememberSessionCheckBox_Unchecked(object sender, RoutedEventArgs e)
        {
            try
            {
                authenticationService?.ForgetRememberedSession();
            }
            catch (IOException)
            {
                AuthStatusText.Text = "The remembered session could not be removed. Close other launcher instances and try again.";
            }
        }

        private void LogoutButton_Click(object sender, RoutedEventArgs e)
        {
            try
            {
                authenticationService.Logout();
            }
            catch (IOException)
            {
                AuthStatusText.Text = "The remembered session could not be removed. Close other launcher instances and try again.";
                RefreshAuthenticationUi();
                return;
            }
            UsernameTextBox.Clear();
            PasswordInput.Clear();
            RememberSessionCheckBox.IsChecked = false;
            AuthStatusText.Text = "Signed out. Your remembered session was removed.";
            RefreshAuthenticationUi();
            UsernameTextBox.Focus();
        }

        private void RefreshAuthenticationUi()
        {
            bool signedIn = authenticationService.IsAuthenticated;
            GuestAccountHeader.Visibility = signedIn ? Visibility.Collapsed : Visibility.Visible;
            SignedInAccountHeader.Visibility = signedIn ? Visibility.Visible : Visibility.Collapsed;
            AuthenticationPanel.Visibility = signedIn ? Visibility.Collapsed : Visibility.Visible;
            SignedInUsernameText.Text = signedIn ? authenticationService.CurrentUsername : string.Empty;
            ApplyPlayAuthenticationState();
        }

        private void ApplyPlayAuthenticationState()
        {
            if (Status == LauncherStatus.Ready)
                PlayButton.Content = authenticationService.IsAuthenticated ? "PLAY FFReSTART" : "SIGN IN TO PLAY";
            else if (Status == LauncherStatus.OfflineReady)
                PlayButton.Content = authenticationService.IsAuthenticated ? "PLAY INSTALLED BUILD" : "SIGN IN TO PLAY";
        }

        private void SetAuthenticationControlsEnabled(bool enabled)
        {
            UsernameTextBox.IsEnabled = enabled;
            PasswordInput.IsEnabled = enabled;
            RememberSessionCheckBox.IsEnabled = enabled;
            LoginButton.IsEnabled = enabled;
            LoginButton.Content = enabled ? "SIGN _IN" : "SIGNING IN…";
        }

        private static string GetAuthenticationError(AuthenticationFailure failure) => failure switch
        {
            AuthenticationFailure.InvalidCredentials => "That username or password was not accepted.",
            AuthenticationFailure.AccountStoreUnavailable => "No local game account was found. Open the game and create an account first.",
            AuthenticationFailure.AccountStoreInvalid => "The local account database could not be read. Open the game to repair it.",
            _ => "Your session is no longer valid. Please sign in again."
        };

        private void GameLocationButton_Click(object sender, RoutedEventArgs e)
        {
            var dialog = new OpenFileDialog
            {
                Title = "Select the FFReStart game executable",
                Filter = "Windows applications (*.exe)|*.exe",
                CheckFileExists = true,
                Multiselect = false
            };

            string? current = gameExecutableLocator.Find(launcherSettings.GameExecutablePath);
            if (!string.IsNullOrEmpty(current))
            {
                dialog.InitialDirectory = Path.GetDirectoryName(current);
                dialog.FileName = Path.GetFileName(current);
            }

            if (dialog.ShowDialog(this) != true) return;
            if (!GameExecutableLocator.IsUsableGameExecutable(dialog.FileName))
            {
                AuthStatusText.Text = "Select the main FFReStart game executable, not a launcher or crash handler.";
                return;
            }

            launcherSettings.GameExecutablePath = Path.GetFullPath(dialog.FileName);
            try
            {
                launcherSettingsStore.Save(launcherSettings);
                RefreshGameLocationText();
                AuthStatusText.Text = "Game location updated.";
                if (Status == LauncherStatus.Failed)
                    Status = LauncherStatus.OfflineReady;
            }
            catch (Exception exception) when (exception is IOException or UnauthorizedAccessException or CryptographicException or PlatformNotSupportedException)
            {
                launcherSettings = new LauncherSettings();
                AuthStatusText.Text = "The game location could not be stored securely, so it was not remembered.";
            }
        }

        private void RefreshGameLocationText()
        {
            string displayPath = gameExecutableLocator.Find(launcherSettings.GameExecutablePath) ?? rootPath;
            InstallPathText.Text = displayPath;
            InstallPathText.ToolTip = displayPath;
        }

        private void DiscordButton_Click(object sender, RoutedEventArgs e) =>
            OpenExternalLink(LauncherLinkDestination.Community, "community Discord");

        private void SupportButton_Click(object sender, RoutedEventArgs e) =>
            OpenExternalLink(LauncherLinkDestination.Support, "support server");

        private void GameFilesButton_Click(object sender, RoutedEventArgs e)
        {
            try
            {
                Directory.CreateDirectory(rootPath);
                Process.Start(new ProcessStartInfo(rootPath) { UseShellExecute = true });
            }
            catch (Exception)
            {
                ShowFailure("Windows couldn't open the game files folder.");
            }
        }

        private void OpenExternalLink(LauncherLinkDestination destination, string destinationName)
        {
            try
            {
                Process.Start(LauncherLinks.CreateStartInfo(destination));
            }
            catch (Exception)
            {
                ShowFailure($"Windows couldn't open the {destinationName} in your default browser.");
            }
        }

        private static string FormatBytes(long bytes)
        {
            if (bytes < 0) return "unknown size";
            if (bytes >= 1024L * 1024L * 1024L) return $"{bytes / (1024d * 1024d * 1024d):0.0} GB";
            if (bytes >= 1024L * 1024L) return $"{bytes / (1024d * 1024d):0.0} MB";
            return $"{bytes / 1024d:0.0} KB";
        }

        private static void ExtractZipToDirectorySkippingUnchangedFiles(string zipPath, string destinationDirectory)
        {
            string fullDestinationDirectory = Path.GetFullPath(destinationDirectory)
                .TrimEnd(Path.DirectorySeparatorChar) + Path.DirectorySeparatorChar;

            using (ZipArchive archive = ZipFile.OpenRead(zipPath))
            {
                foreach (ZipArchiveEntry entry in archive.Entries)
                {
                    string destinationPath = Path.GetFullPath(Path.Combine(fullDestinationDirectory, entry.FullName));
                    if (!destinationPath.StartsWith(fullDestinationDirectory, StringComparison.OrdinalIgnoreCase))
                    {
                        throw new IOException($"Blocked unsafe zip entry path: {entry.FullName}");
                    }

                    if (string.IsNullOrEmpty(entry.Name))
                    {
                        Directory.CreateDirectory(destinationPath);
                        continue;
                    }

                    string destinationFolder = Path.GetDirectoryName(destinationPath)
                        ?? throw new IOException("A game archive entry has no destination directory.");
                    if (!Directory.Exists(destinationFolder)) Directory.CreateDirectory(destinationFolder);
                    if (File.Exists(destinationPath) && IsSameFile(entry, destinationPath)) continue;
                    ExtractEntryToFile(entry, destinationPath);
                }
            }
        }

        private static bool IsSameFile(ZipArchiveEntry entry, string existingFilePath)
        {
            if (new FileInfo(existingFilePath).Length != entry.Length) return false;
            return string.Equals(GetFileHash(existingFilePath), GetZipEntryHash(entry), StringComparison.OrdinalIgnoreCase);
        }

        private static string GetFileHash(string filePath)
        {
            using (SHA256 sha256 = SHA256.Create())
            using (FileStream stream = File.OpenRead(filePath))
            {
                return BitConverter.ToString(sha256.ComputeHash(stream)).Replace("-", "");
            }
        }

        private static string GetZipEntryHash(ZipArchiveEntry entry)
        {
            using (SHA256 sha256 = SHA256.Create())
            using (Stream stream = entry.Open())
            {
                return BitConverter.ToString(sha256.ComputeHash(stream)).Replace("-", "");
            }
        }

        private static void ExtractEntryToFile(ZipArchiveEntry entry, string destinationPath)
        {
            string tempPath = destinationPath + ".tmp";
            if (File.Exists(tempPath)) File.Delete(tempPath);

            using (Stream entryStream = entry.Open())
            using (FileStream fileStream = new FileStream(tempPath, FileMode.CreateNew, FileAccess.Write))
            {
                entryStream.CopyTo(fileStream);
            }

            if (File.Exists(destinationPath)) File.Delete(destinationPath);
            File.Move(tempPath, destinationPath);
        }
    }

    internal readonly struct LauncherVersion
    {
        private readonly short major;
        private readonly short minor;
        private readonly short subMinor;

        internal LauncherVersion(short major, short minor, short subMinor)
        {
            this.major = major;
            this.minor = minor;
            this.subMinor = subMinor;
        }

        internal LauncherVersion(string version)
        {
            major = 0;
            minor = 0;
            subMinor = 0;
            if (string.IsNullOrWhiteSpace(version)) return;

            string[] versionStrings = version.Trim().Split('.');
            if (versionStrings.Length != 3) return;
            short.TryParse(versionStrings[0], out major);
            short.TryParse(versionStrings[1], out minor);
            short.TryParse(versionStrings[2], out subMinor);
        }

        internal bool IsDifferentThan(LauncherVersion otherVersion) =>
            major != otherVersion.major || minor != otherVersion.minor || subMinor != otherVersion.subMinor;

        public override string ToString() => $"{major}.{minor}.{subMinor}";
    }
}
