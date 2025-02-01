# Obtener todos los archivos .java en el directorio actual y subdirectorios
$files = Get-ChildItem -Recurse -Filter *.java

# Eliminar el archivo output.txt si ya existe
if (Test-Path output.txt) {
    Remove-Item output.txt
}

# Crear una instancia StreamWriter con UTF8 sin BOM
$utf8WithoutBom = New-Object System.Text.UTF8Encoding $false
$stream = New-Object System.IO.StreamWriter "output.txt", $true, $utf8WithoutBom

# Recorrer cada archivo .java
foreach ($file in $files) {
    # Ignorar archivos en el directorio 'lib' y subdirectorios de 'lib'
    if ($file.DirectoryName -notlike '*\lib*') {
        # Agregar el nombre del archivo al output.txt
        $stream.WriteLine("archivo: $($file.Name)`r`n``````java")

        # Agregar el contenido del archivo .java al output.txt
        $reader = New-Object System.IO.StreamReader $file.FullName, $utf8WithoutBom
        while($reader.Peek() -ge 0) {
            $stream.WriteLine($reader.ReadLine())
        }
        $reader.Close()

        # Agregar el final del bloque al output.txt
        $stream.WriteLine("``````")
    }
}

# Cerrar el StreamWriter al final
$stream.Close()
