using System.Diagnostics;

namespace GameLauncher;

public static class GameLaunchCommand
{
    public const string AuthenticationArgument = "--auth-token";

    public static ProcessStartInfo Create(string executablePath, string token)
    {
        if (string.IsNullOrWhiteSpace(executablePath))
            throw new ArgumentException("A game executable path is required.", nameof(executablePath));
        if (string.IsNullOrWhiteSpace(token))
            throw new ArgumentException("An authentication ticket is required.", nameof(token));

        var startInfo = new ProcessStartInfo
        {
            FileName = executablePath,
            WorkingDirectory = Path.GetDirectoryName(executablePath) ?? string.Empty,
            UseShellExecute = false
        };
        startInfo.ArgumentList.Add(AuthenticationArgument);
        startInfo.ArgumentList.Add(token);
        return startInfo;
    }
}
