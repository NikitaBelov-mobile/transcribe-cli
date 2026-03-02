using System.Collections.Generic;
using System.Linq;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace TranscribeDesktop.WinUI.Views.Onboarding;

public sealed partial class OnboardingRuntimePage : Page
{
    public OnboardingRuntimePage()
    {
        InitializeComponent();
        MainWindow.Instance?.SetStatus("Onboarding step 3/4: runtime setup");
        _ = RefreshStatusAsync();
    }

    private async System.Threading.Tasks.Task RefreshStatusAsync()
    {
        var window = MainWindow.Instance;
        var api = window?.Services.Api;
        if (api is null)
        {
            return;
        }

        try
        {
            var status = await api.GetBootstrapStatusAsync(default);
            ComponentsList.ItemsSource = status.Components.Select(c => new ComponentItem
            {
                Name = c.Name ?? "-",
                Status = c.Status ?? "-",
                Message = string.IsNullOrWhiteSpace(c.Path) ? (c.Message ?? string.Empty) : $"{c.Message} | {c.Path}",
            }).ToList();

            if (status.Ready)
            {
                RuntimeSummary.Text = "Runtime is ready.";
                NextButton.IsEnabled = true;
            }
            else if (!string.IsNullOrWhiteSpace(status.Error))
            {
                RuntimeSummary.Text = "Runtime setup failed: " + status.Error;
                NextButton.IsEnabled = false;
            }
            else if (status.InProgress)
            {
                RuntimeSummary.Text = "Runtime setup in progress...";
                NextButton.IsEnabled = false;
            }
            else
            {
                RuntimeSummary.Text = "Runtime setup is required.";
                NextButton.IsEnabled = false;
            }
        }
        catch (System.Exception ex)
        {
            RuntimeSummary.Text = "Failed to read runtime state: " + ex.Message;
            NextButton.IsEnabled = false;
        }
    }

    private async void SetupButton_Click(object sender, RoutedEventArgs e)
    {
        var api = MainWindow.Instance?.Services.Api;
        if (api is null)
        {
            return;
        }

        try
        {
            MainWindow.Instance?.SetStatus("Starting runtime bootstrap...");
            await api.EnsureBootstrapAsync(default);
            await RefreshStatusAsync();
        }
        catch (System.Exception ex)
        {
            MainWindow.Instance?.SetStatus("Runtime bootstrap failed: " + ex.Message, isError: true);
        }
    }

    private async void RefreshButton_Click(object sender, RoutedEventArgs e)
    {
        await RefreshStatusAsync();
    }

    private void BackButton_Click(object sender, RoutedEventArgs e)
    {
        MainWindow.Instance?.NavigateOnboardingStep("privacy");
    }

    private void NextButton_Click(object sender, RoutedEventArgs e)
    {
        MainWindow.Instance?.NavigateOnboardingStep("models");
    }

    private sealed class ComponentItem
    {
        public string Name { get; set; } = string.Empty;
        public string Status { get; set; } = string.Empty;
        public string Message { get; set; } = string.Empty;
    }
}
