#Requires -Version 5.1
[CmdletBinding()]
param([switch]$Restore)

$ErrorActionPreference = 'Stop'
$BackupFile = Join-Path $PSScriptRoot 'Block-BrowserPermissions.backup.json'

# 2 = Block for Chromium content settings below.
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

# Names used by Chromium in each profile's Preferences file.
$PreferenceNames = [ordered]@{
    DefaultCookiesSetting              = 'cookies'
    DefaultImagesSetting               = 'images'
    DefaultJavaScriptSetting           = 'javascript'
    DefaultPopupsSetting               = 'popups'
    DefaultNotificationsSetting        = 'notifications'
    DefaultGeolocationSetting          = 'geolocation'
    DefaultMediaStreamCameraSetting    = 'media_stream_camera'
    DefaultMediaStreamMicSetting       = 'media_stream_mic'
    DefaultSensorsSetting              = 'sensors'
    DefaultClipboardSetting            = 'clipboard'
    DefaultAutomaticDownloadsSetting   = 'automatic_downloads'
    DefaultFileSystemReadGuardSetting  = 'file_system_read_guard'
    DefaultFileSystemWriteGuardSetting = 'file_system_write_guard'
    DefaultWebBluetoothGuardSetting    = 'web_bluetooth_guard'
    DefaultWebUsbGuardSetting          = 'web_usb_guard'
    DefaultWebHidGuardSetting          = 'web_hid_guard'
    DefaultSerialGuardSetting          = 'serial_guard'
    DefaultIdleDetectionSetting        = 'idle_detection'
    DefaultLocalFontsSetting           = 'local_fonts'
}

$BrowserDataPaths = @(
    (Join-Path $env:LOCALAPPDATA 'BraveSoftware\Brave-Browser\User Data'),
    (Join-Path $env:LOCALAPPDATA 'Microsoft\Edge\User Data')
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

function Get-BackupDWordValue {
    param($Entry)
    $Values = @($Entry.Value)
    if ($Values.Count -ne 1 -or $null -eq $Values[0]) {
        throw "Invalid backup DWORD value for $($Entry.Path) / $($Entry.Name): expected one number, got $($Values.Count)."
    }
    $ParsedValue = 0
    if (-not [int]::TryParse([string]$Values[0], [ref]$ParsedValue)) {
        throw "Invalid backup DWORD value for $($Entry.Path) / $($Entry.Name): '$($Values[0])'."
    }
    return $ParsedValue
}

# Editing Preferences while the browsers are running risks losing changes.
if (Get-Process -Name brave,msedge -ErrorAction SilentlyContinue) {
    throw 'Close Brave and Edge completely, including background processes, before running this script.'
}

if ($Restore) {
    if (-not (Test-Path -LiteralPath $BackupFile)) {
        throw "Backup not found: $BackupFile. No changes were made."
    }
    # Expand JSON array entries individually (PowerShell 5.1 may retain an outer array).
    $ParsedBackup = Get-Content -LiteralPath $BackupFile -Raw | ConvertFrom-Json
    $Backup = @(
        foreach ($Item in $ParsedBackup) {
            foreach ($Record in @($Item)) {
                if ($Record.Path -isnot [string] -or $Record.Name -isnot [string]) {
                    throw "Invalid backup entry structure: $BackupFile"
                }
                $Record
            }
        }
    )
    foreach ($Entry in $Backup) {
        if ($Entry.Existed) {
            if (-not (Test-Path -LiteralPath $Entry.Path)) {
                New-Item -Path $Entry.Path -Force | Out-Null
            }
            New-ItemProperty -LiteralPath $Entry.Path -Name $Entry.Name -PropertyType DWord -Value (Get-BackupDWordValue -Entry $Entry) -Force | Out-Null
        }
        elseif (Test-RegistryValue -Path $Entry.Path -Name $Entry.Name) {
            Remove-ItemProperty -LiteralPath $Entry.Path -Name $Entry.Name
        }
    }
    Write-Host 'Previous policy values restored. Restart Brave and Edge.' -ForegroundColor Green
    Write-Host "Backup kept at: $BackupFile"
    Write-Warning 'This restores Registry policies only; profile Preferences backups are saved alongside each Preferences file (*.before-block-editable.bak).'
    exit 0
}

# Keep the original snapshot on subsequent runs to allow a true restore.
if (-not (Test-Path -LiteralPath $BackupFile)) {
    throw "Original policy backup not found: $BackupFile. Cannot safely identify policies created by the previous script."
}
else {
    Write-Host "Using existing backup: $BackupFile"
}

# Expand JSON array entries individually (PowerShell 5.1 may retain an outer array).
    $ParsedBackup = Get-Content -LiteralPath $BackupFile -Raw | ConvertFrom-Json
    $Backup = @(
        foreach ($Item in $ParsedBackup) {
            foreach ($Record in @($Item)) {
                if ($Record.Path -isnot [string] -or $Record.Name -isnot [string]) {
                    throw "Invalid backup entry structure: $BackupFile"
                }
                $Record
            }
        }
    )
foreach ($Path in $BrowserPaths) {
    foreach ($Name in (Get-AllPolicyNames)) {
        $Entry = @($Backup | Where-Object { $_.Path -eq $Path -and $_.Name -eq $Name })
        if ($Entry.Count -ne 1) { throw "Missing or duplicate backup entry: $Path / $Name" }
        if ($Entry[0].Existed) { $null = Get-BackupDWordValue -Entry $Entry[0] }
    }
}

# Backup and update existing profiles only. Do not create or replace profiles.
foreach ($Directory in $BrowserDataPaths) {
    if (-not (Test-Path -LiteralPath $Directory)) { continue }
    $Profiles = @(Get-ChildItem -LiteralPath $Directory -Directory | Where-Object { $_.Name -eq 'Default' -or $_.Name -match '^Profile \d+$' })
    foreach ($Profile in $Profiles) {
        $PreferencesPath = Join-Path $Profile.FullName 'Preferences'
        if (-not (Test-Path -LiteralPath $PreferencesPath)) { continue }

        $Preferences = Get-Content -LiteralPath $PreferencesPath -Raw -Encoding UTF8 | ConvertFrom-Json
        if (-not $Preferences.profile) {
            $Preferences | Add-Member -NotePropertyName 'profile' -NotePropertyValue ([pscustomobject]@{}) -Force
        }
        if (-not $Preferences.profile.default_content_setting_values) {
            $Preferences.profile | Add-Member -NotePropertyName 'default_content_setting_values' -NotePropertyValue ([pscustomobject]@{}) -Force
        }
        foreach ($Name in $BlockedPolicies.Keys) {
            $Setting = $PreferenceNames[$Name]
            $Preferences.profile.default_content_setting_values | Add-Member -NotePropertyName $Setting -NotePropertyValue 2 -Force
        }
        $Preferences.profile | Add-Member -NotePropertyName 'block_third_party_cookies' -NotePropertyValue $true -Force

        $PreferencesBackupPath = "$PreferencesPath.before-block-editable.bak"
        if (-not (Test-Path -LiteralPath $PreferencesBackupPath)) {
            Copy-Item -LiteralPath $PreferencesPath -Destination $PreferencesBackupPath -ErrorAction Stop
        }
        $TemporaryPath = "$PreferencesPath.block-editable.tmp"
        try {
            $Json = ConvertTo-Json -InputObject $Preferences -Depth 100 -Compress
            [System.IO.File]::WriteAllText($TemporaryPath, $Json, (New-Object System.Text.UTF8Encoding($false)))
            Move-Item -LiteralPath $TemporaryPath -Destination $PreferencesPath -Force
        }
        finally {
            if (Test-Path -LiteralPath $TemporaryPath) { Remove-Item -LiteralPath $TemporaryPath -Force }
        }
        Write-Host "Editable defaults applied: $PreferencesPath" -ForegroundColor Green
    }
}

# Remove only the policies that did not exist in the original backup.
foreach ($Path in $BrowserPaths) {
    foreach ($Name in (Get-AllPolicyNames)) {
        $Entry = $Backup | Where-Object { $_.Path -eq $Path -and $_.Name -eq $Name } | Select-Object -First 1
        if ($null -eq $Entry) { throw "Backup entry not found: $Path / $Name" }
        if ($Entry.Existed) {
            # Preserve policies that predated the previous script.
            New-ItemProperty -LiteralPath $Path -Name $Name -PropertyType DWord -Value (Get-BackupDWordValue -Entry $Entry) -Force | Out-Null
        }
        elseif (Test-RegistryValue -Path $Path -Name $Name) {
            Remove-ItemProperty -LiteralPath $Path -Name $Name
        }
    }
    Write-Host "Original policies restored: $Path" -ForegroundColor Green
}

Write-Host ''
Write-Host 'Restart both browsers and check brave://policy and edge://policy.'
Write-Warning 'JavaScript, images and all cookies are blocked by default. Many websites will not work.'
Write-Warning 'Preferences are editable, but not every Chromium setting supports per-site exceptions or the value 2.'
Write-Host 'To restore Registry policies: .\block-editable.ps1 -Restore'
