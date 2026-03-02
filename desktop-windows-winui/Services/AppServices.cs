using System;
using System.IO;
using System.Threading;
using System.Threading.Tasks;
using TranscribeDesktop.WinUI.Models;

namespace TranscribeDesktop.WinUI.Services;

public sealed class AppServices : IDisposable
{
    private readonly CancellationTokenSource _lifetime = new();

    public DaemonHost Daemon { get; private set; } = new();
    public TranscribeApiClient? Api { get; private set; }
    public SettingsStore? SettingsStore { get; private set; }
    public UserSettings Settings { get; private set; } = new();
    public string StateDirectory { get; private set; } = string.Empty;

    public async Task InitializeAsync()
    {
        StateDirectory = ResolveStateDirectory();
        SettingsStore = new SettingsStore(StateDirectory);
        Settings = await SettingsStore.LoadAsync().ConfigureAwait(false);

        await Daemon.StartAsync(_lifetime.Token).ConfigureAwait(false);
        Api = new TranscribeApiClient(Daemon.BaseUrl);
    }

    public Task SaveSettingsAsync()
    {
        return SettingsStore is null ? Task.CompletedTask : SettingsStore.SaveAsync(Settings);
    }

    public void Dispose()
    {
        _lifetime.Cancel();
        Api?.Dispose();
        Daemon.Dispose();
        _lifetime.Dispose();
    }

    private static string ResolveStateDirectory()
    {
        var appData = Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData);
        if (string.IsNullOrWhiteSpace(appData))
        {
            appData = AppContext.BaseDirectory;
        }

        var root = Path.Combine(appData, "TranscribeCLI");
        Directory.CreateDirectory(root);
        return root;
    }
}
