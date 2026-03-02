using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace TranscribeDesktop.WinUI.Views;

public sealed partial class SettingsPage : Page
{
    public SettingsPage()
    {
        InitializeComponent();

        var services = MainWindow.Current?.Services;
        if (services is null)
        {
            return;
        }

        DiagnosticsToggle.IsOn = services.Settings.AllowAnonymousDiagnostics;
        PreferredModelText.Text = "Preferred model: " + services.Settings.PreferredModel;
        StatePathText.Text = "State directory: " + services.StateDirectory;
    }

    private async void SaveButton_Click(object sender, RoutedEventArgs e)
    {
        if (MainWindow.Current is not { } window)
        {
            return;
        }

        window.Services.Settings.AllowAnonymousDiagnostics = DiagnosticsToggle.IsOn;
        await window.Services.SaveSettingsAsync();
        window.SetStatus("Settings saved");
    }

    private async void RestartOnboardingButton_Click(object sender, RoutedEventArgs e)
    {
        if (MainWindow.Current is not { } window)
        {
            return;
        }

        window.Services.Settings.OnboardingCompleted = false;
        await window.Services.SaveSettingsAsync();

        window.SetStatus("Onboarding reset. Restart app to run onboarding flow again.");
    }
}
