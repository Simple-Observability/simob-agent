param (
  [Parameter(Mandatory=$true)]
  [string]$ApiKey,

  [switch]$SkipKeyCheck,

  [switch]$SkipTelemetry,

  [switch]$SkipServiceStart,

  [Parameter(ValueFromRemainingArguments=$true)]
  $ExtraArgs
)

# -----------------------------------------------------------------------------
# Settings and constants
# -----------------------------------------------------------------------------

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$ServiceName = "simob"
$DisplayName = "Simple Observability agent"
$InstallDir  = "$env:ProgramFiles\SimpleObservability\simob"
$ExeName     = "simob.exe"
$ExePath     = Join-Path -Path $InstallDir -ChildPath $ExeName
$BaseUrl     = "https://api.simpleobservability.com"

# -----------------------------------------------------------------------------
# Helper functions
# -----------------------------------------------------------------------------

function Write-Log {
  param([string]$Message, [string]$Level="INFO")
  $Color = "White"
  if ($Level -eq "ERROR") { $Color = "Red" }
  Write-Host "[$Level] $Message" -ForegroundColor $Color
}

function Exit-WithTelemetry {
  param([string]$Reason)
  Write-Log "Installation failed: $Reason" "ERROR"
  if ($SkipTelemetry) {
    exit 1
  }
  $TelemetryEndpoint = "$BaseUrl/telemetry/install"
  $Payload = @{ reason = $Reason } | ConvertTo-Json
  try {
    Invoke-RestMethod -Uri $TelemetryEndpoint -Method Post -Body $Payload -ContentType "application/json" -TimeoutSec 5 -ErrorAction SilentlyContinue
  } catch {
    # Ignore telemetry failures
  }
  exit 1
}

function Test-IsAdmin {
  $Identity = [Security.Principal.WindowsIdentity]::GetCurrent()
  $Principal = [Security.Principal.WindowsPrincipal]$Identity
  return $Principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Get-Architecture {
  if ($env:PROCESSOR_ARCHITECTURE -eq "AMD64") {
    return "amd64"
  } elseif ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") {
    return "arm64"
  } else {
    return $null
  }
}

# -----------------------------------------------------------------------------
# Main
# -----------------------------------------------------------------------------

#  Admin Check
if (-not (Test-IsAdmin)) {
  Exit-WithTelemetry "Script run without Administrator privileges"
}

# Dependency Check (PowerShell Version)
if ($PSVersionTable.PSVersion.Major -lt 5) {
  Exit-WithTelemetry "PowerShell version < 5.0. Please upgrade PowerShell."
}

# Validate API Key
if (-not $SkipKeyCheck) {
  Write-Log "Validating API key on remote server..."
  try {
    $Uri = "$BaseUrl/check-key/"
    $Body = @{ api_key = $ApiKey } | ConvertTo-Json
    Invoke-RestMethod -Uri $Uri -Method Post -Body $Body -ContentType "application/json" -ErrorAction Stop | Out-Null
    Write-Log "API key is valid."
  } catch {
    $StatusCode = $_.Exception.Response.StatusCode.value__
    Write-Log "API Key validation failed. HTTP $StatusCode" "ERROR"
    Exit-WithTelemetry "API key is not valid (HTTP $StatusCode)"
  }
}

# Fetch binary
# If BINARY_PATH env var is set, use that, otherwise download
$Arch = Get-Architecture
if (-not $Arch) {
  Exit-WithTelemetry "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE"
}
if ($env:BINARY_PATH) {
  Write-Log "Using existing binary at: $env:BINARY_PATH"
  if (-not (Test-Path $env:BINARY_PATH)) {
    Exit-WithTelemetry "BINARY_PATH defined but file not found"
  }
  $TempFile = "$env:TEMP\simob-install.exe"
  Copy-Item -Path $env:BINARY_PATH -Destination $TempFile -Force
} else {
  $DownloadUrl = "https://github.com/Simple-Observability/simob-agent/releases/latest/download/simob-windows-$Arch.exe"
  Write-Log "Downloading binary from $DownloadUrl..."
  $TempFile = "$env:TEMP\simob-install.exe"
  try {
    Invoke-WebRequest -Uri $DownloadUrl -OutFile $TempFile -UseBasicParsing
    Write-Log "Download complete."
  } catch {
    Exit-WithTelemetry "Failed to download binary: $_"
  }
}

# Install binary
Write-Log "Installing binary to $InstallDir..."
if (-not (Test-Path $InstallDir)) {
  New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}
try {
  Move-Item -Path $TempFile -Destination $ExePath -Force
} catch {
  Exit-WithTelemetry "Failed to move binary to install directory.?"
}

# Initialize agent
Write-Log "Initializing agent..."
try {
  $InitProcess = Start-Process -FilePath $ExePath -ArgumentList "init `"$ApiKey`" $ExtraArgs" -Wait -NoNewWindow -PassThru
  if ($InitProcess.ExitCode -ne 0) {
    throw "Agent init command returned exit code $($InitProcess.ExitCode)"
  }
} catch {
  Exit-WithTelemetry "Agent initialization failed: $_"
}

# Setup Windows service
Write-Log "Configuring Windows Service..."
if (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue) {
  Write-Log -Message "Service already exists."
} else {
  # Create new service
  New-Service -Name $ServiceName -DisplayName $DisplayName -BinaryPathName "`"$ExePath`" start" -StartupType Automatic
  Write-Log -Message "Created new service."
}

# Set recovery options: Restart service on failure (1st, 2nd, and subsequent failures)
sc.exe failure $ServiceName reset= 86400 actions= restart/60000/restart/120000/takeNoAction/0

# Start Service
Write-Log "Starting service..."
if (-not $SkipServiceStart) {
  try {
    Start-Service -Name $ServiceName
    Write-Log "Service started successfully."
  } catch {
    Exit-WithTelemetry "Failed to start service: $_"
  }
}

Write-Log ""
Write-Log "----------------------------------------------------------------"
Write-Log "Simple Observability (simob) agent installed successfully!"
Write-Log "Location: $ExePath"
Write-Log "Service:  $ServiceName (Running as LocalSystem)"
Write-Log "----------------------------------------------------------------"