using System.Collections.Generic;
using System.Linq;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace TranscribeDesktop.WinUI.Views.Onboarding;

public sealed partial class OnboardingModelPage : Page
{
    private List<PresetItem> _presets = new();

    public OnboardingModelPage()
    {
        InitializeComponent();
        MainWindow.Current?.SetStatus("Onboarding step 4/4: model preparation");
        _ = RefreshAsync();
    }

    private async System.Threading.Tasks.Task RefreshAsync()
    {
        var api = MainWindow.Current?.Services.Api;
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

            _presets = presets.Presets
                .Where(p => !string.IsNullOrWhiteSpace(p.Name))
                .Select(p => new PresetItem
                {
                    Name = p.Name!,
                    Display = string.IsNullOrWhiteSpace(p.Alias) ? p.Name! : $"{p.Alias} ({p.Name})",
                })
                .ToList();
            PresetCombo.ItemsSource = _presets;
            if (_presets.Count > 0)
            {
                PresetCombo.SelectedIndex = 0;
            }
        }
        catch (System.Exception ex)
        {
            MainWindow.Current?.SetStatus("Failed to load model info: " + ex.Message, isError: true);
        }
    }

    private async void InstallButton_Click(object sender, RoutedEventArgs e)
    {
        var api = MainWindow.Current?.Services.Api;
        if (api is null)
        {
            return;
        }

        var selected = PresetCombo.SelectedItem as PresetItem;
        if (selected is null)
        {
            return;
        }

        try
        {
            MainWindow.Current?.SetStatus("Installing model preset: " + selected.Name);
            await api.InstallModelAsync(selected.Name, default);
            await api.SetDefaultModelAsync(selected.Name, default);

            var window = MainWindow.Current;
            if (window is not null)
            {
                window.Services.Settings.PreferredModel = selected.Name;
                await window.Services.SaveSettingsAsync();
            }

            await RefreshAsync();
            MainWindow.Current?.SetStatus("Model installed: " + selected.Name);
        }
        catch (System.Exception ex)
        {
            MainWindow.Current?.SetStatus("Model install failed: " + ex.Message, isError: true);
        }
    }

    private async void RefreshButton_Click(object sender, RoutedEventArgs e)
    {
        await RefreshAsync();
    }

    private void BackButton_Click(object sender, RoutedEventArgs e)
    {
        MainWindow.Current?.NavigateOnboardingStep("runtime");
    }

    private async void FinishButton_Click(object sender, RoutedEventArgs e)
    {
        if (MainWindow.Current is { } window)
        {
            await window.CompleteOnboardingAsync();
        }
    }

    private sealed class PresetItem
    {
        public string Name { get; set; } = string.Empty;
        public string Display { get; set; } = string.Empty;

        public override string ToString() => Display;
    }
}
