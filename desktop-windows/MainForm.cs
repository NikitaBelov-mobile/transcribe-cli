using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Threading;
using System.Threading.Tasks;
using System.Windows.Forms;

namespace TranscribeDesktop;

public sealed class MainForm : Form
{
    private static readonly Color BackgroundColor = Color.FromArgb(15, 23, 42);
    private static readonly Color PanelColor = Color.FromArgb(30, 41, 59);
    private static readonly Color InputColor = Color.FromArgb(51, 65, 85);
    private static readonly Color TextColor = Color.FromArgb(241, 245, 249);
    private static readonly Color MutedTextColor = Color.FromArgb(148, 163, 184);
    private static readonly Color AccentColor = Color.FromArgb(14, 165, 233);
    private static readonly Color AccentHoverColor = Color.FromArgb(2, 132, 199);
    private static readonly Color SuccessColor = Color.FromArgb(34, 197, 94);
    private static readonly Color SuccessHoverColor = Color.FromArgb(22, 163, 74);
    private static readonly Color NeutralButtonColor = Color.FromArgb(71, 85, 105);
    private static readonly Color NeutralButtonHoverColor = Color.FromArgb(100, 116, 139);

    private readonly DaemonHost _daemon = new();
    private readonly CancellationTokenSource _lifetimeCts = new();

    private TranscribeApiClient? _api;
    private System.Windows.Forms.Timer? _refreshTimer;

    private readonly Label _connectionLabel = new();
    private readonly Label _bootstrapLabel = new();
    private readonly ListView _bootstrapList = new();
    private readonly Label _updateLabel = new();
    private readonly Label _defaultModelLabel = new();
    private readonly Label _modelsDirLabel = new();
    private readonly ComboBox _installedModels = new();
    private readonly ComboBox _presetModels = new();
    private readonly ComboBox _runModel = new();
    private readonly TextBox _language = new();
    private readonly DataGridView _jobsGrid = new();
    private readonly ToolStripStatusLabel _status = new();

    private readonly Button _addFilesButton = new();
    private readonly Button _setDefaultButton = new();
    private readonly Button _installModelButton = new();
    private readonly Button _checkUpdatesButton = new();
    private readonly Button _refreshButton = new();
    private readonly Button _cancelJobButton = new();
    private readonly Button _retryJobButton = new();
    private readonly Button _openResultButton = new();

    private bool _refreshInProgress;
    private bool _modelInstallInProgress;
    private List<JobDto> _jobs = new();

    public MainForm()
    {
        Text = "Transcribe Desktop";
        Width = 1320;
        Height = 900;
        MinimumSize = new Size(1024, 700);
        StartPosition = FormStartPosition.CenterScreen;

        InitializeLayout();
        ApplyTheme();
    }

    protected override async void OnShown(EventArgs e)
    {
        base.OnShown(e);
        try
        {
            SetStatus("Starting local service...");
            await _daemon.StartAsync(_lifetimeCts.Token);
            _api = new TranscribeApiClient(_daemon.BaseUrl);
            _connectionLabel.Text = _daemon.StartedByApp
                ? $"Daemon started locally: {_daemon.BaseUrl}"
                : $"Connected to existing daemon: {_daemon.BaseUrl}";

            _refreshTimer = new System.Windows.Forms.Timer { Interval = 2000 };
            _refreshTimer.Tick += async (_, _) => await RefreshAllAsync();
            _refreshTimer.Start();

            await RefreshAllAsync(force: true);
            SetStatus("Ready");
        }
        catch (Exception ex)
        {
            MessageBox.Show(this, ex.Message, "Startup error", MessageBoxButtons.OK, MessageBoxIcon.Error);
            Close();
        }
    }

    protected override void OnFormClosing(FormClosingEventArgs e)
    {
        _refreshTimer?.Stop();
        _lifetimeCts.Cancel();
        _api?.Dispose();
        _daemon.Dispose();
        _lifetimeCts.Dispose();
        base.OnFormClosing(e);
    }

    private void InitializeLayout()
    {
        var root = new TableLayoutPanel
        {
            Dock = DockStyle.Fill,
            ColumnCount = 1,
            RowCount = 4,
            Padding = new Padding(14),
        };
        root.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        root.RowStyles.Add(new RowStyle(SizeType.Absolute, 290));
        root.RowStyles.Add(new RowStyle(SizeType.Absolute, 96));
        root.RowStyles.Add(new RowStyle(SizeType.Percent, 100));

        _connectionLabel.Dock = DockStyle.Fill;
        _connectionLabel.Font = new Font("Segoe UI Semibold", 10.5F, FontStyle.Bold);
        _connectionLabel.Text = "Initializing...";
        _connectionLabel.Margin = new Padding(0, 0, 0, 8);
        root.Controls.Add(_connectionLabel, 0, 0);

        root.Controls.Add(BuildTopSection(), 0, 1);
        root.Controls.Add(BuildQueueSection(), 0, 2);
        root.Controls.Add(BuildJobsSection(), 0, 3);

        var strip = new StatusStrip();
        strip.Items.Add(_status);

        Controls.Add(root);
        Controls.Add(strip);
    }

    private Control BuildTopSection()
    {
        var top = new TableLayoutPanel
        {
            Dock = DockStyle.Fill,
            ColumnCount = 2,
            RowCount = 1,
        };
        top.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 52));
        top.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 48));

        var runtimeGroup = new GroupBox { Text = "Onboarding and Runtime", Dock = DockStyle.Fill, Padding = new Padding(10) };
        var runtimeLayout = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 1, RowCount = 3 };
        runtimeLayout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        runtimeLayout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        runtimeLayout.RowStyles.Add(new RowStyle(SizeType.Percent, 100));

        _bootstrapLabel.AutoSize = true;
        _bootstrapLabel.Text = "Checking runtime...";
        runtimeLayout.Controls.Add(_bootstrapLabel, 0, 0);

        _updateLabel.AutoSize = true;
        _updateLabel.Text = "Checking updates...";
        runtimeLayout.Controls.Add(_updateLabel, 0, 1);

        _bootstrapList.Dock = DockStyle.Fill;
        _bootstrapList.View = View.Details;
        _bootstrapList.FullRowSelect = true;
        _bootstrapList.GridLines = true;
        _bootstrapList.Columns.Add("Component", 120);
        _bootstrapList.Columns.Add("Status", 110);
        _bootstrapList.Columns.Add("Message", 220);
        _bootstrapList.Columns.Add("Path", 420);
        runtimeLayout.Controls.Add(_bootstrapList, 0, 2);

        runtimeGroup.Controls.Add(runtimeLayout);

        var modelsGroup = new GroupBox { Text = "Models and Updates", Dock = DockStyle.Fill, Padding = new Padding(10) };
        var modelsLayout = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 2, RowCount = 7 };
        modelsLayout.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 160));
        modelsLayout.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
        for (var i = 0; i < 7; i++)
        {
            modelsLayout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        }

        modelsLayout.Controls.Add(new Label { Text = "Default model:", AutoSize = true, Margin = new Padding(0, 8, 3, 8) }, 0, 0);
        _defaultModelLabel.AutoSize = true;
        _defaultModelLabel.Margin = new Padding(3, 8, 3, 8);
        modelsLayout.Controls.Add(_defaultModelLabel, 1, 0);

        modelsLayout.Controls.Add(new Label { Text = "Models dir:", AutoSize = true, Margin = new Padding(0, 8, 3, 8) }, 0, 1);
        _modelsDirLabel.AutoEllipsis = true;
        _modelsDirLabel.Dock = DockStyle.Fill;
        _modelsDirLabel.Margin = new Padding(3, 8, 3, 8);
        modelsLayout.Controls.Add(_modelsDirLabel, 1, 1);

        modelsLayout.Controls.Add(new Label { Text = "Installed:", AutoSize = true, Margin = new Padding(0, 8, 3, 8) }, 0, 2);
        _installedModels.Dock = DockStyle.Fill;
        _installedModels.DropDownStyle = ComboBoxStyle.DropDownList;
        modelsLayout.Controls.Add(_installedModels, 1, 2);

        _setDefaultButton.Text = "Set as default";
        _setDefaultButton.AutoSize = true;
        _setDefaultButton.Click += async (_, _) => await SetDefaultModelAsync();
        modelsLayout.Controls.Add(_setDefaultButton, 1, 3);

        modelsLayout.Controls.Add(new Label { Text = "Preset:", AutoSize = true, Margin = new Padding(0, 8, 3, 8) }, 0, 4);
        _presetModels.Dock = DockStyle.Fill;
        _presetModels.DropDownStyle = ComboBoxStyle.DropDownList;
        modelsLayout.Controls.Add(_presetModels, 1, 4);

        _installModelButton.Text = "Download model";
        _installModelButton.AutoSize = true;
        _installModelButton.Click += async (_, _) => await InstallModelAsync();
        modelsLayout.Controls.Add(_installModelButton, 1, 5);

        var updatesPanel = new FlowLayoutPanel { Dock = DockStyle.Fill, AutoSize = true, WrapContents = false };
        _checkUpdatesButton.Text = "Check updates";
        _checkUpdatesButton.AutoSize = true;
        _checkUpdatesButton.Click += async (_, _) => await CheckUpdatesAsync();
        _refreshButton.Text = "Refresh now";
        _refreshButton.AutoSize = true;
        _refreshButton.Click += async (_, _) => await RefreshAllAsync(force: true);
        updatesPanel.Controls.Add(_checkUpdatesButton);
        updatesPanel.Controls.Add(_refreshButton);
        modelsLayout.Controls.Add(updatesPanel, 1, 6);

        modelsGroup.Controls.Add(modelsLayout);

        top.Controls.Add(runtimeGroup, 0, 0);
        top.Controls.Add(modelsGroup, 1, 0);
        return top;
    }

    private Control BuildQueueSection()
    {
        var queueGroup = new GroupBox { Text = "Queue", Dock = DockStyle.Fill, Padding = new Padding(10) };
        var queueLayout = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 8, RowCount = 1 };
        queueLayout.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));
        queueLayout.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 120));
        queueLayout.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));
        queueLayout.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
        queueLayout.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 160));
        queueLayout.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 140));
        queueLayout.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 140));
        queueLayout.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 160));

        queueLayout.Controls.Add(new Label { Text = "Language:", AutoSize = true, Margin = new Padding(0, 9, 8, 0) }, 0, 0);
        _language.Text = "auto";
        _language.Dock = DockStyle.Fill;
        queueLayout.Controls.Add(_language, 1, 0);

        queueLayout.Controls.Add(new Label { Text = "Model:", AutoSize = true, Margin = new Padding(8, 9, 8, 0) }, 2, 0);
        _runModel.Dock = DockStyle.Fill;
        _runModel.DropDownStyle = ComboBoxStyle.DropDown;
        queueLayout.Controls.Add(_runModel, 3, 0);

        _addFilesButton.Text = "Add files";
        _addFilesButton.Dock = DockStyle.Fill;
        _addFilesButton.Click += async (_, _) => await AddFilesAsync();
        queueLayout.Controls.Add(_addFilesButton, 4, 0);

        _cancelJobButton.Text = "Cancel job";
        _cancelJobButton.Dock = DockStyle.Fill;
        _cancelJobButton.Click += async (_, _) => await CancelSelectedJobAsync();
        queueLayout.Controls.Add(_cancelJobButton, 5, 0);

        _retryJobButton.Text = "Retry job";
        _retryJobButton.Dock = DockStyle.Fill;
        _retryJobButton.Click += async (_, _) => await RetrySelectedJobAsync();
        queueLayout.Controls.Add(_retryJobButton, 6, 0);

        _openResultButton.Text = "Open result";
        _openResultButton.Dock = DockStyle.Fill;
        _openResultButton.Click += (_, _) => OpenSelectedResult();
        queueLayout.Controls.Add(_openResultButton, 7, 0);

        queueGroup.Controls.Add(queueLayout);
        return queueGroup;
    }

    private Control BuildJobsSection()
    {
        var jobsGroup = new GroupBox { Text = "Jobs", Dock = DockStyle.Fill, Padding = new Padding(10) };
        _jobsGrid.Dock = DockStyle.Fill;
        _jobsGrid.AllowUserToAddRows = false;
        _jobsGrid.AllowUserToDeleteRows = false;
        _jobsGrid.ReadOnly = true;
        _jobsGrid.MultiSelect = false;
        _jobsGrid.SelectionMode = DataGridViewSelectionMode.FullRowSelect;
        _jobsGrid.AutoSizeColumnsMode = DataGridViewAutoSizeColumnsMode.Fill;
        _jobsGrid.RowHeadersVisible = false;
        _jobsGrid.EnableHeadersVisualStyles = false;

        _jobsGrid.Columns.Add(new DataGridViewTextBoxColumn { Name = "ID", DataPropertyName = "Id", FillWeight = 110 });
        _jobsGrid.Columns.Add(new DataGridViewTextBoxColumn { Name = "Status", DataPropertyName = "Status", FillWeight = 70 });
        _jobsGrid.Columns.Add(new DataGridViewTextBoxColumn { Name = "Progress", DataPropertyName = "Progress", FillWeight = 55 });
        _jobsGrid.Columns.Add(new DataGridViewTextBoxColumn { Name = "Model", DataPropertyName = "Model", FillWeight = 90 });
        _jobsGrid.Columns.Add(new DataGridViewTextBoxColumn { Name = "File", DataPropertyName = "FilePath", FillWeight = 190 });
        _jobsGrid.Columns.Add(new DataGridViewTextBoxColumn { Name = "Message", DataPropertyName = "Message", FillWeight = 150 });

        jobsGroup.Controls.Add(_jobsGrid);
        return jobsGroup;
    }

    private async Task RefreshAllAsync(bool force = false)
    {
        if (_refreshInProgress && !force)
        {
            return;
        }
        if (_api is null)
        {
            return;
        }

        _refreshInProgress = true;
        try
        {
            var ct = _lifetimeCts.Token;

            var health = await _api.HealthAsync(ct);
            _connectionLabel.Text = $"{_daemon.BaseUrl} | core version: {health.Version}";

            var bootstrap = await _api.GetBootstrapStatusAsync(ct);
            UpdateBootstrapUi(bootstrap);

            if (!bootstrap.Ready && !bootstrap.InProgress)
            {
                await _api.EnsureBootstrapAsync(ct);
            }

            var update = await _api.GetUpdateStatusAsync(ct);
            UpdateUpdateUi(update);

            var models = await _api.GetModelsAsync(ct);
            UpdateModelsUi(models, bootstrap.Ready);

            var presets = await _api.GetPresetsAsync(ct);
            UpdatePresetUi(presets.Presets);

            var jobs = await _api.GetJobsAsync(ct);
            _jobs = jobs.Jobs
                .OrderByDescending(j => j.CreatedAt)
                .ToList();
            UpdateJobsGrid();
        }
        catch (OperationCanceledException)
        {
            // no-op
        }
        catch (Exception ex)
        {
            SetStatus($"Error: {ex.Message}", isError: true);
        }
        finally
        {
            _refreshInProgress = false;
        }
    }

    private void UpdateBootstrapUi(BootstrapStatus bootstrap)
    {
        _bootstrapList.BeginUpdate();
        _bootstrapList.Items.Clear();
        foreach (var component in bootstrap.Components)
        {
            var item = new ListViewItem(component.Name ?? "-");
            item.SubItems.Add(component.Status ?? "-");
            item.SubItems.Add(component.Message ?? "");
            item.SubItems.Add(component.Path ?? "");
            _bootstrapList.Items.Add(item);
        }
        _bootstrapList.EndUpdate();

        if (bootstrap.Ready)
        {
            _bootstrapLabel.Text = "Runtime is ready";
            _bootstrapLabel.ForeColor = Color.FromArgb(74, 222, 128);
        }
        else if (!string.IsNullOrWhiteSpace(bootstrap.Error))
        {
            _bootstrapLabel.Text = "Runtime error: " + bootstrap.Error;
            _bootstrapLabel.ForeColor = Color.FromArgb(248, 113, 113);
        }
        else if (bootstrap.InProgress)
        {
            _bootstrapLabel.Text = "Preparing runtime...";
            _bootstrapLabel.ForeColor = Color.FromArgb(251, 191, 36);
        }
        else
        {
            _bootstrapLabel.Text = "Runtime not ready, starting automatic setup...";
            _bootstrapLabel.ForeColor = Color.FromArgb(251, 191, 36);
        }
    }

    private void UpdateUpdateUi(UpdateStatus update)
    {
        if (!update.Enabled)
        {
            _updateLabel.Text = "Auto-update is disabled";
            _updateLabel.ForeColor = MutedTextColor;
            return;
        }

        if (!string.IsNullOrWhiteSpace(update.Error))
        {
            _updateLabel.Text = "Update error: " + update.Error;
            _updateLabel.ForeColor = Color.FromArgb(248, 113, 113);
            return;
        }

        var parts = new List<string>();
        if (!string.IsNullOrWhiteSpace(update.CurrentVersion))
        {
            parts.Add("Current: " + update.CurrentVersion);
        }
        if (!string.IsNullOrWhiteSpace(update.LatestVersion))
        {
            parts.Add("Latest: " + update.LatestVersion);
        }
        if (!string.IsNullOrWhiteSpace(update.Message))
        {
            parts.Add(update.Message);
        }

        _updateLabel.Text = parts.Count == 0 ? "No update data yet" : string.Join(" | ", parts);
        _updateLabel.ForeColor = update.UpdateAvailable ? Color.FromArgb(251, 191, 36) : Color.FromArgb(74, 222, 128);
    }

    private void UpdateModelsUi(ModelsResponse models, bool runtimeReady)
    {
        var defaultModel = models.DefaultModel ?? string.Empty;
        _defaultModelLabel.Text = string.IsNullOrWhiteSpace(defaultModel) ? "-" : defaultModel;
        _modelsDirLabel.Text = models.ModelsDir ?? "-";

        var names = models.Models
            .Select(m => m.Name)
            .Where(n => !string.IsNullOrWhiteSpace(n))
            .Select(n => n!)
            .Distinct(StringComparer.OrdinalIgnoreCase)
            .OrderBy(n => n, StringComparer.OrdinalIgnoreCase)
            .ToList();

        ReplaceComboItems(_installedModels, names, defaultModel);

        var runModelPreferred = string.IsNullOrWhiteSpace(_runModel.Text) ? defaultModel : _runModel.Text.Trim();
        ReplaceComboItems(_runModel, names, runModelPreferred, allowCustomText: true);

        _addFilesButton.Enabled = runtimeReady;
        _setDefaultButton.Enabled = names.Count > 0;
    }

    private void UpdatePresetUi(List<ModelPreset> presets)
    {
        var selected = _presetModels.SelectedItem as PresetItem;
        var selectedName = selected?.Name;

        _presetModels.BeginUpdate();
        _presetModels.Items.Clear();
        foreach (var preset in presets)
        {
            if (string.IsNullOrWhiteSpace(preset.Name))
            {
                continue;
            }
            _presetModels.Items.Add(new PresetItem(preset.Name!, preset.Alias));
        }
        _presetModels.EndUpdate();

        if (_presetModels.Items.Count == 0)
        {
            _installModelButton.Enabled = false;
            return;
        }

        _installModelButton.Enabled = !_modelInstallInProgress;
        if (!string.IsNullOrWhiteSpace(selectedName))
        {
            foreach (var item in _presetModels.Items)
            {
                if (item is PresetItem p && string.Equals(p.Name, selectedName, StringComparison.OrdinalIgnoreCase))
                {
                    _presetModels.SelectedItem = item;
                    return;
                }
            }
        }
        _presetModels.SelectedIndex = 0;
    }

    private void UpdateJobsGrid()
    {
        var selectedId = GetSelectedJobId();

        _jobsGrid.Rows.Clear();
        foreach (var job in _jobs)
        {
            _jobsGrid.Rows.Add(
                job.Id,
                job.Status,
                job.Progress + "%",
                job.Model,
                job.FilePath,
                BuildJobMessage(job)
            );
        }

        if (!string.IsNullOrWhiteSpace(selectedId))
        {
            foreach (DataGridViewRow row in _jobsGrid.Rows)
            {
                if (string.Equals(Convert.ToString(row.Cells[0].Value), selectedId, StringComparison.OrdinalIgnoreCase))
                {
                    row.Selected = true;
                    break;
                }
            }
        }
    }

    private async Task AddFilesAsync()
    {
        if (_api is null)
        {
            return;
        }

        using var dialog = new OpenFileDialog
        {
            Multiselect = true,
            CheckFileExists = true,
            Filter = "Media files|*.mp3;*.wav;*.m4a;*.aac;*.flac;*.ogg;*.opus;*.mp4;*.mkv;*.mov;*.avi;*.webm;*.m4v|All files|*.*",
            Title = "Select audio/video files",
        };

        if (dialog.ShowDialog(this) != DialogResult.OK || dialog.FileNames.Length == 0)
        {
            return;
        }

        var language = string.IsNullOrWhiteSpace(_language.Text) ? "auto" : _language.Text.Trim();
        var defaultModel = _defaultModelLabel.Text == "-" ? string.Empty : (_defaultModelLabel.Text ?? string.Empty);
        var model = string.IsNullOrWhiteSpace(_runModel.Text) ? defaultModel : _runModel.Text.Trim();
        if (model == "-")
        {
            model = string.Empty;
        }

        try
        {
            SetStatus($"Queueing {dialog.FileNames.Length} file(s)...");
            foreach (var filePath in dialog.FileNames)
            {
                await _api.AddJobAsync(new AddJobRequest
                {
                    FilePath = filePath,
                    Language = language,
                    Model = model,
                }, _lifetimeCts.Token);
            }
            await RefreshAllAsync(force: true);
            SetStatus("Files were queued");
        }
        catch (Exception ex)
        {
            SetStatus($"Failed to add file: {ex.Message}", isError: true);
        }
    }

    private async Task SetDefaultModelAsync()
    {
        if (_api is null)
        {
            return;
        }

        var model = _installedModels.SelectedItem as string;
        if (string.IsNullOrWhiteSpace(model))
        {
            return;
        }

        try
        {
            await _api.SetDefaultModelAsync(model, _lifetimeCts.Token);
            await RefreshAllAsync(force: true);
            SetStatus("Default model updated: " + model);
        }
        catch (Exception ex)
        {
            SetStatus("Failed to change default model: " + ex.Message, isError: true);
        }
    }

    private async Task InstallModelAsync()
    {
        if (_api is null)
        {
            return;
        }
        if (_modelInstallInProgress)
        {
            return;
        }

        if (_presetModels.SelectedItem is not PresetItem preset)
        {
            return;
        }

        try
        {
            _modelInstallInProgress = true;
            _installModelButton.Enabled = false;
            UseWaitCursor = true;
            SetStatus("Downloading model (can take several minutes): " + preset.Name);
            await _api.InstallModelAsync(preset.Name, _lifetimeCts.Token);
            await RefreshAllAsync(force: true);
            SetStatus("Model installed: " + preset.Name);
        }
        catch (Exception ex)
        {
            SetStatus("Model install error: " + ex.Message, isError: true);
        }
        finally
        {
            _modelInstallInProgress = false;
            UseWaitCursor = false;
            _installModelButton.Enabled = _presetModels.Items.Count > 0;
        }
    }

    private async Task CheckUpdatesAsync()
    {
        if (_api is null)
        {
            return;
        }

        try
        {
            SetStatus("Checking updates...");
            await _api.CheckUpdatesAsync(_lifetimeCts.Token);
            await RefreshAllAsync(force: true);
            SetStatus("Update check complete");
        }
        catch (Exception ex)
        {
            SetStatus("Update check error: " + ex.Message, isError: true);
        }
    }

    private async Task CancelSelectedJobAsync()
    {
        if (_api is null)
        {
            return;
        }

        var id = GetSelectedJobId();
        if (string.IsNullOrWhiteSpace(id))
        {
            return;
        }

        try
        {
            await _api.CancelJobAsync(id, _lifetimeCts.Token);
            await RefreshAllAsync(force: true);
            SetStatus("Job canceled: " + id);
        }
        catch (Exception ex)
        {
            SetStatus("Cancel error: " + ex.Message, isError: true);
        }
    }

    private async Task RetrySelectedJobAsync()
    {
        if (_api is null)
        {
            return;
        }

        var id = GetSelectedJobId();
        if (string.IsNullOrWhiteSpace(id))
        {
            return;
        }

        try
        {
            await _api.RetryJobAsync(id, _lifetimeCts.Token);
            await RefreshAllAsync(force: true);
            SetStatus("Job re-queued: " + id);
        }
        catch (Exception ex)
        {
            SetStatus("Retry error: " + ex.Message, isError: true);
        }
    }

    private void OpenSelectedResult()
    {
        var job = GetSelectedJob();
        if (job is null)
        {
            return;
        }

        var candidates = new[] { job.ResultText, job.ResultSrt, job.ResultVtt }
            .Where(path => !string.IsNullOrWhiteSpace(path) && File.Exists(path))
            .Cast<string>()
            .ToList();

        if (candidates.Count > 0)
        {
            OpenInExplorer(candidates[0]);
            return;
        }

        if (!string.IsNullOrWhiteSpace(job.OutputDir) && Directory.Exists(job.OutputDir))
        {
            OpenInExplorer(job.OutputDir, isDirectory: true);
            return;
        }

        SetStatus("Result is not available yet", isError: true);
    }

    private JobDto? GetSelectedJob()
    {
        var id = GetSelectedJobId();
        if (string.IsNullOrWhiteSpace(id))
        {
            return null;
        }
        return _jobs.FirstOrDefault(j => string.Equals(j.Id, id, StringComparison.OrdinalIgnoreCase));
    }

    private string? GetSelectedJobId()
    {
        if (_jobsGrid.SelectedRows.Count == 0)
        {
            return null;
        }
        return Convert.ToString(_jobsGrid.SelectedRows[0].Cells[0].Value);
    }

    private static string BuildJobMessage(JobDto job)
    {
        if (!string.IsNullOrWhiteSpace(job.Error))
        {
            return "error: " + job.Error;
        }
        return job.Message ?? string.Empty;
    }

    private void SetStatus(string text, bool isError = false)
    {
        _status.Text = text;
        _status.ForeColor = isError ? Color.FromArgb(248, 113, 113) : TextColor;
    }

    private void ApplyTheme()
    {
        Font = new Font("Segoe UI", 9.5F, FontStyle.Regular, GraphicsUnit.Point);
        BackColor = BackgroundColor;
        ForeColor = TextColor;

        ApplyThemeRecursive(this);
        _connectionLabel.ForeColor = TextColor;
        _defaultModelLabel.ForeColor = TextColor;
        _modelsDirLabel.ForeColor = TextColor;

        ConfigureButton(_addFilesButton, AccentColor, AccentHoverColor);
        ConfigureButton(_installModelButton, SuccessColor, SuccessHoverColor);
        ConfigureButton(_setDefaultButton, NeutralButtonColor, NeutralButtonHoverColor);
        ConfigureButton(_checkUpdatesButton, NeutralButtonColor, NeutralButtonHoverColor);
        ConfigureButton(_refreshButton, NeutralButtonColor, NeutralButtonHoverColor);
        ConfigureButton(_cancelJobButton, NeutralButtonColor, NeutralButtonHoverColor);
        ConfigureButton(_retryJobButton, NeutralButtonColor, NeutralButtonHoverColor);
        ConfigureButton(_openResultButton, NeutralButtonColor, NeutralButtonHoverColor);

        _jobsGrid.BackgroundColor = PanelColor;
        _jobsGrid.BorderStyle = BorderStyle.None;
        _jobsGrid.CellBorderStyle = DataGridViewCellBorderStyle.SingleHorizontal;
        _jobsGrid.ColumnHeadersDefaultCellStyle.BackColor = InputColor;
        _jobsGrid.ColumnHeadersDefaultCellStyle.ForeColor = TextColor;
        _jobsGrid.ColumnHeadersDefaultCellStyle.SelectionBackColor = InputColor;
        _jobsGrid.ColumnHeadersDefaultCellStyle.SelectionForeColor = TextColor;
        _jobsGrid.DefaultCellStyle.BackColor = PanelColor;
        _jobsGrid.DefaultCellStyle.ForeColor = TextColor;
        _jobsGrid.DefaultCellStyle.SelectionBackColor = Color.FromArgb(3, 105, 161);
        _jobsGrid.DefaultCellStyle.SelectionForeColor = Color.White;
        _jobsGrid.AlternatingRowsDefaultCellStyle.BackColor = Color.FromArgb(34, 49, 76);
        _jobsGrid.GridColor = InputColor;

        _bootstrapList.BorderStyle = BorderStyle.None;
        _bootstrapList.BackColor = PanelColor;
        _bootstrapList.ForeColor = TextColor;

        if (Controls.OfType<StatusStrip>().FirstOrDefault() is StatusStrip strip)
        {
            strip.BackColor = PanelColor;
            strip.ForeColor = TextColor;
        }
    }

    private void ApplyThemeRecursive(Control control)
    {
        switch (control)
        {
            case GroupBox box:
                box.BackColor = PanelColor;
                box.ForeColor = TextColor;
                break;
            case TableLayoutPanel table:
                table.BackColor = table.Parent is GroupBox ? PanelColor : BackgroundColor;
                table.ForeColor = TextColor;
                break;
            case FlowLayoutPanel flow:
                flow.BackColor = PanelColor;
                flow.ForeColor = TextColor;
                break;
            case Label label:
                label.ForeColor = MutedTextColor;
                break;
            case TextBox textBox:
                textBox.BackColor = InputColor;
                textBox.ForeColor = TextColor;
                textBox.BorderStyle = BorderStyle.FixedSingle;
                break;
            case ComboBox combo:
                combo.FlatStyle = FlatStyle.Flat;
                combo.BackColor = InputColor;
                combo.ForeColor = TextColor;
                break;
            case StatusStrip strip:
                strip.BackColor = PanelColor;
                strip.ForeColor = TextColor;
                break;
        }

        foreach (Control child in control.Controls)
        {
            ApplyThemeRecursive(child);
        }
    }

    private static void ConfigureButton(Button button, Color baseColor, Color hoverColor)
    {
        button.FlatStyle = FlatStyle.Flat;
        button.FlatAppearance.BorderSize = 0;
        button.FlatAppearance.MouseOverBackColor = hoverColor;
        button.FlatAppearance.MouseDownBackColor = hoverColor;
        button.BackColor = baseColor;
        button.ForeColor = Color.White;
        button.Padding = new Padding(8, 6, 8, 6);
    }

    private static void ReplaceComboItems(ComboBox combo, List<string> values, string? preferred, bool allowCustomText = false)
    {
        var unique = values
            .Where(v => !string.IsNullOrWhiteSpace(v))
            .Distinct(StringComparer.OrdinalIgnoreCase)
            .OrderBy(v => v, StringComparer.OrdinalIgnoreCase)
            .ToList();

        var selectedText = string.IsNullOrWhiteSpace(preferred) ? combo.Text : preferred.Trim();

        combo.BeginUpdate();
        combo.Items.Clear();
        foreach (var value in unique)
        {
            combo.Items.Add(value);
        }
        combo.EndUpdate();

        combo.DropDownStyle = allowCustomText ? ComboBoxStyle.DropDown : ComboBoxStyle.DropDownList;

        if (!string.IsNullOrWhiteSpace(selectedText))
        {
            var idx = combo.FindStringExact(selectedText);
            if (idx >= 0)
            {
                combo.SelectedIndex = idx;
            }
            else if (allowCustomText)
            {
                combo.Text = selectedText;
            }
            else if (combo.Items.Count > 0)
            {
                combo.SelectedIndex = 0;
            }
            return;
        }

        if (combo.Items.Count > 0 && combo.SelectedIndex < 0)
        {
            combo.SelectedIndex = 0;
        }
    }

    private static void OpenInExplorer(string path, bool isDirectory = false)
    {
        if (string.IsNullOrWhiteSpace(path))
        {
            return;
        }

        if (isDirectory)
        {
            Process.Start(new ProcessStartInfo
            {
                FileName = "explorer.exe",
                Arguments = $"\"{path}\"",
                UseShellExecute = true,
            });
            return;
        }

        Process.Start(new ProcessStartInfo
        {
            FileName = "explorer.exe",
            Arguments = $"/select,\"{path}\"",
            UseShellExecute = true,
        });
    }

    private sealed class PresetItem
    {
        public string Name { get; }
        public string? Alias { get; }

        public PresetItem(string name, string? alias)
        {
            Name = name;
            Alias = alias;
        }

        public override string ToString()
        {
            if (!string.IsNullOrWhiteSpace(Alias) && !string.Equals(Alias, Name, StringComparison.OrdinalIgnoreCase))
            {
                return $"{Alias} ({Name})";
            }
            return Name;
        }
    }
}
