$path = "internal/http/handlers/client.go"
$lines = Get-Content -Path $path
$out = New-Object System.Collections.Generic.List[string]
for ($i = 0; $i -lt $lines.Count; $i++) {
    $line = $lines[$i]
    if ($line -match '^\twriteError\(w, http.StatusInternalServerError, err.Error\(\)\)' -and $i -gt 0 -and $i + 1 -lt $lines.Count) {
        $prev = $lines[$i - 1]
        $next = $lines[$i + 1]
        if ($prev -match '^\t\}' -and $next -match '^\treturn') {
            $i += 1
            continue
        }
    }
    $out.Add($line)
}
Set-Content -Path $path -Value $out -Encoding utf8
