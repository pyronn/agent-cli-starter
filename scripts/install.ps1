[CmdletBinding()]
param(
    [string]$Version = $(if ($env:AGENTCTL_VERSION) { $env:AGENTCTL_VERSION } else { "latest" }),
    [string]$InstallDir = $(if ($env:AGENTCTL_INSTALL_DIR) { $env:AGENTCTL_INSTALL_DIR } else { Join-Path $HOME "bin" }),
    [switch]$NoModifyPath
)

$ErrorActionPreference = "Stop"
$CliName = "agentctl"
$RepositoryUrl = "https://github.com/pyronn/agent-cli-starter"
$Repository = $RepositoryUrl -replace '^https://github\.com/', ''

$Architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
switch ($Architecture) {
    "X64" { $Arch = "amd64" }
    "Arm64" { $Arch = "arm64" }
    default { throw "Unsupported CPU architecture: $Architecture" }
}

if ($Version -eq "latest") {
    $Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repository/releases/latest"
    $Version = $Release.tag_name
}
if ($Version -notmatch '^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$') {
    throw "Invalid version '$Version'; expected a tag such as v1.2.3"
}

$Archive = "$CliName-$Version-windows-$Arch.zip"
$Binary = "$CliName-windows-$Arch.exe"
$BaseUrl = "https://github.com/$Repository/releases/download/$Version"
$TempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("$CliName-install-" + [guid]::NewGuid())

try {
    New-Item -ItemType Directory -Force $TempDir | Out-Null
    Write-Host "Downloading $CliName $Version for windows/$Arch..."
    Invoke-WebRequest -Uri "$BaseUrl/$Archive" -OutFile (Join-Path $TempDir $Archive)
    Invoke-WebRequest -Uri "$BaseUrl/SHA256SUMS" -OutFile (Join-Path $TempDir "SHA256SUMS")

    $ChecksumLine = Get-Content (Join-Path $TempDir "SHA256SUMS") |
        Where-Object { $_ -match "^[0-9a-fA-F]{64}\s+\*?$([regex]::Escape($Archive))$" } |
        Select-Object -First 1
    if (-not $ChecksumLine) {
        throw "Checksum for $Archive is missing"
    }
    $Expected = ($ChecksumLine -split '\s+')[0].ToUpperInvariant()
    $Actual = (Get-FileHash (Join-Path $TempDir $Archive) -Algorithm SHA256).Hash.ToUpperInvariant()
    if ($Actual -ne $Expected) {
        throw "SHA256 checksum mismatch for $Archive"
    }

    Expand-Archive -LiteralPath (Join-Path $TempDir $Archive) -DestinationPath $TempDir -Force
    $Source = Join-Path $TempDir $Binary
    if (-not (Test-Path -LiteralPath $Source -PathType Leaf)) {
        throw "Archive does not contain $Binary"
    }
    New-Item -ItemType Directory -Force $InstallDir | Out-Null
    $Destination = Join-Path $InstallDir "$CliName.exe"
    Copy-Item -LiteralPath $Source -Destination $Destination -Force

    if (-not $NoModifyPath) {
        $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
        $PathEntries = @($UserPath -split ';' | Where-Object { $_ })
        if ($PathEntries -notcontains $InstallDir) {
            $NewPath = (@($PathEntries) + $InstallDir) -join ';'
            [Environment]::SetEnvironmentVariable("Path", $NewPath, "User")
            $env:Path = "$InstallDir;$env:Path"
            Write-Host "Added $InstallDir to your user PATH."
        }
    }

    Write-Host "Installed $CliName $Version to $Destination"
    & $Destination version
} finally {
    if (Test-Path -LiteralPath $TempDir) {
        Remove-Item -LiteralPath $TempDir -Recurse -Force
    }
}
