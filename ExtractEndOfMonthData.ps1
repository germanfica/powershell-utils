# Leer el archivo de entrada línea por línea
$lines = Get-Content "input.txt"

# Inicializar un array vacío para almacenar los precios de fin de mes
$endOfMonthPrices = @()

# Iterar a través de cada línea del archivo
foreach ($line in $lines) {
    # Dividir la línea en campos basado en el tabulador
    $fields = $line -split "\t"
    
    # Extraer la fecha y el precio
    $date = $fields[0]
    $price = $fields[1]
    
    # Convertir la fecha a un objeto DateTime
    $dateObj = [datetime]::ParseExact($date, "dd/MM/yyyy", $null)
    
    # Determinar el último día del mes para esa fecha
    # $lastDayOfMonth = (Get-Date $dateObj).AddMonths(1).AddDays(-((Get-Date $dateObj).Day))
    # Obtener el último día del mes
    $lastDayOfMonth = [System.DateTime]::DaysInMonth($dateObj.Year, $dateObj.Month)

    # Crear un nuevo objeto DateTime para el último día del mes
    $dateWithLastDayOfMonth = New-Object System.DateTime($dateObj.Year, $dateObj.Month, $lastDayOfMonth)
    # Write-Host "Último día del mes" $dateObj $dateWithLastDayOfMonth

    # Verificar si la fecha es el último día del mes
    if ($dateObj -eq $dateWithLastDayOfMonth) {
        # Reemplazar la coma con un punto en el precio
        $price = $price -replace ",", "."
        
        # Añadir el precio al array
        $endOfMonthPrices += "$date`t$price"
    }
}

# Guardar los precios de fin de mes en output.txt
$endOfMonthPrices | Out-File "output.txt"

# Imprimir un mensaje para indicar que la operación se ha completado
Write-Host "El archivo output.txt ha sido creado con éxito."