# Leer las líneas del archivo de entrada
$lines = Get-Content -Path "input.txt"

# Inicializar una lista vacía para almacenar las líneas de fin de mes
$endOfMonthLines = @()

# Recorrer cada línea del archivo
foreach ($line in $lines) {
    # Dividir la línea en partes utilizando el carácter de tabulación como delimitador
    $parts = $line -split "\t"

    # Obtener la fecha y el precio de las partes
    $date = $parts[0]
    $price = $parts[1]

    # Comprobar si la fecha corresponde al final del mes
    $day = [int]($date -split "/")[0]
    $nextDay = $day + 1
    $nextDate = $date -replace "^$day/", "$nextDay/"

    if ($nextDate -notin $lines -split "\t") {
        # Reemplazar la coma en el precio por un punto
        $price = $price -replace ",", "."

        # Agregar la línea de fin de mes a la lista
        Write-Host "Date" $date
        $endOfMonthLines += "$date`t$price"
    }
}

# Guardar las líneas de fin de mes en el archivo de salida
$endOfMonthLines | Out-File -FilePath "output.txt"

# Imprimir un mensaje para indicar que la operación se ha completado
Write-Host "El archivo output.txt ha sido creado con éxito."
