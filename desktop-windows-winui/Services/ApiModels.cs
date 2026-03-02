using System;
using System.Collections.Generic;
using System.Text.Json.Serialization;

namespace TranscribeDesktop.WinUI.Services;

public sealed class ApiErrorResponse
{
    [JsonPropertyName("error")]
    public string? Error { get; set; }
}

public sealed class HealthResponse
{
    [JsonPropertyName("status")]
    public string? Status { get; set; }

    [JsonPropertyName("service")]
    public string? Service { get; set; }

    [JsonPropertyName("version")]
    public string? Version { get; set; }
}

public sealed class BootstrapStatus
{
    [JsonPropertyName("ready")]
    public bool Ready { get; set; }

    [JsonPropertyName("inProgress")]
    public bool InProgress { get; set; }

    [JsonPropertyName("error")]
    public string? Error { get; set; }

    [JsonPropertyName("components")]
    public List<ComponentStatus> Components { get; set; } = new();
}

public sealed class ComponentStatus
{
    [JsonPropertyName("name")]
    public string? Name { get; set; }

    [JsonPropertyName("status")]
    public string? Status { get; set; }

    [JsonPropertyName("message")]
    public string? Message { get; set; }

    [JsonPropertyName("path")]
    public string? Path { get; set; }
}

public sealed class UpdateStatus
{
    [JsonPropertyName("enabled")]
    public bool Enabled { get; set; }

    [JsonPropertyName("inProgress")]
    public bool InProgress { get; set; }

    [JsonPropertyName("currentVersion")]
    public string? CurrentVersion { get; set; }

    [JsonPropertyName("latestVersion")]
    public string? LatestVersion { get; set; }

    [JsonPropertyName("updateAvailable")]
    public bool UpdateAvailable { get; set; }

    [JsonPropertyName("downloaded")]
    public bool Downloaded { get; set; }

    [JsonPropertyName("message")]
    public string? Message { get; set; }

    [JsonPropertyName("error")]
    public string? Error { get; set; }
}

public sealed class ModelsResponse
{
    [JsonPropertyName("defaultModel")]
    public string? DefaultModel { get; set; }

    [JsonPropertyName("modelsDir")]
    public string? ModelsDir { get; set; }

    [JsonPropertyName("models")]
    public List<ModelInfo> Models { get; set; } = new();
}

public sealed class ModelInfo
{
    [JsonPropertyName("name")]
    public string? Name { get; set; }

    [JsonPropertyName("path")]
    public string? Path { get; set; }

    [JsonPropertyName("sizeBytes")]
    public long SizeBytes { get; set; }
}

public sealed class PresetsResponse
{
    [JsonPropertyName("presets")]
    public List<ModelPreset> Presets { get; set; } = new();
}

public sealed class ModelPreset
{
    [JsonPropertyName("name")]
    public string? Name { get; set; }

    [JsonPropertyName("alias")]
    public string? Alias { get; set; }

    [JsonPropertyName("sizeBytes")]
    public long SizeBytes { get; set; }
}

public sealed class JobsResponse
{
    [JsonPropertyName("jobs")]
    public List<JobDto> Jobs { get; set; } = new();
}

public sealed class JobDto
{
    [JsonPropertyName("id")]
    public string? Id { get; set; }

    [JsonPropertyName("filePath")]
    public string? FilePath { get; set; }

    [JsonPropertyName("outputDir")]
    public string? OutputDir { get; set; }

    [JsonPropertyName("language")]
    public string? Language { get; set; }

    [JsonPropertyName("model")]
    public string? Model { get; set; }

    [JsonPropertyName("status")]
    public string? Status { get; set; }

    [JsonPropertyName("progress")]
    public int Progress { get; set; }

    [JsonPropertyName("message")]
    public string? Message { get; set; }

    [JsonPropertyName("error")]
    public string? Error { get; set; }

    [JsonPropertyName("resultText")]
    public string? ResultText { get; set; }

    [JsonPropertyName("resultSrt")]
    public string? ResultSrt { get; set; }

    [JsonPropertyName("resultVtt")]
    public string? ResultVtt { get; set; }

    [JsonPropertyName("createdAt")]
    public DateTimeOffset CreatedAt { get; set; }
}

public sealed class AddJobRequest
{
    [JsonPropertyName("filePath")]
    public string FilePath { get; set; } = string.Empty;

    [JsonPropertyName("outputDir")]
    public string OutputDir { get; set; } = string.Empty;

    [JsonPropertyName("language")]
    public string Language { get; set; } = "auto";

    [JsonPropertyName("model")]
    public string Model { get; set; } = string.Empty;
}

public sealed class ModelUseRequest
{
    [JsonPropertyName("name")]
    public string Name { get; set; } = string.Empty;
}

public sealed class ModelInstallRequest
{
    [JsonPropertyName("name")]
    public string Name { get; set; } = string.Empty;
}
