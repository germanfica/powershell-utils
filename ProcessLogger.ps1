# Intervalo de tiempo en milisegundos para verificar los procesos en ejecución
$checkInterval = 5000

# Ruta del archivo de log
$logFilePath = "processLog.txt"

while ($true) {
    # Obtener todos los procesos en ejecución
    $processes = Get-Process
    foreach ($proc in $processes) {
        try {
            # Información básica del proceso
            $logEntry = "Timestamp: $(Get-Date), Process: $($proc.ProcessName), ID: $($proc.Id)"

            # Función para escribir en el log con reintentos
            function Write-Log {
                param([string]$entry, [string]$path)
                $attempt = 0
                $success = $false
                while (-not $success -and $attempt -lt 5) {
                    try {
                        # Intenta abrir el archivo, escribir el log y cerrarlo
                        $stream = [System.IO.StreamWriter]::new($path, $true)
                        $stream.WriteLine($entry)
                        $success = $true
                    }
                    catch {
                        Start-Sleep -Milliseconds 500
                    }
                    finally {
                        $stream.Close()
                    }
                    $attempt++
                }
            }

            # Intenta escribir en el log
            Write-Log -entry $logEntry -path $logFilePath
        }
        catch {
            # En caso de error, intenta escribir en el log
            $errorEntry = "Error logging process information: $($_.Exception.Message)"
            Write-Log -entry $errorEntry -path $logFilePath
        }
    }

    # Espera un tiempo antes de verificar nuevamente
    Start-Sleep -Milliseconds $checkInterval
}
