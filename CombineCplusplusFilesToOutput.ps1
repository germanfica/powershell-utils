# Obtener todos los archivos .cpp, .h y .hpp en el directorio actual y subdirectorios, excluyendo build_msvc
$files = Get-ChildItem -Recurse -File |
Where-Object { $_.FullName -notmatch '\\build_msvc\\' } |
Where-Object { $_.Extension -in '.cpp', '.h', '.hpp' }

# Eliminar el archivo output.txt si ya existe
if (Test-Path output.txt) {
    Remove-Item output.txt
}

# Crear una instancia StreamWriter con UTF8 sin BOM
$utf8WithoutBom = New-Object System.Text.UTF8Encoding $false
$stream = New-Object System.IO.StreamWriter "output.txt", $true, $utf8WithoutBom

# Recorrer cada archivo seleccionado
foreach ($file in $files) {
    # Ignorar archivos en el directorio 'lib' y subdirectorios de 'lib'
    if ($file.DirectoryName -notlike '*\lib*') {
        # Determinar el tipo de bloque según la extensión
        $ext = $file.Extension.TrimStart('.').ToLower()

        # Agregar el nombre del archivo y el tipo de bloque correspondiente
        $stream.WriteLine("archivo: $($file.Name)`r`n``````$ext")

        # Agregar el contenido del archivo al output.txt
        $reader = New-Object System.IO.StreamReader $file.FullName, $utf8WithoutBom
        while ($reader.Peek() -ge 0) {
            $stream.WriteLine($reader.ReadLine())
        }
        $reader.Close()

        # Agregar el final del bloque al output.txt
        $stream.WriteLine("``````")
    }
}

# Cerrar el StreamWriter al final
$stream.Close()
