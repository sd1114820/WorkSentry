using System;
using System.Collections.Generic;
using System.Globalization;
using System.Windows;
using System.Windows.Controls;
using WpfTextBox = System.Windows.Controls.TextBox;
using WpfComboBox = System.Windows.Controls.ComboBox;
using System.Windows.Documents;
using System.Windows.Media;
using MediaColor = System.Windows.Media.Color;
using MediaSolidColorBrush = System.Windows.Media.SolidColorBrush;

namespace WorkSentry.Client;

internal sealed partial class CheckoutFormWindow : Window
{
    private const int TextMaxLength = 1000;
    private static readonly MediaSolidColorBrush LabelBrush = new(MediaColor.FromRgb(75, 85, 99));
    private static readonly MediaSolidColorBrush RequiredBrush = new(MediaColor.FromRgb(220, 38, 38));
    private readonly CheckoutTemplate _template;
    private readonly List<FieldInput> _inputs = new();

    public ClientCheckoutPayload? Result { get; private set; }

    public CheckoutFormWindow(CheckoutTemplate template)
    {
        InitializeComponent();
        _template = template;
        TemplateNameText.Text = string.IsNullOrWhiteSpace(template.Name) ? "-" : template.Name;
        CancelButton.Click += (_, _) => Close();
        ConfirmButton.Click += (_, _) => Confirm();
        BuildFields();
        UpdateConfirmState(false);
    }

    private void BuildFields()
    {
        if (_template.Fields == null || _template.Fields.Count == 0)
        {
            NoFieldsText.Visibility = Visibility.Visible;
            return;
        }

        foreach (var field in _template.Fields)
        {
            var container = new StackPanel
            {
                Margin = new Thickness(0, 0, 0, 16)
            };

            var label = new TextBlock
            {
                FontWeight = FontWeights.SemiBold,
                FontSize = 13,
                Foreground = LabelBrush
            };
            label.Inlines.Add(new Run(field.Name));
            if (field.Required)
            {
                label.Inlines.Add(new Run(" *") { Foreground = RequiredBrush });
            }
            container.Children.Add(label);

            FrameworkElement inputElement;
            if (string.Equals(field.Type, "text", StringComparison.OrdinalIgnoreCase))
            {
                var textBox = new WpfTextBox
                {
                    Style = (Style)FindResource("InputBox"),
                    Height = 90,
                    AcceptsReturn = true,
                    TextWrapping = TextWrapping.Wrap,
                    VerticalScrollBarVisibility = ScrollBarVisibility.Auto,
                    MaxLength = TextMaxLength
                };
                textBox.TextChanged += (_, _) => UpdateConfirmState(false);
                inputElement = textBox;
                _inputs.Add(new FieldInput(field, textBox, null));
            }
            else if (string.Equals(field.Type, "number", StringComparison.OrdinalIgnoreCase))
            {
                var numberBox = new WpfTextBox
                {
                    Style = (Style)FindResource("InputBox"),
                    Height = 36
                };
                numberBox.TextChanged += (_, _) => UpdateConfirmState(false);
                inputElement = numberBox;
                _inputs.Add(new FieldInput(field, numberBox, null));
            }
            else if (string.Equals(field.Type, "select", StringComparison.OrdinalIgnoreCase))
            {
                var combo = new WpfComboBox
                {
                    Style = (Style)FindResource("InputCombo"),
                    Height = 36,
                    IsEditable = false,
                    ItemsSource = field.Options ?? new List<string>()
                };
                combo.SelectionChanged += (_, _) => UpdateConfirmState(false);
                inputElement = combo;
                _inputs.Add(new FieldInput(field, null, combo));
            }
            else
            {
                var fallback = new WpfTextBox
                {
                    Style = (Style)FindResource("InputBox"),
                    Height = 36
                };
                fallback.TextChanged += (_, _) => UpdateConfirmState(false);
                inputElement = fallback;
                _inputs.Add(new FieldInput(field, fallback, null));
            }

            container.Children.Add(inputElement);
            FieldsPanel.Children.Add(container);
        }
    }

    private void Confirm()
    {
        if (TryBuildPayload(out var payload, out var error))
        {
            Result = payload;
            DialogResult = true;
            Close();
            return;
        }
        ErrorText.Text = error;
    }

    private void UpdateConfirmState(bool showError)
    {
        if (TryBuildPayload(out _, out var error))
        {
            ConfirmButton.IsEnabled = true;
            if (!showError)
            {
                ErrorText.Text = string.Empty;
            }
            return;
        }

        ConfirmButton.IsEnabled = false;
        if (showError)
        {
            ErrorText.Text = error;
        }
        else
        {
            ErrorText.Text = string.Empty;
        }
    }

    private bool TryBuildPayload(out ClientCheckoutPayload payload, out string error)
    {
        payload = new ClientCheckoutPayload
        {
            TemplateId = _template.TemplateId,
            Data = new Dictionary<string, string>()
        };
        error = string.Empty;

        foreach (var input in _inputs)
        {
            var field = input.Field;
            var value = input.GetValue();

            if (string.IsNullOrWhiteSpace(value))
            {
                if (field.Required)
                {
                    error = LanguageService.Format("CheckoutErrorRequired", field.Name);
                    return false;
                }
                continue;
            }

            value = value.Trim();
            if (string.Equals(field.Type, "text", StringComparison.OrdinalIgnoreCase))
            {
                if (new StringInfo(value).LengthInTextElements > TextMaxLength)
                {
                    error = LanguageService.Format("CheckoutErrorLength", field.Name, TextMaxLength);
                    return false;
                }
            }
            else if (string.Equals(field.Type, "number", StringComparison.OrdinalIgnoreCase))
            {
                if (!TryParseNumber(value))
                {
                    error = LanguageService.Format("CheckoutErrorNumber", field.Name);
                    return false;
                }
            }
            else if (string.Equals(field.Type, "select", StringComparison.OrdinalIgnoreCase))
            {
                if (!OptionExists(field.Options, value))
                {
                    error = LanguageService.Format("CheckoutErrorSelect", field.Name);
                    return false;
                }
            }

            payload.Data[field.Id.ToString()] = value;
        }

        return true;
    }

    private static bool TryParseNumber(string value)
    {
        return double.TryParse(value, NumberStyles.Float, CultureInfo.CurrentCulture, out _)
               || double.TryParse(value, NumberStyles.Float, CultureInfo.InvariantCulture, out _);
    }

    private static bool OptionExists(List<string> options, string value)
    {
        if (options == null || options.Count == 0)
        {
            return false;
        }
        foreach (var option in options)
        {
            if (string.Equals(option?.Trim(), value, StringComparison.OrdinalIgnoreCase))
            {
                return true;
            }
        }
        return false;
    }

    private sealed class FieldInput
    {
        public CheckoutField Field { get; }
        public WpfTextBox? TextBox { get; }
        public WpfComboBox? ComboBox { get; }

        public FieldInput(CheckoutField field, WpfTextBox? textBox, WpfComboBox? comboBox)
        {
            Field = field;
            TextBox = textBox;
            ComboBox = comboBox;
        }

        public string GetValue()
        {
            if (TextBox != null)
            {
                return TextBox.Text;
            }
            if (ComboBox != null)
            {
                return ComboBox.SelectedItem?.ToString() ?? string.Empty;
            }
            return string.Empty;
        }
    }
}
