# Obtener todos los dispositivos PnP
$allDevices = Get-PnpDevice

# Inicializar arreglos vacíos para almacenar diferentes tipos de dispositivos
$pairedDevices = @()
$failedDevices = @()
$connectedDevices = @()
$disconnectedDevices = @()
$unknownStatusDevices = @()  # Arreglo para dispositivos con estado desconocido

# Filtrar y categorizar los dispositivos Bluetooth
foreach ($device in $allDevices) {
    if ($device.Class -eq "Bluetooth" -or $device.FriendlyName -like "*Bluetooth*") {
        $deviceInfo = New-Object PSObject -Property @{
            "DeviceID"     = $device.DeviceID
            "Name"         = $device.FriendlyName
            "Status"       = $device.Status
            "Manufacturer" = $device.Manufacturer
            "ConnectionStatus" = "Unknown"  # Agregar un campo de estado de conexión
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
                $deviceInfo.ConnectionStatus = "Connected"
            } else {
                $disconnectedDevices += $deviceInfo
                $deviceInfo.ConnectionStatus = "Disconnected"
            }
        } else {
            # Registrar que la propiedad "Data" no está disponible
            Write-Host "Warning: Could not obtain 'Data' property for device $($deviceInfo.Name)"
            $unknownStatusDevices += $deviceInfo  # Añadir al arreglo de dispositivos con estado desconocido
        }
    }
}

# Mostrar la información
Write-Host "Dispositivos emparejados:"
$pairedDevices | Format-Table -Property "DeviceID", "Name", "Status", "Manufacturer", "ConnectionStatus"

Write-Host "`nDispositivos que fallaron:"
$failedDevices | Format-Table -Property "DeviceID", "Name", "Status", "Manufacturer", "ConnectionStatus"

Write-Host "`nDispositivos conectados:"
$connectedDevices | Format-Table -Property "DeviceID", "Name", "Status", "Manufacturer", "ConnectionStatus"

Write-Host "`nDispositivos no conectados:"
$disconnectedDevices | Format-Table -Property "DeviceID", "Name", "Status", "Manufacturer", "ConnectionStatus"

Write-Host "`nDispositivos con estado desconocido:"
$unknownStatusDevices | Format-Table -Property "DeviceID", "Name", "Status", "Manufacturer", "ConnectionStatus"
