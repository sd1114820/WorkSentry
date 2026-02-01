[CmdletBinding()]
param(
    [ValidateSet('','web-backend','web-frontend','web','client','all')]
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

function Wait-ReturnToMenu {
    Write-Host ''
    Write-Host '按任意键返回菜单...' -ForegroundColor DarkGray
    try {
        $null = $Host.UI.RawUI.ReadKey('NoEcho,IncludeKeyDown')
    } catch {
        $null = Read-Host '按回车返回菜单'
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

function Get-ClientCsprojPath {
    return (Join-Path $RepoRoot 'client/WorkSentry.Client/WorkSentry.Client.csproj')
}

function Get-ClientVersionFromCsproj([string]$csprojPath) {
    if (-not (Test-Path -LiteralPath $csprojPath)) {
        return '1.0.0'
    }

    $xml = Get-Content -Raw -Encoding UTF8 $csprojPath
    $m = [regex]::Match($xml, '<Version>([^<]+)</Version>')
    if ($m.Success) {
        return $m.Groups[1].Value.Trim()
    }

    return '1.0.0'
}

function Test-SemVer3([string]$value) {
    return ($value -match '^\d+\.\d+\.\d+$')
}

function Increment-Patch([string]$value) {
    if (-not (Test-SemVer3 $value)) {
        return '1.0.1'
    }

    $parts = $value.Split('.')
    $major = [int]$parts[0]
    $minor = [int]$parts[1]
    $patch = [int]$parts[2]
    $patch++
    return "$major.$minor.$patch"
}

function Set-ClientVersionInCsproj([string]$csprojPath, [string]$version) {
    $xml = Get-Content -Raw -Encoding UTF8 $csprojPath

    if ($xml -match '<Version>[^<]+</Version>') {
        $xml = [regex]::Replace($xml, '<Version>[^<]+</Version>', "<Version>$version</Version>")
    } else {
        $insert = "    <Version>$version</Version>" + "`r`n"
        if ($xml -match '(<RootNamespace>[^<]+</RootNamespace>\s*\r?\n)') {
            $xml = [regex]::Replace($xml, '(<RootNamespace>[^<]+</RootNamespace>\s*\r?\n)', "`$1$insert")
        } else {
            $xml = [regex]::Replace($xml, '(<PropertyGroup>\s*\r?\n)', "`$1$insert")
        }
    }

    Set-Content -Encoding UTF8 $csprojPath $xml
}

function Prompt-ClientVersion([string]$currentVersion) {
    Write-Host "当前客户端版本: $currentVersion" -ForegroundColor DarkGray

    while ($true) {
        $input = Read-Host '请输入版本号（留空自动 +1，例如 1.0.0 -> 1.0.1）'
        $input = ($input ?? '').Trim()

        if ([string]::IsNullOrWhiteSpace($input)) {
            $next = Increment-Patch $currentVersion
            Write-Host "自动升级到: $next" -ForegroundColor Green
            return $next
        }

        if (Test-SemVer3 $input) {
            return $input
        }

        Write-Host '版本号格式不正确，请输入 x.y.z（例如 1.0.1）' -ForegroundColor Yellow
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

    $csproj = Get-ClientCsprojPath
    if (-not (Test-Path -LiteralPath $csproj)) {
        throw "未找到客户端项目文件: $csproj"
    }

    $currentVersion = Get-ClientVersionFromCsproj $csproj
    $newVersion = Prompt-ClientVersion $currentVersion
    Set-ClientVersionInCsproj $csproj $newVersion

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
        /p:DebugSymbols=false `
        /p:Version=$newVersion `
        /p:InformationalVersion=$newVersion `
        /p:IncludeSourceRevisionInInformationalVersion=false

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
        Write-Host '请退出正在运行的客户端后重新编译，以覆盖默认文件。' -ForegroundColor Yellow
    } else {
        Write-Host "输出: $outExe" -ForegroundColor Green
    }

    Write-Host "版本: $newVersion" -ForegroundColor DarkGray
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

function Invoke-Target([string]$target) {
    switch ($target) {
        'web-backend' { Build-WebBackend }
        'web-frontend' { Build-WebFrontend }
        'web' { Build-Web }
        'client' { Build-Client }
        'all' {
            Build-Web
            Build-Client
        }
        default { throw "未知目标: $target" }
    }
}

while ($true) {
    if ([string]::IsNullOrWhiteSpace($Target)) {
        try {
            $Target = Prompt-Target
        } catch {
            Write-Host $_.Exception.Message -ForegroundColor Red
            $Target = ''
            Wait-ReturnToMenu
            continue
        }
    }

    if ([string]::IsNullOrWhiteSpace($Target)) {
        Write-Host '已退出。'
        break
    }

    try {
        Invoke-Target $Target
        Write-Host "`n完成。" -ForegroundColor Green
    } catch {
        Write-Host "`n失败：$($_.Exception.Message)" -ForegroundColor Red
    }

    $Target = ''
    Wait-ReturnToMenu
}

