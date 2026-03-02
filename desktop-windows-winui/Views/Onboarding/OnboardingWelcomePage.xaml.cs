using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace TranscribeDesktop.WinUI.Views.Onboarding;

public sealed partial class OnboardingWelcomePage : Page
{
    public OnboardingWelcomePage()
    {
        InitializeComponent();
        MainWindow.Instance?.SetStatus("Onboarding step 1/4: introduction");
    }

    private void NextButton_Click(object sender, RoutedEventArgs e)
    {
        MainWindow.Instance?.NavigateOnboardingStep("privacy");
    }
}
