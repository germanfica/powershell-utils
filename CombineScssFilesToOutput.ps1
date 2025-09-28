# Obtener todos los archivos .scss en el directorio actual y subdirectorios
$files = Get-ChildItem -Recurse -Filter *.scss |
Where-Object { $_.FullName -notmatch '\\node_modules\\' } |
Where-Object { $_.FullName -notmatch '\\dist\\' }

# Eliminar el archivo output.txt si ya existe
if (Test-Path output.txt) {
    Remove-Item output.txt
}

# Crear una instancia StreamWriter con UTF8 sin BOM
$utf8WithoutBom = New-Object System.Text.UTF8Encoding $false
$stream = New-Object System.IO.StreamWriter "output.txt", $true, $utf8WithoutBom

# Recorrer cada archivo .scss
foreach ($file in $files) {
    # Ignorar archivos que terminan en .spec.scss
    if ($file.Name -like "*.spec.scss") {
        continue
    }

    # Agregar el nombre del archivo al output.txt
    $stream.WriteLine("archivo: $($file.Name)`r`n``````typescript")

    # Agregar el contenido del archivo .scss al output.txt
    $reader = New-Object System.IO.StreamReader $file.FullName, $utf8WithoutBom
    while($reader.Peek() -ge 0) {
        $stream.WriteLine($reader.ReadLine())
    }
    $reader.Close()

    # Agregar el final del bloque al output.txt
    $stream.WriteLine("``````")
}

# Cerrar el StreamWriter al final
$stream.Close()