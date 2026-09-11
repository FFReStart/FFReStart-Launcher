using System;
using System.ComponentModel;
using System.Diagnostics;
using System.IO;
using System.IO.Compression;
using System.Net;
using System.Security.Cryptography;
using System.Windows;

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
        private string gameFolder;
        private string gameExe;

        private const string VersionUrl = "https://github.com/FFReStart/FFReStart_Builds/releases/latest/download/version.txt";
        private const string GameZipUrl = "https://github.com/FFReStart/FFReStart_Builds/releases/latest/download/FFReStart-Dev-Build.zip";

        private LauncherStatus _status;

        internal LauncherStatus Status
        {
            get => _status;
            set
            {
                _status = value;

                switch (_status)
                {
                    case LauncherStatus.ready:
                        PlayButton.Content = "Play Now!";
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

            rootPath = Directory.GetCurrentDirectory();

            versionFile = Path.Combine(rootPath, "Version.txt");
            gameZip = Path.Combine(rootPath, "FFReStart-Dev-Build.zip");

            gameFolder = Path.Combine(rootPath, "FFReStart-Dev-Build");
            gameExe = Path.Combine(gameFolder, "FFReStart-Dev-Build.exe");
        }

        private void Window_ContentRendered(object sender, EventArgs e)
        {
            CheckForUpdates();
        }

        private void CheckForUpdates()
        {
            try
            {
                LauncherVersion onlineVersion = GetOnlineVersion();

                if (File.Exists(versionFile))
                {
                    LauncherVersion localVersion = new LauncherVersion(File.ReadAllText(versionFile).Trim());
                    VersionText.Text = localVersion.ToString();

                    if (onlineVersion.IsDifferentThan(localVersion))
                    {
                        InstallGameFiles(true, onlineVersion);
                    }
                    else
                    {
                        Status = LauncherStatus.ready;
                    }
                }
                else
                {
                    InstallGameFiles(false, onlineVersion);
                }
            }
            catch (Exception ex)
            {
                Status = LauncherStatus.failed;
                MessageBox.Show($"Error checking for game updates:\n\n{ex.Message}");
            }
        }

        private LauncherVersion GetOnlineVersion()
        {
            using (WebClient webClient = new WebClient())
            {
                string versionText = webClient.DownloadString(VersionUrl).Trim();
                return new LauncherVersion(versionText);
            }
        }

        private void InstallGameFiles(bool isUpdate, LauncherVersion onlineVersion)
        {
            try
            {
                Status = isUpdate ? LauncherStatus.downloadingUpdate : LauncherStatus.downloadingGame;

                if (File.Exists(gameZip))
                {
                    File.Delete(gameZip);
                }

                WebClient webClient = new WebClient();
                webClient.DownloadFileCompleted += DownloadGameCompletedCallback;

                webClient.DownloadFileAsync(
                    new Uri(GameZipUrl),
                    gameZip,
                    onlineVersion
                );
            }
            catch (Exception ex)
            {
                Status = LauncherStatus.failed;
                MessageBox.Show($"Error installing game files:\n\n{ex.Message}");
            }
        }

        private void DownloadGameCompletedCallback(object sender, AsyncCompletedEventArgs e)
        {
            try
            {
                if (e.Cancelled)
                {
                    Status = LauncherStatus.failed;
                    MessageBox.Show("Game download was cancelled.");
                    return;
                }

                if (e.Error != null)
                {
                    Status = LauncherStatus.failed;
                    MessageBox.Show($"Game download failed:\n\n{e.Error.Message}");
                    return;
                }

                if (!File.Exists(gameZip))
                {
                    Status = LauncherStatus.failed;
                    MessageBox.Show("Game download finished, but the zip file was not found.");
                    return;
                }

                LauncherVersion onlineVersion = (LauncherVersion)e.UserState;

                ExtractZipToDirectorySkippingUnchangedFiles(gameZip, rootPath);

                File.Delete(gameZip);

                File.WriteAllText(versionFile, onlineVersion.ToString());

                VersionText.Text = onlineVersion.ToString();
                Status = LauncherStatus.ready;
            }
            catch (Exception ex)
            {
                Status = LauncherStatus.failed;
                MessageBox.Show($"Error finishing download:\n\n{ex.Message}");
            }
        }

        private void PlayButton_Click(object sender, RoutedEventArgs e)
        {
            if (Status == LauncherStatus.ready)
            {
                if (!File.Exists(gameExe))
                {
                    Status = LauncherStatus.failed;
                    MessageBox.Show($"Could not find game executable:\n\n{gameExe}");
                    return;
                }

                ProcessStartInfo startInfo = new ProcessStartInfo(gameExe)
                {
                    WorkingDirectory = gameFolder
                };

                Process.Start(startInfo);
                Close();
            }
            else if (Status == LauncherStatus.failed)
            {
                CheckForUpdates();
            }
        }

        private void ExtractZipToDirectorySkippingUnchangedFiles(string zipPath, string destinationDirectory)
        {
            string fullDestinationDirectory = Path.GetFullPath(destinationDirectory);

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

                    string destinationFolder = Path.GetDirectoryName(destinationPath);

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