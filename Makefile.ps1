# OpenOcta PowerShell 构建脚本
# 功能与 Makefile 对齐，供 Windows PowerShell / CMD 用户使用
# 用法: .\Makefile.ps1 [ui|embed|go|build|clean|wails|wails-nsis|wails-dev]
# 默认: build（完整构建）

param(
    [Parameter(Position = 0)]
    [string]$Target = "build"
)

$ErrorActionPreference = "Stop"
$RepoRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $RepoRoot

function Write-Step($msg) {
    Write-Host "==> $msg" -ForegroundColor Cyan
}

function Invoke-Ui {
    Write-Step "构建前端..."
    Set-Location "$RepoRoot\ui"
    & npm install
    & npm run build
    Set-Location $RepoRoot
}

function Invoke-Embed {
    Write-Step "从 git tag 设置版本..."
    # 使用 bash 执行 set-version.sh（Git Bash 通常已安装）
    $bash = Get-Command bash -ErrorAction SilentlyContinue
    if ($bash) {
        & bash "$RepoRoot\scripts\set-version.sh"
    }
    else {
        # 回退：手动设置版本
        $version = (git describe --tags --always 2>$null) -replace '^v', ''
        if (-not $version) { $version = "0.0.0-dev" }
        Write-Step "版本: $version"
        $envFile = "$RepoRoot\src\.env"
        if (Test-Path $envFile) {
            $lines = Get-Content $envFile
            $found = $false
            $lines = $lines | ForEach-Object {
                if ($_ -match '^OPENOCTA_BUNDLED_VERSION=') {
                    $found = $true
                    "OPENOCTA_BUNDLED_VERSION=$version"
                }
                else { $_ }
            }
            if (-not $found) { $lines += "OPENOCTA_BUNDLED_VERSION=$version" }
            $lines | Set-Content $envFile
        }
    }

    Invoke-Ui

    Write-Step "复制 embed 资源..."
    $embedDir = "$RepoRoot\src\embed"
    if (-not (Test-Path $embedDir)) { New-Item -ItemType Directory -Path $embedDir -Force | Out-Null }

    @("src\config-schema.json", "src\openocta.json.example", "src\.env") | ForEach-Object {
        $src = "$RepoRoot\$_"
        if (Test-Path $src) {
            Copy-Item $src $embedDir -Force
        }
    }
}

function Invoke-Go {
    Invoke-Embed
    Write-Step "构建 Go 二进制..."
    Set-Location "$RepoRoot\src"
    & go build -ldflags "-s -w" -o ..\openocta.exe .\cmd\openocta
    Set-Location $RepoRoot
    Write-Step "完成: .\openocta.exe"
}

function Invoke-Build {
    Invoke-Go
}

function Invoke-Clean {
    Write-Step "清理..."
    $paths = @(
        "dist",
        "build",
        "src\embed\frontend",
        "src\embed\config-schema.json",
        "src\embed\openocta.json.example",
        "src\build\bin",
        "openocta",
        "openocta.exe",
        "openocta-launcher",
        "openocta-launcher.exe"
    )
    foreach ($p in $paths) {
        $full = Join-Path $RepoRoot $p
        if (Test-Path $full) {
            Remove-Item $full -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
    Write-Step "清理完成"
}

function Invoke-PrepareWailsIcons {
    $srcPng = "$RepoRoot\imgs\openocta_logo_wails.png"
    if (-not (Test-Path $srcPng)) {
        $srcPng2 = "$RepoRoot\imgs\openocta_logo.png"
        if (-not (Test-Path $srcPng2)) {
            throw "ERROR: 缺少 imgs/openocta_logo_wails.png 或 imgs/openocta_logo.png"
        }
        # 简单复制（假设已缩放）
        Copy-Item $srcPng2 $srcPng -Force
    }

    $buildDir = "$RepoRoot\src\build"
    $winDir = "$buildDir\windows"
    if (-not (Test-Path $winDir)) { New-Item -ItemType Directory -Path $winDir -Force | Out-Null }

    Copy-Item $srcPng "$buildDir\appicon.png" -Force
    & node "$RepoRoot\scripts\png-to-ico.mjs" $srcPng "$buildDir\appicon.ico"
    Copy-Item "$buildDir\appicon.ico" "$winDir\icon.ico" -Force
}

function Invoke-Wails {
    Invoke-Embed
    Invoke-PrepareWailsIcons
    Write-Step "Wails 桌面应用构建（当前平台）..."
    Set-Location "$RepoRoot\src"
    & wails build -skipbindings
    Set-Location $RepoRoot
}

function Invoke-WailsNsis {
    Write-Step "Wails Windows 安装器构建..."

    # 查找 NSIS
    $nsisPaths = @(
        "${env:ProgramFiles(x86)}\NSIS",
        "${env:ProgramFiles}\NSIS",
        "${env:ProgramFiles(x86)}\NSIS\Bin",
        "C:\NSIS",
        "${env:ProgramData}\chocolatey\lib\nsis\tools\NSIS",
        "${env:ProgramData}\chocolatey\lib\nsis\tools\nsis"
    )
    $nsisRoot = $null
    foreach ($p in $nsisPaths) {
        if (Test-Path "$p\makensis.exe") {
            $nsisRoot = $p
            break
        }
    }
    if (-not $nsisRoot) {
        $makensis = Get-Command makensis -ErrorAction SilentlyContinue
        if ($makensis) {
            $nsisRoot = Split-Path -Parent $makensis.Source
        }
    }
    if (-not $nsisRoot) {
        Write-Error "ERROR: 未找到 NSIS 的 makensis.exe。请安装 NSIS: https://nsis.sourceforge.io/Download"
    }
    Write-Step "NSIS 目录: $nsisRoot"

    Invoke-Embed
    Invoke-PrepareWailsIcons

    $env:PATH = "$nsisRoot;$env:PATH"
    Set-Location "$RepoRoot\src"
    & wails build -platform windows/amd64 -nsis -skipbindings
    Set-Location $RepoRoot

    $installers = Get-ChildItem "$RepoRoot\src\build\bin\*-installer.exe" -ErrorAction SilentlyContinue
    if (-not $installers) {
        Write-Error "ERROR: 未生成 *-installer.exe。请查看 Wails 日志中 NSIS 相关报错。"
    }
    if (-not (Test-Path "$RepoRoot\dist")) { New-Item -ItemType Directory -Path "$RepoRoot\dist" -Force | Out-Null }
    Copy-Item "$RepoRoot\src\build\bin\*.exe" "$RepoRoot\dist\" -Force -ErrorAction SilentlyContinue
    Write-Step "安装器: $($installers[0].FullName)"
    Write-Step "已复制 .exe 到 dist/"
}

function Invoke-WailsDev {
    Invoke-Embed
    Invoke-PrepareWailsIcons
    Write-Step "Wails 开发模式（热重载）..."
    Set-Location "$RepoRoot\src"
    & wails dev
    Set-Location $RepoRoot
}

# 主入口
switch ($Target) {
    "ui"       { Invoke-Ui }
    "embed"    { Invoke-Embed }
    "go"       { Invoke-Go }
    "build"    { Invoke-Build }
    "clean"    { Invoke-Clean }
    "wails"    { Invoke-Wails }
    "wails-nsis" { Invoke-WailsNsis }
    "wails-dev"  { Invoke-WailsDev }
    default {
        Write-Host "用法: .\Makefile.ps1 [ui|embed|go|build|clean|wails|wails-nsis|wails-dev]"
        Write-Host "  ui         - 仅构建前端"
        Write-Host "  embed      - 构建前端并复制 embed 资源"
        Write-Host "  go         - 完整构建（前端+embed+Go）"
        Write-Host "  build      - 同 go（默认）"
        Write-Host "  clean      - 清理构建产物"
        Write-Host "  wails      - Wails 桌面应用（Windows .exe）"
        Write-Host "  wails-nsis - Wails Windows 安装器（需 NSIS）"
        Write-Host "  wails-dev  - Wails 开发模式（热重载）"
        exit 1
    }
}
