using System;
using System.Diagnostics;
using System.IO;
using System.IO.Compression;
using System.Net.Http;
using System.Security.Cryptography;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Input;
using GameLauncher.Authentication;
using Microsoft.Win32;

namespace GameLauncher
{
    enum LauncherStatus
    {
        ready,
        failed,
        downloadingGame,
        downloadingUpdate
    }

    /// <summary>
    /// Interaction logic for MainWindow.xaml
    /// </summary>
    public partial class MainWindow : Window
    {
        private string rootPath;
        private string versionFile;
        private string gameZip;
        private string settingsFolder;
        private string settingsFile;
        private GameExecutableLocator gameExecutableLocator;
        private LauncherSettingsStore launcherSettingsStore;
        private LauncherSettings launcherSettings;
        private LauncherAuthenticationService authenticationService;

        private const string VersionUrl = "https://github.com/FFReStart/FFReStart_Builds/releases/latest/download/version.txt";
        private const string GameZipUrl = "https://github.com/FFReStart/FFReStart_Builds/releases/latest/download/FFReStart-Dev-Build.zip";
        private const string ApplicationFolderName = "FFReStart";
        private static readonly HttpClient HttpClient = new() { Timeout = TimeSpan.FromMinutes(10) };

        private LauncherStatus _status = LauncherStatus.downloadingUpdate;

        internal LauncherStatus Status
        {
            get => _status;
            set
            {
                _status = value;

                switch (_status)
                {
                    case LauncherStatus.ready:
                        PlayButton.Content = authenticationService?.IsAuthenticated == true ? "Play Now!" : "Sign In to Play";
                        break;

                    case LauncherStatus.failed:
                        PlayButton.Content = "Update Failed - Retry";
                        break;

                    case LauncherStatus.downloadingGame:
                        PlayButton.Content = "Downloading Game";
                        break;

                    case LauncherStatus.downloadingUpdate:
                        PlayButton.Content = "Downloading Update";
                        break;
                }
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
            settingsFile = Path.Combine(settingsFolder, "settings.dat");
            gameExecutableLocator = new GameExecutableLocator(rootPath);

            var dataProtector = new DpapiDataProtector();
            launcherSettingsStore = new LauncherSettingsStore(settingsFile, dataProtector);
            launcherSettings = launcherSettingsStore.Load();
            var accountRepository = new GameAccountRepository(GameAccountRepository.GetDefaultAccountDatabasePath());
            var sessionStore = new SecureSessionStore(
                Path.Combine(settingsFolder, "auth-session.dat"),
                dataProtector);
            authenticationService = new LauncherAuthenticationService(
                accountRepository,
                sessionStore,
                new AuthTicketService());

            AuthenticationResult restored = authenticationService.RestoreSession();
            if (!restored.Success && restored.Failure is AuthenticationFailure.AccountStoreInvalid)
            {
                AuthStatusText.Text = "The local account database could not be read.";
            }

            RefreshAuthenticationUi();
            RefreshGameLocationText();
        }

        private async void Window_ContentRendered(object sender, EventArgs e)
        {
            await CheckForUpdatesAsync();
        }

        private async Task CheckForUpdatesAsync()
        {
            try
            {
                LauncherVersion onlineVersion = await GetOnlineVersionAsync();

                if (File.Exists(versionFile))
                {
                    LauncherVersion localVersion = new LauncherVersion(File.ReadAllText(versionFile).Trim());
                    VersionText.Text = localVersion.ToString();

                    if (onlineVersion.IsDifferentThan(localVersion))
                    {
                        await InstallGameFilesAsync(true, onlineVersion);
                    }
                    else
                    {
                        Status = LauncherStatus.ready;
                    }
                }
                else
                {
                    await InstallGameFilesAsync(false, onlineVersion);
                }
            }
            catch (Exception ex)
            {
                Status = LauncherStatus.failed;
                MessageBox.Show($"Error checking for or installing game files:\n\n{ex.Message}");
            }
        }

        private static async Task<LauncherVersion> GetOnlineVersionAsync()
        {
            string versionText = (await HttpClient.GetStringAsync(VersionUrl)).Trim();
            return new LauncherVersion(versionText);
        }

        private async Task InstallGameFilesAsync(bool isUpdate, LauncherVersion onlineVersion)
        {
            Status = isUpdate ? LauncherStatus.downloadingUpdate : LauncherStatus.downloadingGame;
            if (File.Exists(gameZip))
            {
                File.Delete(gameZip);
            }

            using (Stream download = await HttpClient.GetStreamAsync(GameZipUrl))
            using (var destination = new FileStream(gameZip, FileMode.CreateNew, FileAccess.Write, FileShare.None, 81920, useAsync: true))
            {
                await download.CopyToAsync(destination);
                await destination.FlushAsync();
            }

            if (!File.Exists(gameZip))
            {
                throw new IOException("Game download finished, but the zip file was not found.");
            }

            ExtractZipToDirectorySkippingUnchangedFiles(gameZip, rootPath);
            File.Delete(gameZip);
            File.WriteAllText(versionFile, onlineVersion.ToString());
            VersionText.Text = onlineVersion.ToString();
            Status = LauncherStatus.ready;
            RefreshGameLocationText();
        }

        private async void PlayButton_Click(object sender, RoutedEventArgs e)
        {
            if (Status == LauncherStatus.ready)
            {
                TicketResult ticket = authenticationService.CreateLaunchTicket();
                if (!ticket.Success || string.IsNullOrEmpty(ticket.Token))
                {
                    AuthStatusText.Text = ticket.ErrorMessage ?? "Sign in before launching the game.";
                    RefreshAuthenticationUi();
                    return;
                }

                string? gameExe = gameExecutableLocator.Find(launcherSettings.GameExecutablePath);
                if (string.IsNullOrEmpty(gameExe))
                {
                    MessageBox.Show("Could not find the game executable. Install/update the game or choose its location.");
                    return;
                }

                string gameFolder = Path.GetDirectoryName(gameExe) ?? rootPath;
                ProcessStartInfo startInfo = new ProcessStartInfo
                {
                    FileName = gameExe,
                    WorkingDirectory = gameFolder,
                    UseShellExecute = false
                };
                startInfo.ArgumentList.Add("--auth-token");
                startInfo.ArgumentList.Add(ticket.Token);

                try
                {
                    Process.Start(startInfo);
                    startInfo.ArgumentList.Clear();
                    Close();
                }
                catch
                {
                    startInfo.ArgumentList.Clear();
                    MessageBox.Show("The game could not be started. Verify the selected game location and try again.");
                }
            }
            else if (Status == LauncherStatus.failed)
            {
                await CheckForUpdatesAsync();
            }
        }

        private async void LoginButton_Click(object sender, RoutedEventArgs e)
        {
            await SignInAsync();
        }

        private async void LoginField_KeyDown(object sender, KeyEventArgs e)
        {
            if (e.Key == Key.Enter)
            {
                e.Handled = true;
                await SignInAsync();
            }
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
            AuthStatusText.Text = "Signing in...";
            try
            {
                bool remember = RememberSessionCheckBox.IsChecked == true;
                AuthenticationResult result = await Task.Run(() => authenticationService.Login(username, password, remember));
                PasswordInput.Clear();
                AuthStatusText.Text = result.Success ? string.Empty : GetAuthenticationError(result.Failure);
            }
            catch (Exception exception) when (exception is IOException or UnauthorizedAccessException or CryptographicException)
            {
                AuthStatusText.Text = "The session could not be stored securely. Please try again.";
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
            authenticationService?.ForgetRememberedSession();
        }

        private void LogoutButton_Click(object sender, RoutedEventArgs e)
        {
            authenticationService.Logout();
            UsernameTextBox.Clear();
            PasswordInput.Clear();
            AuthStatusText.Text = "Signed out.";
            RefreshAuthenticationUi();
            UsernameTextBox.Focus();
        }

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

            if (dialog.ShowDialog(this) != true)
            {
                return;
            }

            if (!GameExecutableLocator.IsUsableGameExecutable(dialog.FileName))
            {
                MessageBox.Show("Select the main FFReStart game executable, not a launcher or crash handler.");
                return;
            }

            launcherSettings.GameExecutablePath = Path.GetFullPath(dialog.FileName);
            try
            {
                launcherSettingsStore.Save(launcherSettings);
            }
            catch (Exception exception) when (exception is IOException or UnauthorizedAccessException or CryptographicException or PlatformNotSupportedException)
            {
                launcherSettings.GameExecutablePath = null;
                MessageBox.Show("The location could not be saved using Windows protected storage. It will not be remembered.");
            }
            RefreshGameLocationText();
        }

        private void RefreshAuthenticationUi()
        {
            bool signedIn = authenticationService?.IsAuthenticated == true;
            LoginPanel.Visibility = signedIn ? Visibility.Collapsed : Visibility.Visible;
            SessionPanel.Visibility = signedIn ? Visibility.Visible : Visibility.Collapsed;
            SignedInUsernameText.Text = authenticationService?.CurrentUsername ?? string.Empty;

            if (Status == LauncherStatus.ready)
            {
                PlayButton.Content = signedIn ? "Play Now!" : "Sign In to Play";
            }
        }

        private void SetAuthenticationControlsEnabled(bool enabled)
        {
            UsernameTextBox.IsEnabled = enabled;
            PasswordInput.IsEnabled = enabled;
            RememberSessionCheckBox.IsEnabled = enabled;
            LoginButton.IsEnabled = enabled;
        }

        private void RefreshGameLocationText()
        {
            string? gameExe = gameExecutableLocator.Find(launcherSettings.GameExecutablePath);
            GameLocationText.Text = gameExe ?? "Game executable will be discovered after installation.";
        }

        private static string GetAuthenticationError(AuthenticationFailure failure) => failure switch
        {
            AuthenticationFailure.InvalidCredentials => "Invalid username or password.",
            AuthenticationFailure.SessionRejected => "Your saved session is no longer valid. Please sign in again.",
            AuthenticationFailure.AccountStoreUnavailable => "No local game account was found. Run the game directly once to create one.",
            AuthenticationFailure.AccountStoreInvalid => "The local account database is invalid.",
            _ => "Sign in failed. Please try again."
        };

        protected override void OnClosed(EventArgs e)
        {
            authenticationService?.Dispose();
            base.OnClosed(e);
        }

        private void ExtractZipToDirectorySkippingUnchangedFiles(string zipPath, string destinationDirectory)
        {
            string fullDestinationDirectory = Path.GetFullPath(destinationDirectory)
                .TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar)
                + Path.DirectorySeparatorChar;

            using (ZipArchive archive = ZipFile.OpenRead(zipPath))
            {
                foreach (ZipArchiveEntry entry in archive.Entries)
                {
                    string destinationPath = Path.GetFullPath(
                        Path.Combine(fullDestinationDirectory, entry.FullName)
                    );

                    if (!destinationPath.StartsWith(fullDestinationDirectory, StringComparison.OrdinalIgnoreCase))
                    {
                        throw new IOException($"Blocked unsafe zip entry path: {entry.FullName}");
                    }

                    bool isDirectory = string.IsNullOrEmpty(entry.Name);

                    if (isDirectory)
                    {
                        Directory.CreateDirectory(destinationPath);
                        continue;
                    }

                    string destinationFolder = Path.GetDirectoryName(destinationPath)!;

                    if (!Directory.Exists(destinationFolder))
                    {
                        Directory.CreateDirectory(destinationFolder);
                    }

                    if (File.Exists(destinationPath) && IsSameFile(entry, destinationPath))
                    {
                        continue;
                    }

                    ExtractEntryToFile(entry, destinationPath);
                }
            }
        }

        private bool IsSameFile(ZipArchiveEntry entry, string existingFilePath)
        {
            FileInfo existingFile = new FileInfo(existingFilePath);

            if (existingFile.Length != entry.Length)
            {
                return false;
            }

            string existingFileHash = GetFileHash(existingFilePath);
            string zipEntryHash = GetZipEntryHash(entry);

            return string.Equals(existingFileHash, zipEntryHash, StringComparison.OrdinalIgnoreCase);
        }

        private string GetFileHash(string filePath)
        {
            using (SHA256 sha256 = SHA256.Create())
            using (FileStream stream = File.OpenRead(filePath))
            {
                byte[] hash = sha256.ComputeHash(stream);
                return BitConverter.ToString(hash).Replace("-", "");
            }
        }

        private string GetZipEntryHash(ZipArchiveEntry entry)
        {
            using (SHA256 sha256 = SHA256.Create())
            using (Stream stream = entry.Open())
            {
                byte[] hash = sha256.ComputeHash(stream);
                return BitConverter.ToString(hash).Replace("-", "");
            }
        }

        private void ExtractEntryToFile(ZipArchiveEntry entry, string destinationPath)
        {
            string tempPath = destinationPath + ".tmp";

            if (File.Exists(tempPath))
            {
                File.Delete(tempPath);
            }

            using (Stream entryStream = entry.Open())
            using (FileStream fileStream = new FileStream(tempPath, FileMode.CreateNew, FileAccess.Write))
            {
                entryStream.CopyTo(fileStream);
            }

            if (File.Exists(destinationPath))
            {
                File.Delete(destinationPath);
            }

            File.Move(tempPath, destinationPath);
        }
    }

    struct LauncherVersion
    {
        internal static LauncherVersion zero = new LauncherVersion(0, 0, 0);

        private short major;
        private short minor;
        private short subMinor;

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

            if (string.IsNullOrWhiteSpace(version))
            {
                return;
            }

            string[] versionStrings = version.Trim().Split('.');

            if (versionStrings.Length != 3)
            {
                return;
            }

            short.TryParse(versionStrings[0], out major);
            short.TryParse(versionStrings[1], out minor);
            short.TryParse(versionStrings[2], out subMinor);
        }

        internal bool IsDifferentThan(LauncherVersion otherVersion)
        {
            return major != otherVersion.major ||
                   minor != otherVersion.minor ||
                   subMinor != otherVersion.subMinor;
        }

        public override string ToString()
        {
            return $"{major}.{minor}.{subMinor}";
        }
    }
}
