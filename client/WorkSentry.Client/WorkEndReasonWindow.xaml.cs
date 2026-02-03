using System.Collections.Generic;
using System.Linq;
using System.Windows;
using System.Windows.Controls;

namespace WorkSentry.Client;

internal sealed partial class WorkEndReasonWindow : Window
{
    public string? Reason { get; private set; }

    public WorkEndReasonWindow(IReadOnlyList<string> messages)
    {
        InitializeComponent();
        ViolationsList.ItemsSource = messages?.Where(x => !string.IsNullOrWhiteSpace(x)).ToList() ?? new List<string>();
        CancelButton.Click += (_, _) => Close();
        ConfirmButton.Click += (_, _) => Confirm();
        ReasonTextBox.TextChanged += (_, _) => UpdateState();
        UpdateState();
    }

    private void Confirm()
    {
        var text = ReasonTextBox.Text.Trim();
        if (string.IsNullOrWhiteSpace(text))
        {
            ErrorText.Text = LanguageService.GetString("WorkEndReasonError");
            return;
        }
        Reason = text;
        DialogResult = true;
        Close();
    }

    private void UpdateState()
    {
        var hasText = !string.IsNullOrWhiteSpace(ReasonTextBox.Text);
        ConfirmButton.IsEnabled = hasText;
        ErrorText.Text = string.Empty;
        PlaceholderText.Visibility = hasText ? Visibility.Collapsed : Visibility.Visible;
    }
}