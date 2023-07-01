# Obtener todos los archivos .php en el directorio actual y subdirectorios
$files = Get-ChildItem -Recurse -Filter *.php

# Eliminar el archivo output.txt si ya existe
if (Test-Path output.txt) {
    Remove-Item output.txt
}

# Recorrer cada archivo .php
foreach ($file in $files) {
    # Agregar el nombre del archivo al output.txt
    "archivo: $($file.Name)`r`n``````php" | Out-File output.txt -Encoding utf8 -Append

    # Agregar el contenido del archivo .php al output.txt
    Get-Content $file.FullName -Encoding utf8 | Out-File output.txt -Encoding utf8 -Append

    # Agregar el final del bloque al output.txt
    "``````" | Out-File output.txt -Encoding utf8 -Append
}
