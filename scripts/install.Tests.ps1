$ErrorActionPreference = "Stop"

$InstallerPath = Join-Path $PSScriptRoot "install.ps1"
$Installer = Get-Content -LiteralPath $InstallerPath -Raw
$ArchitectureExpression = '[System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture'
$ReleaseGuard = 'if ($Version -eq "latest") {'
$SuccessMarker = "__ARCHITECTURE_FALLBACK_OK__"
$ExpectedInstallDir = Join-Path $HOME ".local\bin"

if (-not $Installer.Contains($ArchitectureExpression)) {
    throw "Architecture detection expression was not found in install.ps1"
}
if (-not $Installer.Contains($ReleaseGuard)) {
    throw "Release guard was not found in install.ps1"
}

# Simulate an older PowerShell/.NET environment where RuntimeInformation cannot
# report the architecture. Stop before the first network request.
$Installer = $Installer.Replace($ArchitectureExpression, '$null')
$Installer = $Installer.Replace($ReleaseGuard, "throw `"$SuccessMarker|`$InstallDir`"`n$ReleaseGuard")

$PreviousInstallDir = $env:AGENTCTL_INSTALL_DIR
try {
    Remove-Item Env:AGENTCTL_INSTALL_DIR -ErrorAction SilentlyContinue
    try {
        Invoke-Expression $Installer
        throw "Installer did not reach the architecture fallback checkpoint"
    } catch {
        if ($_.Exception.Message -ne "$SuccessMarker|$ExpectedInstallDir") {
            throw "Architecture fallback or default install directory failed: $($_.Exception.Message)"
        }
    }
} finally {
    if ($null -eq $PreviousInstallDir) {
        Remove-Item Env:AGENTCTL_INSTALL_DIR -ErrorAction SilentlyContinue
    } else {
        $env:AGENTCTL_INSTALL_DIR = $PreviousInstallDir
    }
}

Write-Output "PASS: installer uses .local/bin and falls back when RuntimeInformation returns null"
