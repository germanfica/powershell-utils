# Obtener todos los archivos .cs en el directorio actual y subdirectorios
$files = Get-ChildItem -Recurse -Filter *.cs

# Eliminar el archivo output.txt si ya existe
if (Test-Path output.txt) {
    Remove-Item output.txt
}

# Crear una instancia StreamWriter con UTF8 sin BOM
$utf8WithoutBom = New-Object System.Text.UTF8Encoding $false
$stream = New-Object System.IO.StreamWriter "output.txt", $true, $utf8WithoutBom

# Recorrer cada archivo .cs
foreach ($file in $files) {
    # Ignorar archivos que terminan en .cs.meta
    if ($file.Name -like "*.cs.meta") {
        continue
    }

    # Agregar el nombre del archivo al output.txt
    $stream.WriteLine("archivo: $($file.Name)`r`n``````csharp")

    # Agregar el contenido del archivo .cs al output.txt
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