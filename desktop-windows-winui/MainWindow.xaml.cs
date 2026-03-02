using System;
using System.Threading.Tasks;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using TranscribeDesktop.WinUI.Views;
using TranscribeDesktop.WinUI.Views.Onboarding;

namespace TranscribeDesktop.WinUI;

public sealed partial class MainWindow : Window
{
    private readonly App _app;
    private bool _initialized;

    public static MainWindow? Instance { get; private set; }

    public Services.AppServices Services => _app.Services;

    public bool IsOnboardingMode { get; private set; }

    public MainWindow(App app)
    {
        _app = app;
        Instance = this;
        InitializeComponent();

        Closed += MainWindow_Closed;
        _ = InitializeAsync();
    }

    public void SetStatus(string text, bool isError = false)
    {
        StatusText.Text = text;
        StatusText.Foreground = isError
            ? new Microsoft.UI.Xaml.Media.SolidColorBrush(Windows.UI.Color.FromArgb(255, 248, 113, 113))
            : (Microsoft.UI.Xaml.Media.Brush)Application.Current.Resources["AppTextBrush"];
    }

    public async Task CompleteOnboardingAsync()
    {
        Services.Settings.OnboardingCompleted = true;
        await Services.SaveSettingsAsync();

        IsOnboardingMode = false;
        RootNav.IsEnabled = true;
        RootNav.IsPaneOpen = true;
        RootNav.IsPaneToggleButtonVisible = true;
        RootNav.SelectedItem = RootNav.MenuItems[0];
        MainFrame.Navigate(typeof(DashboardPage));
        SetStatus("Onboarding completed. App is ready.");
    }

    public void NavigateOnboardingStep(string step)
    {
        Type target = step switch
        {
            "privacy" => typeof(OnboardingPrivacyPage),
            "runtime" => typeof(OnboardingRuntimePage),
            "models" => typeof(OnboardingModelPage),
            _ => typeof(OnboardingWelcomePage),
        };
        MainFrame.Navigate(target);
    }

    private async Task InitializeAsync()
    {
        if (_initialized)
        {
            return;
        }
        _initialized = true;

        try
        {
            SetStatus("Starting local daemon and loading settings...");
            await Services.InitializeAsync();

            ConnectionText.Text = Services.Daemon.StartedByApp
                ? $"Connected to local daemon: {Services.Daemon.BaseUrl}"
                : $"Connected to existing daemon: {Services.Daemon.BaseUrl}";
            SubConnectionText.Text = $"State: {Services.StateDirectory}";

            if (!Services.Settings.OnboardingCompleted)
            {
                IsOnboardingMode = true;
                RootNav.IsEnabled = false;
                RootNav.IsPaneOpen = false;
                RootNav.IsPaneToggleButtonVisible = false;
                MainFrame.Navigate(typeof(OnboardingWelcomePage));
                SetStatus("Onboarding required: go through data sharing, runtime setup, and model preparation.");
                return;
            }

            IsOnboardingMode = false;
            RootNav.IsEnabled = true;
            RootNav.IsPaneToggleButtonVisible = true;
            RootNav.SelectedItem = RootNav.MenuItems[0];
            MainFrame.Navigate(typeof(DashboardPage));
            SetStatus("Ready.");
        }
        catch (Exception ex)
        {
            RootNav.IsEnabled = false;
            RootNav.IsPaneOpen = false;
            RootNav.IsPaneToggleButtonVisible = false;
            MainFrame.Navigate(typeof(ErrorPage), ex.Message);
            SetStatus($"Startup error: {ex.Message}", isError: true);
        }
    }

    private void RootNav_SelectionChanged(NavigationView sender, NavigationViewSelectionChangedEventArgs args)
    {
        if (IsOnboardingMode)
        {
            return;
        }

        if (args.SelectedItemContainer?.Tag is not string tag)
        {
            return;
        }

        switch (tag)
        {
            case "dashboard":
                MainFrame.Navigate(typeof(DashboardPage));
                break;
            case "queue":
                MainFrame.Navigate(typeof(QueuePage));
                break;
            case "models":
                MainFrame.Navigate(typeof(ModelsPage));
                break;
            case "settings":
                MainFrame.Navigate(typeof(SettingsPage));
                break;
        }
    }

    private void MainWindow_Closed(object sender, WindowEventArgs args)
    {
        Services.Dispose();
    }
}
