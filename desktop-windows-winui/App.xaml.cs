using Microsoft.UI.Xaml;

namespace TranscribeDesktop.WinUI;

public partial class App : Application
{
    public static MainWindow? MainWindowRef { get; private set; }

    public Services.AppServices Services { get; } = new();

    public App()
    {
        InitializeComponent();
    }

    protected override void OnLaunched(LaunchActivatedEventArgs args)
    {
        MainWindowRef = new MainWindow(this);
        MainWindowRef.Activate();
    }
}
