namespace TranscribeDesktop.WinUI.Models;

public sealed class UserSettings
{
    public bool OnboardingCompleted { get; set; }

    public bool AllowAnonymousDiagnostics { get; set; }

    public string PreferredModel { get; set; } = "ggml-large-v3-turbo";
}
