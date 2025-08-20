# Ejecutar como Administrador
$driverBlocks = pnputil /enum-drivers | Out-String -Stream | Where-Object { $_ -match "Published Name|Driver Name" }

for ($i = 0; $i -lt $driverBlocks.Count - 1; $i++) {
    if ($driverBlocks[$i] -like "*Published Name*") {
        $nameLine = $driverBlocks[$i]
        $driverLine = $driverBlocks[$i + 1]

        if ($driverLine -like "*Generic Bluetooth Adapter*") {
            $publishedName = ($nameLine -split ":")[1].Trim()
            Write-Host "Eliminando $publishedName ($driverLine)"
            pnputil /delete-driver $publishedName /uninstall /force
        }
    }
}
