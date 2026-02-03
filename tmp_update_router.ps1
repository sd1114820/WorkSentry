$path = "internal/http/router.go"
$lines = Get-Content -Path $path
$out = New-Object System.Collections.Generic.List[string]
foreach ($line in $lines) {
    $out.Add($line)
    if ($line -match '/api/v1/client/report') {
        $out.Add('`tmux.HandleFunc("/api/v1/client/checkout-template", h.ClientCheckoutTemplate)')
    }
    if ($line -match '/api/v1/admin/employees/unbind') {
        $out.Add('`tmux.HandleFunc("/api/v1/admin/checkout-templates", adminOnly(h.CheckoutTemplates))')
        $out.Add('`tmux.HandleFunc("/api/v1/admin/checkout-fields", adminOnly(h.CheckoutFields))')
    }
}
Set-Content -Path $path -Value $out -Encoding utf8
