# Inicializar un arreglo vacío para almacenar la información de los dispositivos
$devicesInfo = @()

# Obtener información de todos los dispositivos PnP
$allDevices = Get-WmiObject -Class Win32_PnPEntity

# Filtrar solo los dispositivos Bluetooth
$bluetoothDevices = $allDevices | Where-Object { $_.Name -like "*Bluetooth*" }

# Recorrer cada dispositivo Bluetooth y recopilar información
foreach ($device in $bluetoothDevices) {
    $deviceInfo = New-Object PSObject -Property @{
        "GUID"  = $device.PNPDeviceID
        "Name"  = $device.Name
        "Class" = $device.PNPClass
        "Status" = $device.Status
        "Manufacturer" = $device.Manufacturer
        "Service" = $device.Service
    }

    # Agregar la información del dispositivo al arreglo
    $devicesInfo += $deviceInfo
}

# Mostrar la información en la consola
$devicesInfo | Format-Table -Property "GUID", "Name", "Class", "Status", "Manufacturer", "Service"

