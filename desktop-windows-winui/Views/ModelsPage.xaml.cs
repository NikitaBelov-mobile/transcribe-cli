using System.Collections.Generic;
using System.Globalization;
using System.Linq;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace TranscribeDesktop.WinUI.Views;

public sealed partial class ModelsPage : Page
{
    private readonly List<PresetItem> _presets = new();

    public ModelsPage()
    {
        InitializeComponent();
        _ = RefreshAsync();
    }

    private async System.Threading.Tasks.Task RefreshAsync()
    {
        var api = MainWindow.Instance?.Services.Api;
        if (api is null)
        {
            return;
        }

        try
        {
            var models = await api.GetModelsAsync(default);
            var presets = await api.GetPresetsAsync(default);

            ModelsDirText.Text = "Models dir: " + (models.ModelsDir ?? "-");
            DefaultModelText.Text = "Default model: " + (models.DefaultModel ?? "-");

            var installedNames = models.Models
                .Select(m => m.Name)
                .Where(n => !string.IsNullOrWhiteSpace(n))
                .Select(n => n!)
                .OrderBy(n => n)
                .ToList();
            InstalledCombo.ItemsSource = installedNames;
            if (installedNames.Count > 0)
            {
                InstalledCombo.SelectedItem = models.DefaultModel;
            }

            InstalledList.ItemsSource = models.Models.Select(m => new ModelItem
            {
                Name = m.Name ?? "-",
                Size = FormatSize(m.SizeBytes),
                Path = m.Path ?? string.Empty,
            }).ToList();

            _presets.Clear();
            _presets.AddRange(presets.Presets
                .Where(p => !string.IsNullOrWhiteSpace(p.Name))
                .Select(p => new PresetItem
                {
                    Name = p.Name!,
                    Display = string.IsNullOrWhiteSpace(p.Alias) ? p.Name! : $"{p.Alias} ({p.Name})",
                }));
            PresetCombo.ItemsSource = _presets;
            if (_presets.Count > 0)
            {
                PresetCombo.SelectedIndex = 0;
            }

            MainWindow.Instance?.SetStatus("Models refreshed");
        }
        catch (System.Exception ex)
        {
            MainWindow.Instance?.SetStatus("Failed to refresh models: " + ex.Message, isError: true);
        }
    }

    private async void SetDefaultButton_Click(object sender, RoutedEventArgs e)
    {
        var api = MainWindow.Instance?.Services.Api;
        if (api is null)
        {
            return;
        }
        if (InstalledCombo.SelectedItem is not string model || string.IsNullOrWhiteSpace(model))
        {
            return;
        }

        try
        {
            await api.SetDefaultModelAsync(model, default);
            MainWindow.Instance!.Services.Settings.PreferredModel = model;
            await MainWindow.Instance.Services.SaveSettingsAsync();
            await RefreshAsync();
        }
        catch (System.Exception ex)
        {
            MainWindow.Instance?.SetStatus("Set default failed: " + ex.Message, isError: true);
        }
    }

    private async void InstallButton_Click(object sender, RoutedEventArgs e)
    {
        var api = MainWindow.Instance?.Services.Api;
        if (api is null)
        {
            return;
        }
        if (PresetCombo.SelectedItem is not PresetItem preset)
        {
            return;
        }

        try
        {
            MainWindow.Instance?.SetStatus("Installing model: " + preset.Name);
            await api.InstallModelAsync(preset.Name, default);
            await api.SetDefaultModelAsync(preset.Name, default);
            MainWindow.Instance!.Services.Settings.PreferredModel = preset.Name;
            await MainWindow.Instance.Services.SaveSettingsAsync();
            await RefreshAsync();
        }
        catch (System.Exception ex)
        {
            MainWindow.Instance?.SetStatus("Install failed: " + ex.Message, isError: true);
        }
    }

    private async void RefreshButton_Click(object sender, RoutedEventArgs e)
    {
        await RefreshAsync();
    }

    private static string FormatSize(long bytes)
    {
        if (bytes < 0)
        {
            return "-";
        }
        var units = new[] { "B", "KB", "MB", "GB" };
        double value = bytes;
        var index = 0;
        while (value >= 1024 && index < units.Length - 1)
        {
            value /= 1024;
            index++;
        }
        return string.Format(CultureInfo.InvariantCulture, "{0:0.##} {1}", value, units[index]);
    }

    private sealed class ModelItem
    {
        public string Name { get; set; } = string.Empty;
        public string Size { get; set; } = string.Empty;
        public string Path { get; set; } = string.Empty;
    }

    private sealed class PresetItem
    {
        public string Name { get; set; } = string.Empty;
        public string Display { get; set; } = string.Empty;

        public override string ToString() => Display;
    }
}
