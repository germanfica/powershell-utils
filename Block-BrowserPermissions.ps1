#Requires -Version 5.1
[CmdletBinding()]
param([switch]$Restore)

$ErrorActionPreference = 'Stop'
$BackupFile = Join-Path $PSScriptRoot 'Block-BrowserPermissions.backup.json'

# 2 = Block for Chromium content-setting policies below.
# These are per-browser defaults, not a guarantee that every Chromium permission is covered.
$BlockedPolicies = [ordered]@{
    DefaultCookiesSetting              = 2
    DefaultImagesSetting               = 2
    DefaultJavaScriptSetting           = 2
    DefaultPopupsSetting               = 2
    DefaultNotificationsSetting        = 2
    DefaultGeolocationSetting          = 2
    DefaultMediaStreamCameraSetting    = 2
    DefaultMediaStreamMicSetting       = 2
    DefaultSensorsSetting              = 2
    DefaultClipboardSetting            = 2
    DefaultAutomaticDownloadsSetting   = 2
    DefaultFileSystemReadGuardSetting  = 2
    DefaultFileSystemWriteGuardSetting = 2
    DefaultWebBluetoothGuardSetting    = 2
    DefaultWebUsbGuardSetting          = 2
    DefaultWebHidGuardSetting          = 2
    DefaultSerialGuardSetting          = 2
    DefaultIdleDetectionSetting        = 2
    DefaultLocalFontsSetting           = 2
}

$ExtraPolicies = [ordered]@{
    BlockThirdPartyCookies = 1
}

$BrowserPaths = @(
    'HKCU:\SOFTWARE\Policies\BraveSoftware\Brave',
    'HKCU:\SOFTWARE\Policies\Microsoft\Edge'
)

function Test-RegistryValue {
    param([string]$Path, [string]$Name)
    if (-not (Test-Path -LiteralPath $Path)) { return $false }
    $Key = Get-Item -LiteralPath $Path
    return ($Key.GetValueNames() -contains $Name)
}

function Get-AllPolicyNames {
    return @($BlockedPolicies.Keys) + @($ExtraPolicies.Keys)
}

if ($Restore) {
    if (-not (Test-Path -LiteralPath $BackupFile)) {
        throw "Backup not found: $BackupFile. No changes were made."
    }
    $Backup = @(Get-Content -LiteralPath $BackupFile -Raw | ConvertFrom-Json)
    foreach ($Entry in $Backup) {
        if ($Entry.Existed) {
            if (-not (Test-Path -LiteralPath $Entry.Path)) {
                New-Item -Path $Entry.Path -Force | Out-Null
            }
            New-ItemProperty -LiteralPath $Entry.Path -Name $Entry.Name -PropertyType DWord -Value ([int]$Entry.Value) -Force | Out-Null
        }
        elseif (Test-RegistryValue -Path $Entry.Path -Name $Entry.Name) {
            Remove-ItemProperty -LiteralPath $Entry.Path -Name $Entry.Name
        }
    }
    Write-Host 'Previous policy values restored. Restart Brave and Edge.' -ForegroundColor Green
    Write-Host "Backup kept at: $BackupFile"
    exit 0
}

# Keep the original snapshot on subsequent runs to allow a true restore.
if (-not (Test-Path -LiteralPath $BackupFile)) {
    $Snapshot = @(
        foreach ($Path in $BrowserPaths) {
            foreach ($Name in (Get-AllPolicyNames)) {
                $Exists = Test-RegistryValue -Path $Path -Name $Name
                $Value = $null
                if ($Exists) {
                    $Value = (Get-ItemProperty -LiteralPath $Path -Name $Name).$Name
                }
                [pscustomobject]@{
                    Path = $Path
                    Name = $Name
                    Existed = [bool]$Exists
                    Value = $Value
                }
            }
        }
    )
    $Snapshot | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $BackupFile -Encoding UTF8
    Write-Host "Backup created: $BackupFile"
}
else {
    Write-Host "Using existing backup: $BackupFile"
}

foreach ($Path in $BrowserPaths) {
    if (-not (Test-Path -LiteralPath $Path)) {
        New-Item -Path $Path -Force | Out-Null
    }
    foreach ($Name in $BlockedPolicies.Keys) {
        New-ItemProperty -LiteralPath $Path -Name $Name -PropertyType DWord -Value $BlockedPolicies[$Name] -Force | Out-Null
    }
    foreach ($Name in $ExtraPolicies.Keys) {
        New-ItemProperty -LiteralPath $Path -Name $Name -PropertyType DWord -Value $ExtraPolicies[$Name] -Force | Out-Null
    }
    Write-Host "Applied $($BlockedPolicies.Count + $ExtraPolicies.Count) policies: $Path" -ForegroundColor Green
}

Write-Host ''
Write-Host 'Restart both browsers and check brave://policy and edge://policy.'
Write-Warning 'JavaScript, images and all cookies are blocked. Many websites will not work.'
Write-Warning 'Not every setting in All permissions has a supported policy. Existing site exceptions or higher-priority policies may affect results.'
Write-Host 'To revert: .\Block-BrowserPermissions.ps1 -Restore'
