using System;
using System.IO;
using System.Text.Json;
using System.Threading.Tasks;
using TranscribeDesktop.WinUI.Models;

namespace TranscribeDesktop.WinUI.Services;

public sealed class SettingsStore
{
    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        WriteIndented = true,
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
    };

    private readonly string _settingsFile;

    public SettingsStore(string stateDirectory)
    {
        Directory.CreateDirectory(stateDirectory);
        _settingsFile = Path.Combine(stateDirectory, "winui-settings.json");
    }

    public async Task<UserSettings> LoadAsync()
    {
        if (!File.Exists(_settingsFile))
        {
            return new UserSettings();
        }

        await using var stream = File.OpenRead(_settingsFile);
        var loaded = await JsonSerializer.DeserializeAsync<UserSettings>(stream, JsonOptions).ConfigureAwait(false);
        return loaded ?? new UserSettings();
    }

    public async Task SaveAsync(UserSettings settings)
    {
        var temp = _settingsFile + ".tmp";
        await using (var stream = File.Create(temp))
        {
            await JsonSerializer.SerializeAsync(stream, settings, JsonOptions).ConfigureAwait(false);
        }

        if (File.Exists(_settingsFile))
        {
            File.Delete(_settingsFile);
        }
        File.Move(temp, _settingsFile);
    }
}
