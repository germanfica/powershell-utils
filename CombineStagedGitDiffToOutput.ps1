# Eliminar el archivo output.txt si ya existe
if (Test-Path output.txt) {
    Remove-Item output.txt
}

# Crear una instancia StreamWriter con UTF8 sin BOM
$utf8WithoutBom = New-Object System.Text.UTF8Encoding $false
$stream = New-Object System.IO.StreamWriter "output.txt", $false, $utf8WithoutBom

# Escribir el encabezado en output.txt
$stream.WriteLine("Git Diff --Staged Output:")

# Variable para controlar si estamos en el primer archivo
$firstFile = $true

# Ejecutar git diff --staged y procesar la salida línea por línea
& git diff --staged . 2>&1 | ForEach-Object {
    # Si la línea comienza con "diff --git", es el inicio de un nuevo archivo
    if ($_ -match "^diff --git") {
        # Si no es el primer archivo, añadir separador y salto de línea
        if (-not $firstFile) {
            $stream.WriteLine("``````")
            $stream.WriteLine("")
        }

        # Añadir un separador inicial antes del nuevo archivo
        $stream.WriteLine("``````")

        # No es el primer archivo anymore
        $firstFile = $false
    }

    # Escribir la línea actual al archivo
    $stream.WriteLine($_)
}

# Añadir un separador final después del último archivo
$stream.WriteLine("``````")

# Cerrar el StreamWriter al final
$stream.Close()
