$path = "internal/http/handlers/client.go"
$lines = Get-Content -Path $path
$out = New-Object System.Collections.Generic.List[string]
$rewritten = $false
$inside = $false
foreach ($line in $lines) {
    if (-not $rewritten -and $line -match '^type ClientCheckoutPayload struct') {
        $rewritten = $true
        $inside = $true
        $out.Add('type ClientCheckoutPayload struct {')
        $out.Add('    TemplateID int64             `json:"templateId"`')
        $out.Add('    Data       map[string]string `json:"data"`')
        $out.Add('}')
        $out.Add('')
        $out.Add('type ClientReportRequest struct {')
        $out.Add('    ProcessName   string `json:"processName"`')
        $out.Add('    WindowTitle   string `json:"windowTitle"`')
        $out.Add('    IdleSeconds   int32  `json:"idleSeconds"`')
        $out.Add('    ClientVersion string `json:"clientVersion"`')
        $out.Add('    ReportType    string `json:"reportType"`')
        $out.Add('    Checkout      *ClientCheckoutPayload `json:"checkout"`')
        $out.Add('}')
        continue
    }
    if ($inside) {
        if ($line -match '^type ClientReportResponse struct') {
            $inside = $false
            $out.Add('')
            $out.Add($line)
        }
        continue
    }
    $out.Add($line)
}

Set-Content -Path $path -Value $out -Encoding utf8
