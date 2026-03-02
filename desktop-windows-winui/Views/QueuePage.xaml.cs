using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Linq;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Windows.Storage;
using Windows.Storage.Pickers;

namespace TranscribeDesktop.WinUI.Views;

public sealed partial class QueuePage : Page
{
    private readonly List<JobItem> _jobs = new();

    public QueuePage()
    {
        InitializeComponent();

        var preferred = MainWindow.Current?.Services.Settings.PreferredModel;
        ModelBox.Text = string.IsNullOrWhiteSpace(preferred) ? "ggml-large-v3-turbo" : preferred;
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
            var jobs = await api.GetJobsAsync(default);
            var selectedId = (JobsList.SelectedItem as JobItem)?.Id;

            _jobs.Clear();
            _jobs.AddRange(jobs.Jobs.Select(j => new JobItem
            {
                Id = j.Id ?? string.Empty,
                Status = j.Status ?? string.Empty,
                Progress = $"{j.Progress}%",
                Model = j.Model ?? string.Empty,
                FilePath = j.FilePath ?? string.Empty,
                OutputDir = j.OutputDir ?? string.Empty,
                ResultText = j.ResultText ?? string.Empty,
                ResultSrt = j.ResultSrt ?? string.Empty,
                ResultVtt = j.ResultVtt ?? string.Empty,
            }));

            JobsList.ItemsSource = null;
            JobsList.ItemsSource = _jobs;

            if (!string.IsNullOrWhiteSpace(selectedId))
            {
                JobsList.SelectedItem = _jobs.FirstOrDefault(x => string.Equals(x.Id, selectedId, StringComparison.OrdinalIgnoreCase));
            }

            MainWindow.Current?.SetStatus("Queue refreshed");
        }
        catch (Exception ex)
        {
            MainWindow.Current?.SetStatus("Queue refresh failed: " + ex.Message, isError: true);
        }
    }

    private async void AddFilesButton_Click(object sender, RoutedEventArgs e)
    {
        var api = MainWindow.Current?.Services.Api;
        if (api is null || MainWindow.Current is null)
        {
            return;
        }

        var picker = new FileOpenPicker();
        var hwnd = WinRT.Interop.WindowNative.GetWindowHandle(MainWindow.Current);
        WinRT.Interop.InitializeWithWindow.Initialize(picker, hwnd);
        picker.FileTypeFilter.Add("*");

        IReadOnlyList<StorageFile> files;
        try
        {
            files = await picker.PickMultipleFilesAsync();
        }
        catch (Exception ex)
        {
            MainWindow.Current.SetStatus("File picker failed: " + ex.Message, isError: true);
            return;
        }

        if (files.Count == 0)
        {
            return;
        }

        var model = string.IsNullOrWhiteSpace(ModelBox.Text) ? "ggml-large-v3-turbo" : ModelBox.Text.Trim();
        var language = string.IsNullOrWhiteSpace(LanguageBox.Text) ? "auto" : LanguageBox.Text.Trim();

        try
        {
            MainWindow.Current.SetStatus($"Queueing {files.Count} file(s)...");
            foreach (var file in files)
            {
                await api.AddJobAsync(new Services.AddJobRequest
                {
                    FilePath = file.Path,
                    Language = language,
                    Model = model,
                }, default);
            }

            await RefreshAsync();
            MainWindow.Current.SetStatus("Files queued");
        }
        catch (Exception ex)
        {
            MainWindow.Current.SetStatus("Queueing failed: " + ex.Message, isError: true);
        }
    }

    private async void RefreshButton_Click(object sender, RoutedEventArgs e)
    {
        await RefreshAsync();
    }

    private async void CancelButton_Click(object sender, RoutedEventArgs e)
    {
        await ChangeSelectedJobAsync(isRetry: false);
    }

    private async void RetryButton_Click(object sender, RoutedEventArgs e)
    {
        await ChangeSelectedJobAsync(isRetry: true);
    }

    private void OpenResultButton_Click(object sender, RoutedEventArgs e)
    {
        var selected = JobsList.SelectedItem as JobItem;
        if (selected is null)
        {
            return;
        }

        var candidates = new[] { selected.ResultText, selected.ResultSrt, selected.ResultVtt }
            .Where(path => !string.IsNullOrWhiteSpace(path) && File.Exists(path))
            .ToList();

        if (candidates.Count > 0)
        {
            OpenInExplorerSelect(candidates[0]);
            return;
        }
        if (!string.IsNullOrWhiteSpace(selected.OutputDir) && Directory.Exists(selected.OutputDir))
        {
            OpenInExplorerOpenDir(selected.OutputDir);
            return;
        }

        MainWindow.Current?.SetStatus("Result is not available yet", isError: true);
    }

    private async System.Threading.Tasks.Task ChangeSelectedJobAsync(bool isRetry)
    {
        var api = MainWindow.Current?.Services.Api;
        var selected = JobsList.SelectedItem as JobItem;
        if (api is null || selected is null || string.IsNullOrWhiteSpace(selected.Id))
        {
            return;
        }

        try
        {
            if (isRetry)
            {
                await api.RetryJobAsync(selected.Id, default);
                MainWindow.Current?.SetStatus("Job re-queued: " + selected.Id);
            }
            else
            {
                await api.CancelJobAsync(selected.Id, default);
                MainWindow.Current?.SetStatus("Job canceled: " + selected.Id);
            }
            await RefreshAsync();
        }
        catch (Exception ex)
        {
            MainWindow.Current?.SetStatus("Job action failed: " + ex.Message, isError: true);
        }
    }

    private static void OpenInExplorerSelect(string path)
    {
        Process.Start(new ProcessStartInfo
        {
            FileName = "explorer.exe",
            Arguments = $"/select,\"{path}\"",
            UseShellExecute = true,
        });
    }

    private static void OpenInExplorerOpenDir(string path)
    {
        Process.Start(new ProcessStartInfo
        {
            FileName = "explorer.exe",
            Arguments = $"\"{path}\"",
            UseShellExecute = true,
        });
    }

    private sealed class JobItem
    {
        public string Id { get; set; } = string.Empty;
        public string Status { get; set; } = string.Empty;
        public string Progress { get; set; } = string.Empty;
        public string Model { get; set; } = string.Empty;
        public string FilePath { get; set; } = string.Empty;
        public string OutputDir { get; set; } = string.Empty;
        public string ResultText { get; set; } = string.Empty;
        public string ResultSrt { get; set; } = string.Empty;
        public string ResultVtt { get; set; } = string.Empty;
    }
}
