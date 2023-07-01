# Obtener todos los archivos .php en el directorio actual y subdirectorios
$files = Get-ChildItem -Recurse -Filter *.php

# Eliminar el archivo output.txt si ya existe
if (Test-Path output.txt) {
    Remove-Item output.txt
}

# Crear una instancia StreamWriter con UTF8 sin BOM
$utf8WithoutBom = New-Object System.Text.UTF8Encoding $false
$stream = New-Object System.IO.StreamWriter "output.txt", $true, $utf8WithoutBom

# Recorrer cada archivo .php
foreach ($file in $files) {
    # Agregar el nombre del archivo al output.txt
    $stream.WriteLine("archivo: $($file.Name)`r`n``````php")

    # Agregar el contenido del archivo .php al output.txt
    $content = Get-Content $file.FullName -Encoding utf8
    foreach ($line in $content) {
        $stream.WriteLine($line)
    }

    # Agregar el final del bloque al output.txt
    $stream.WriteLine("``````")
}

# Cerrar el StreamWriter al final
$stream.Close()
