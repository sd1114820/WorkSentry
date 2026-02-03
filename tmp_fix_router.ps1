$path = "internal/http/router.go"
$content = Get-Content -Raw -Path $path
$content = $content -replace '`tmux', "`tmux"
Set-Content -Path $path -Value $content -Encoding utf8
