# Obtener todos los dispositivos PnP
$allDevices = Get-PnpDevice

# Inicializar arreglos vacíos para almacenar diferentes tipos de dispositivos
$pairedDevices = @()
$failedDevices = @()
$connectedDevices = @()
$disconnectedDevices = @()

# Filtrar y categorizar los dispositivos Bluetooth
foreach ($device in $allDevices) {
    if ($device.Class -eq "Bluetooth" -or $device.FriendlyName -like "*Bluetooth*") {
        $deviceInfo = New-Object PSObject -Property @{
            "DeviceID"     = $device.DeviceID
            "Name"         = $device.FriendlyName
            "Status"       = $device.Status
            "Manufacturer" = $device.Manufacturer
        }

        # Categorizar el dispositivo según su estado
        switch ($device.Status) {
            "OK" { $pairedDevices += $deviceInfo }
            "Error" { $failedDevices += $deviceInfo }
        }

        # Obtener la propiedad del dispositivo, si está disponible
        $deviceProp = Get-PnpDeviceProperty -InstanceId $device.DeviceID -KeyName "DEVPKEY_Device_DevNodeStatus"
        if ($deviceProp -and $deviceProp.Data) {
            $deviceStatus = $deviceProp | Select-Object -ExpandProperty Data
            if ($deviceStatus -eq 0x1000a0) {
                $connectedDevices += $deviceInfo
            } else {
                $disconnectedDevices += $deviceInfo
            }
        }
    }
}

# Mostrar la información
Write-Host "Dispositivos emparejados:"
$pairedDevices | Format-Table -Property "DeviceID", "Name", "Status", "Manufacturer"

Write-Host "`nDispositivos que fallaron:"
$failedDevices | Format-Table -Property "DeviceID", "Name", "Status", "Manufacturer"

Write-Host "`nDispositivos conectados:"
$connectedDevices | Format-Table -Property "DeviceID", "Name", "Status", "Manufacturer"

Write-Host "`nDispositivos no conectados:"
$disconnectedDevices | Format-Table -Property "DeviceID", "Name", "Status", "Manufacturer"
