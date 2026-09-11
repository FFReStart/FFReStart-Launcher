using GameLauncher;
using System.Diagnostics;

const string expectedCommunity = "https://discord.gg/Q5je3v9Bjg";
const string expectedSupport = "https://discord.gg/VNVjmPn2Fn";

AssertEqual(expectedCommunity, LauncherLinks.GetUrl(LauncherLinkDestination.Community), "Community destination");
AssertEqual(expectedSupport, LauncherLinks.GetUrl(LauncherLinkDestination.Support), "Support destination");

AssertStartInfo(expectedCommunity, LauncherLinkDestination.Community);
AssertStartInfo(expectedSupport, LauncherLinkDestination.Support);

if (LauncherLinks.GetUrl(LauncherLinkDestination.Community) == LauncherLinks.GetUrl(LauncherLinkDestination.Support))
{
    throw new InvalidOperationException("Community and Support must remain distinct destinations.");
}

foreach (string url in new[] { expectedCommunity, expectedSupport })
{
    Uri parsed = new Uri(url, UriKind.Absolute);
    AssertEqual("https", parsed.Scheme, $"Secure scheme for {url}");
    AssertEqual("discord.gg", parsed.Host, $"Discord host for {url}");
}

Console.WriteLine("Launcher link contract tests passed.");

static void AssertEqual(string expected, string actual, string label)
{
    if (!string.Equals(expected, actual, StringComparison.Ordinal))
    {
        throw new InvalidOperationException($"{label}: expected '{expected}', got '{actual}'.");
    }
}

static void AssertStartInfo(string expectedUrl, LauncherLinkDestination destination)
{
    ProcessStartInfo startInfo = LauncherLinks.CreateStartInfo(destination);
    AssertEqual(expectedUrl, startInfo.FileName, $"Browser target for {destination}");
    if (!startInfo.UseShellExecute)
    {
        throw new InvalidOperationException($"{destination} must use the OS safe default-browser shell mechanism.");
    }
}
