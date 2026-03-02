using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace TranscribeDesktop.WinUI.Views.Onboarding;

public sealed partial class OnboardingPrivacyPage : Page
{
    public OnboardingPrivacyPage()
    {
        InitializeComponent();

        var settings = MainWindow.Current?.Services.Settings;
        DiagnosticsToggle.IsOn = settings?.AllowAnonymousDiagnostics ?? false;
        MainWindow.Current?.SetStatus("Onboarding step 2/4: data sharing");
    }

    private async void NextButton_Click(object sender, RoutedEventArgs e)
    {
        var window = MainWindow.Current;
        if (window is null)
        {
            return;
        }

        window.Services.Settings.AllowAnonymousDiagnostics = DiagnosticsToggle.IsOn;
        await window.Services.SaveSettingsAsync();
        window.NavigateOnboardingStep("runtime");
    }

    private void BackButton_Click(object sender, RoutedEventArgs e)
    {
        MainWindow.Current?.NavigateOnboardingStep("welcome");
    }
}
