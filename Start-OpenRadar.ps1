$ErrorActionPreference = "Stop"

Set-Location -LiteralPath $PSScriptRoot

$port = 5001
$baseUrl = "http://localhost:$port"
$statusUrl = "$baseUrl/api/gather/status"
$binaryPath = Join-Path $PSScriptRoot "work\OpenRadar-dev.exe"
$npmLockPath = Join-Path $PSScriptRoot "package-lock.json"
$nodeModulesLockPath = Join-Path $PSScriptRoot "node_modules\.package-lock.json"

function Write-Section {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Message
    )

    Write-Host ""
    Write-Host "== $Message ==" -ForegroundColor Cyan
}

function Wait-ForRadar {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Url,
        [int]$TimeoutSeconds = 45
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        try {
            Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 2 | Out-Null
            return $true
        } catch {
            Start-Sleep -Milliseconds 750
        }
    }

    return $false
}

function Get-ValidAdapterIP {
    $ipPath = Join-Path $PSScriptRoot "ip.txt"
    if (-not (Test-Path -LiteralPath $ipPath)) {
        return $null
    }

    $savedIP = Get-Content -LiteralPath $ipPath -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $savedIP) {
        return $null
    }

    $savedIP = $savedIP.Trim()
    $parsed = $null
    if ([System.Net.IPAddress]::TryParse($savedIP, [ref]$parsed)) {
        return $savedIP
    }

    Write-Host "Saved adapter IP in ip.txt is invalid. OpenRadar will prompt you to select an adapter." -ForegroundColor Yellow
    return $null
}

function Get-LatestWriteTimeUtc {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$Paths,
        [string[]]$Include = @("*")
    )

    $latest = [datetime]::MinValue

    foreach ($path in $Paths) {
        if (-not (Test-Path -LiteralPath $path)) {
            continue
        }

        $item = Get-Item -LiteralPath $path
        if ($item.PSIsContainer) {
            $files = Get-ChildItem -LiteralPath $path -Recurse -File -Include $Include -ErrorAction SilentlyContinue
            foreach ($file in $files) {
                if ($file.LastWriteTimeUtc -gt $latest) {
                    $latest = $file.LastWriteTimeUtc
                }
            }
            continue
        }

        if ($item.LastWriteTimeUtc -gt $latest) {
            $latest = $item.LastWriteTimeUtc
        }
    }

    return $latest
}

function Test-NeedsNpmInstall {
    $nodeModulesPath = Join-Path $PSScriptRoot "node_modules"
    if (-not (Test-Path -LiteralPath $nodeModulesPath)) {
        return $true
    }

    if (-not (Test-Path -LiteralPath $nodeModulesLockPath)) {
        return $true
    }

    $projectLock = Get-Item -LiteralPath $npmLockPath -ErrorAction SilentlyContinue
    $installedLock = Get-Item -LiteralPath $nodeModulesLockPath -ErrorAction SilentlyContinue
    if (-not $projectLock -or -not $installedLock) {
        return $true
    }

    return $projectLock.LastWriteTimeUtc -gt $installedLock.LastWriteTimeUtc
}

function Test-NeedsFrontendBuild {
    $tailwindPath = Join-Path $PSScriptRoot "web\styles\tailwind.css"
    $htmxPath = Join-Path $PSScriptRoot "web\scripts\vendors\htmx.min.js"
    $lucidePath = Join-Path $PSScriptRoot "web\scripts\vendors\lucide.min.js"
    $fontPath = Join-Path $PSScriptRoot "web\styles\fonts\space-grotesk-latin-700-normal.woff2"

    $outputs = @($tailwindPath, $htmxPath, $lucidePath, $fontPath)
    foreach ($output in $outputs) {
        if (-not (Test-Path -LiteralPath $output)) {
            return $true
        }
    }

    $latestInput = Get-LatestWriteTimeUtc -Paths @(
        (Join-Path $PSScriptRoot "package.json"),
        $npmLockPath,
        (Join-Path $PSScriptRoot "web\styles"),
        (Join-Path $PSScriptRoot "web\scripts"),
        (Join-Path $PSScriptRoot "internal\templates")
    ) -Include @("*.css", "*.js", "*.gohtml", "package.json", "package-lock.json")

    $earliestOutput = ($outputs | ForEach-Object { (Get-Item -LiteralPath $_).LastWriteTimeUtc } | Measure-Object -Minimum).Minimum
    return $latestInput -gt $earliestOutput
}

function Test-NeedsGoBuild {
    if (-not (Test-Path -LiteralPath $binaryPath)) {
        return $true
    }

    $binaryWriteTime = (Get-Item -LiteralPath $binaryPath).LastWriteTimeUtc
    $latestSource = Get-LatestWriteTimeUtc -Paths @(
        (Join-Path $PSScriptRoot "go.mod"),
        (Join-Path $PSScriptRoot "go.sum"),
        (Join-Path $PSScriptRoot "embed_dev.go"),
        (Join-Path $PSScriptRoot "embed_prod.go"),
        (Join-Path $PSScriptRoot "cmd"),
        (Join-Path $PSScriptRoot "internal")
    ) -Include @("*.go", "go.mod", "go.sum")

    return $latestSource -gt $binaryWriteTime
}

function Resolve-ListeningProcess {
    param(
        [int]$Port
    )

    $listener = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue |
        Select-Object -First 1

    if (-not $listener) {
        return $null
    }

    try {
        return Get-Process -Id $listener.OwningProcess -ErrorAction Stop
    } catch {
        return $null
    }
}

function Is-LauncherManagedProcess {
    param(
        [Parameter(Mandatory = $true)]
        [System.Diagnostics.Process]$Process
    )

    if ($Process.ProcessName -ieq "radar" -or $Process.ProcessName -ieq "OpenRadar-dev") {
        return $true
    }

    try {
        $processPath = $Process.Path
        if ($processPath) {
            return [string]::Equals(
                [System.IO.Path]::GetFullPath($processPath),
                [System.IO.Path]::GetFullPath($binaryPath),
                [System.StringComparison]::OrdinalIgnoreCase
            )
        }
    } catch {
        return $false
    }

    return $false
}

Write-Host "====================================="
Write-Host "         OpenRadar Launcher"
Write-Host "====================================="
$savedAdapterIP = Get-ValidAdapterIP

Write-Section "Checking existing server"
$existingProcess = Resolve-ListeningProcess -Port $port
if ($existingProcess) {
    if (Is-LauncherManagedProcess -Process $existingProcess) {
        Write-Host "Stopping existing OpenRadar process on port $port (PID $($existingProcess.Id))..."
        Stop-Process -Id $existingProcess.Id -Force
        Start-Sleep -Seconds 1
    } else {
        Write-Host "Port $port is already in use by '$($existingProcess.ProcessName)' (PID $($existingProcess.Id))." -ForegroundColor Red
        Write-Host "Close that process or change the app port before launching OpenRadar." -ForegroundColor Yellow
        exit 1
    }
} else {
    Write-Host "Port $port is free."
}

Write-Section "Checking required tools"
$goCommand = Get-Command go -ErrorAction SilentlyContinue
if (-not $goCommand) {
    Write-Host "Go is not installed or not on PATH." -ForegroundColor Red
    exit 1
}

$npmCommand = Get-Command npm.cmd -ErrorAction SilentlyContinue
if (-not $npmCommand) {
    Write-Host "npm.cmd is not installed or not on PATH." -ForegroundColor Red
    exit 1
}

Write-Host "Go:  $($goCommand.Source)"
Write-Host "npm: $($npmCommand.Source)"

$npcapService = Get-Service -Name "npcap","npf" -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $npcapService) {
    Write-Host "Npcap was not detected. Install it from https://npcap.com/#download before starting OpenRadar." -ForegroundColor Red
    exit 1
}

Write-Section "Preparing dependencies"
if (Test-NeedsNpmInstall) {
    Write-Host "Installing npm packages..."
    & npm.cmd ci
    if ($LASTEXITCODE -ne 0) {
        Write-Host "npm install failed." -ForegroundColor Red
        exit $LASTEXITCODE
    }
} else {
    Write-Host "npm packages are up to date."
}

if (Test-NeedsFrontendBuild) {
    Write-Host "Building frontend assets..."
    & npm.cmd run build
    if ($LASTEXITCODE -ne 0) {
        Write-Host "Frontend asset build failed." -ForegroundColor Red
        exit $LASTEXITCODE
    }
} else {
    Write-Host "Frontend assets are up to date."
}

Write-Section "Preparing OpenRadar binary"
if (-not (Test-Path -LiteralPath (Split-Path -Parent $binaryPath))) {
    New-Item -ItemType Directory -Path (Split-Path -Parent $binaryPath) | Out-Null
}

if (Test-NeedsGoBuild) {
    Write-Host "Building OpenRadar launcher binary..."
    & go build -o $binaryPath ./cmd/radar
    if ($LASTEXITCODE -ne 0) {
        Write-Host "Go build failed." -ForegroundColor Red
        exit $LASTEXITCODE
    }
} else {
    Write-Host "OpenRadar binary is up to date."
}

Write-Section "Starting browser watcher"
$browserJob = Start-Job -ScriptBlock {
    param($StatusUrl, $BaseUrl)

    $deadline = (Get-Date).AddSeconds(45)
    while ((Get-Date) -lt $deadline) {
        try {
            Invoke-WebRequest -Uri $StatusUrl -UseBasicParsing -TimeoutSec 2 | Out-Null
            Start-Process $BaseUrl | Out-Null
            return
        } catch {
            Start-Sleep -Milliseconds 750
        }
    }
} -ArgumentList $statusUrl, $baseUrl

Write-Section "Launching OpenRadar"
Write-Host "The browser will open automatically when the server responds."
Write-Host "Keep this window open while using the radar."
if ($savedAdapterIP) {
    Write-Host "Using saved adapter IP: $savedAdapterIP"
} else {
    Write-Host "No valid saved adapter IP found. If prompted, choose your internet adapter once and it will be remembered."
}
Write-Host ""

$appArgs = @("-dev")
if ($savedAdapterIP) {
    $appArgs += @("-ip", $savedAdapterIP)
}

try {
    & $binaryPath @appArgs
    $exitCode = $LASTEXITCODE
} finally {
    if ($browserJob) {
        if ($browserJob.State -eq "Completed") {
            Receive-Job -Job $browserJob -AutoRemoveJob | Out-Null
        } else {
            Stop-Job -Job $browserJob -ErrorAction SilentlyContinue | Out-Null
            Remove-Job -Job $browserJob -Force -ErrorAction SilentlyContinue | Out-Null
        }
    }
}

if ($exitCode -eq 0) {
    Write-Host ""
    Write-Host "OpenRadar exited normally."
    exit 0
}

Write-Host ""
Write-Host "OpenRadar exited with code $exitCode." -ForegroundColor Red
exit $exitCode
