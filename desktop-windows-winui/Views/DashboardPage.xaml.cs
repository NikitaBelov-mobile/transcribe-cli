using System.Linq;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace TranscribeDesktop.WinUI.Views;

public sealed partial class DashboardPage : Page
{
    public DashboardPage()
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
            var health = await api.HealthAsync(default);
            var runtime = await api.GetBootstrapStatusAsync(default);
            var update = await api.GetUpdateStatusAsync(default);

            HealthText.Text = $"Daemon: {health.Status} | Service: {health.Service} | Version: {health.Version}";

            var components = string.Join("; ", runtime.Components.Select(c => $"{c.Name}:{c.Status}"));
            RuntimeText.Text = runtime.Ready
                ? $"Runtime ready | {components}"
                : $"Runtime pending (inProgress={runtime.InProgress}) | {components} | error={runtime.Error}";

            UpdateText.Text = update.Enabled
                ? $"Update: current={update.CurrentVersion}, latest={update.LatestVersion}, available={update.UpdateAvailable}, message={update.Message}, error={update.Error}"
                : "Update: disabled";

            MainWindow.Instance?.SetStatus("Dashboard refreshed");
        }
        catch (System.Exception ex)
        {
            MainWindow.Instance?.SetStatus("Dashboard refresh failed: " + ex.Message, isError: true);
        }
    }

    private async void RefreshButton_Click(object sender, RoutedEventArgs e)
    {
        await RefreshAsync();
    }

    private async void CheckUpdatesButton_Click(object sender, RoutedEventArgs e)
    {
        var api = MainWindow.Instance?.Services.Api;
        if (api is null)
        {
            return;
        }

        try
        {
            await api.CheckUpdatesAsync(default);
            await RefreshAsync();
        }
        catch (System.Exception ex)
        {
            MainWindow.Instance?.SetStatus("Update check failed: " + ex.Message, isError: true);
        }
    }

    private async void RunSetupButton_Click(object sender, RoutedEventArgs e)
    {
        var api = MainWindow.Instance?.Services.Api;
        if (api is null)
        {
            return;
        }

        try
        {
            await api.EnsureBootstrapAsync(default);
            await RefreshAsync();
        }
        catch (System.Exception ex)
        {
            MainWindow.Instance?.SetStatus("Runtime setup failed: " + ex.Message, isError: true);
        }
    }
}
