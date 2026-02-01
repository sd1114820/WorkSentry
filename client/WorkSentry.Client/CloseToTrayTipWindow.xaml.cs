using System.Windows;

namespace WorkSentry.Client;

internal sealed partial class CloseToTrayTipWindow : Window
{
    public bool Remember => RememberCheck.IsChecked == true;

    public CloseToTrayTipWindow()
    {
        InitializeComponent();
        OkButton.Click += (_, _) =>
        {
            DialogResult = true;
            Close();
        };
    }
}
