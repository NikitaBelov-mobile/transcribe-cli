using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Navigation;

namespace TranscribeDesktop.WinUI.Views;

public sealed partial class ErrorPage : Page
{
    public ErrorPage()
    {
        InitializeComponent();
    }

    protected override void OnNavigatedTo(NavigationEventArgs e)
    {
        base.OnNavigatedTo(e);
        ErrorText.Text = e.Parameter?.ToString() ?? "Unknown startup failure";
    }
}
