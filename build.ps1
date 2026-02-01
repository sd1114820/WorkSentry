[CmdletBinding()]
param(
    [ValidateSet('web-backend','web-frontend','web','client','all')]
    [string]$Target = ''
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

if ($PSVersionTable.PSVersion.Major -lt 7) {
    Write-Host "需要 PowerShell 7 才能运行该脚本。当前版本: $($PSVersionTable.PSVersion)" -ForegroundColor Red
    exit 1
}

$RepoRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $RepoRoot

function Write-Step([string]$message) {
    Write-Host "`n==> $message" -ForegroundColor Cyan
}

function Ensure-Tool([string]$name, [string]$installHint) {
    if (-not (Get-Command $name -ErrorAction SilentlyContinue)) {
        throw "缺少工具: $name。$installHint"
    }
}

function Ensure-Directory([string]$path) {
    if (-not (Test-Path -LiteralPath $path)) {
        New-Item -ItemType Directory -Path $path | Out-Null
    }
}

function Test-FileLocked([string]$path) {
    try {
        $stream = [System.IO.File]::Open($path, [System.IO.FileMode]::Open, [System.IO.FileAccess]::ReadWrite, [System.IO.FileShare]::None)
        $stream.Close()
        return $false
    } catch {
        return $true
    }
}

function Try-ReplaceFile([string]$source, [string]$dest) {
    $temp = "$dest.tmp"
    try {
        if (Test-Path -LiteralPath $dest) {
            if (Test-FileLocked $dest) {
                return $false
            }
        }
        Copy-Item -LiteralPath $source -Destination $temp -Force
        Move-Item -LiteralPath $temp -Destination $dest -Force
        return $true
    } catch {
        return $false
    } finally {
        if (Test-Path -LiteralPath $temp) {
            Remove-Item -LiteralPath $temp -Force
        }
    }
}

function Build-WebBackend {
    Write-Step '编译网页后端 (Go)'
    Ensure-Tool 'go' '请先安装 Go，并确保 go 在 PATH 中。'

    if (Get-Command sqlc -ErrorAction SilentlyContinue) {
        Write-Host '检测到 sqlc，先生成数据库代码...' -ForegroundColor DarkGray
        sqlc generate
    }

    Ensure-Directory 'dist'

    $output = Join-Path $RepoRoot 'dist/server.exe'
    go build -trimpath -ldflags "-s -w" -o $output ./cmd/server

    Write-Host "输出: $output" -ForegroundColor Green
}

function Build-WebFrontend {
    Write-Step '编译网页前端 (静态资源)'

    $required = @(
        'internal/web/static/index.html',
        'internal/web/static/assets/app.js',
        'internal/web/static/assets/app.css'
    )

    foreach ($file in $required) {
        if (-not (Test-Path -LiteralPath (Join-Path $RepoRoot $file))) {
            throw "缺少前端资源文件: $file"
        }
    }

    Ensure-Directory 'dist'
    Ensure-Directory 'dist/web'

    Copy-Item -LiteralPath (Join-Path $RepoRoot 'internal/web/static') -Destination (Join-Path $RepoRoot 'dist/web/static') -Recurse -Force

    Write-Host '前端为静态资源，无需额外编译；已复制到 dist/web/static 供检查。' -ForegroundColor Green
}

function Build-Web {
    Build-WebFrontend
    Build-WebBackend
}

function Build-Client {
    Write-Step '编译客户端 (.NET 8 单文件 + 自包含)'
    Ensure-Tool 'dotnet' '请先安装 .NET 8 SDK，并确保 dotnet 在 PATH 中。'

    Ensure-Directory 'dist'
    Ensure-Directory 'dist/client'

    $outDir = Join-Path $RepoRoot 'dist/client/publish'
    if (Test-Path -LiteralPath $outDir) {
        Remove-Item -LiteralPath $outDir -Recurse -Force
    }
    Ensure-Directory $outDir

    dotnet publish "client/WorkSentry.Client/WorkSentry.Client.csproj" -c Release -r win-x64 -o $outDir `
        /p:PublishSingleFile=true `
        /p:SelfContained=true `
        /p:IncludeNativeLibrariesForSelfExtract=true `
        /p:IncludeAllContentForSelfExtract=true `
        /p:DebugType=none `
        /p:DebugSymbols=false

    $publishExe = Join-Path $outDir 'WorkSentry.Client.exe'
    if (-not (Test-Path -LiteralPath $publishExe)) {
        throw "未找到发布产物: $publishExe"
    }

    $info = Get-Item -LiteralPath $publishExe
    if ($info.Length -lt 5MB) {
        throw "客户端输出体积异常（$($info.Length) bytes），这不是单文件自包含 EXE。请确认 dotnet publish 没被环境变量或脚本覆盖。"
    }

    $outExe = Join-Path $RepoRoot 'dist/client/WorkSentry.Client.exe'
    if (-not (Try-ReplaceFile $publishExe $outExe)) {
        $timestamp = Get-Date -Format 'yyyyMMdd-HHmmss'
        $fallback = Join-Path $RepoRoot "dist/client/WorkSentry.Client.$timestamp.exe"
        Copy-Item -LiteralPath $publishExe -Destination $fallback -Force
        Write-Host "目标文件正在被占用，已输出到: $fallback" -ForegroundColor Yellow
        Write-Host "请退出正在运行的客户端后重新编译，以覆盖默认文件。" -ForegroundColor Yellow
    } else {
        Write-Host "输出: $outExe" -ForegroundColor Green
    }
    Write-Host "大小: $([Math]::Round($info.Length / 1MB, 1)) MB" -ForegroundColor DarkGray
}

function Prompt-Target {
    Write-Host ''
    Write-Host '请选择要编译的目标:' -ForegroundColor Yellow
    Write-Host '  1) 编译网页后端'
    Write-Host '  2) 编译网页前端'
    Write-Host '  3) 编译网页前后端'
    Write-Host '  4) 编译客户端'
    Write-Host '  5) 全部'
    Write-Host '  0) 退出'

    $choice = Read-Host '输入序号'
    switch ($choice) {
        '1' { return 'web-backend' }
        '2' { return 'web-frontend' }
        '3' { return 'web' }
        '4' { return 'client' }
        '5' { return 'all' }
        '0' { return '' }
        default { throw '无效选择' }
    }
}

if ([string]::IsNullOrWhiteSpace($Target)) {
    $Target = Prompt-Target
    if ([string]::IsNullOrWhiteSpace($Target)) {
        Write-Host '已退出。'
        exit 0
    }
}

switch ($Target) {
    'web-backend' { Build-WebBackend }
    'web-frontend' { Build-WebFrontend }
    'web' { Build-Web }
    'client' { Build-Client }
    'all' {
        Build-Web
        Build-Client
    }
}

Write-Host "`n完成。" -ForegroundColor Green
