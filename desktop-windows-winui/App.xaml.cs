using Microsoft.UI.Xaml;
using System;
using System.Threading.Tasks;

namespace TranscribeDesktop.WinUI;

public partial class App : Application
{
    public static MainWindow? MainWindowRef { get; private set; }

    public Services.AppServices Services { get; } = new();

    public App()
    {
        InitializeComponent();
        TranscribeDesktop.WinUI.Services.StartupLog.Write("App ctor");

        UnhandledException += App_UnhandledException;
        AppDomain.CurrentDomain.UnhandledException += CurrentDomain_UnhandledException;
        TaskScheduler.UnobservedTaskException += TaskScheduler_UnobservedTaskException;
    }

    protected override void OnLaunched(LaunchActivatedEventArgs args)
    {
        try
        {
            TranscribeDesktop.WinUI.Services.StartupLog.Write("OnLaunched start");
            MainWindowRef = new MainWindow(this);
            MainWindowRef.Activate();
            TranscribeDesktop.WinUI.Services.StartupLog.Write("MainWindow activated");
        }
        catch (Exception ex)
        {
            TranscribeDesktop.WinUI.Services.StartupLog.Write(ex, "OnLaunched fatal");
            throw;
        }
    }

    private void App_UnhandledException(object sender, Microsoft.UI.Xaml.UnhandledExceptionEventArgs e)
    {
        TranscribeDesktop.WinUI.Services.StartupLog.Write(e.Exception, "Application.UnhandledException");
    }

    private void CurrentDomain_UnhandledException(object? sender, UnhandledExceptionEventArgs e)
    {
        if (e.ExceptionObject is Exception ex)
        {
            TranscribeDesktop.WinUI.Services.StartupLog.Write(ex, "AppDomain.UnhandledException");
        }
        else
        {
            TranscribeDesktop.WinUI.Services.StartupLog.Write("AppDomain.UnhandledException (non-exception object)");
        }
    }

    private void TaskScheduler_UnobservedTaskException(object? sender, UnobservedTaskExceptionEventArgs e)
    {
        TranscribeDesktop.WinUI.Services.StartupLog.Write(e.Exception, "TaskScheduler.UnobservedTaskException");
    }
}
