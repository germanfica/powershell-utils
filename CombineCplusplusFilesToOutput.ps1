# Obtener todos los archivos .cpp, .h y .hpp en el directorio actual y subdirectorios
$files = Get-ChildItem -Path . -Recurse -File |
Where-Object { $_.FullName -notmatch '\\build_msvc\\' } |
Where-Object { $_.Extension -in '.cpp', '.h', '.hpp' }

# Eliminar el archivo output.txt si ya existe
$outputPath = "output.txt"
if (Test-Path $outputPath) {
    Remove-Item $outputPath
}

# Crear una instancia StreamWriter con UTF8 sin BOM
$utf8WithoutBom = New-Object System.Text.UTF8Encoding $false
$stream = New-Object System.IO.StreamWriter $outputPath, $true, $utf8WithoutBom

# Recorrer cada archivo
foreach ($file in $files) {
    # Ignorar archivos .meta u otros archivos no fuente si es necesario
    # if ($file.Name -like "*.meta") {
    #     continue
    # }

    # Escribir encabezado con ruta relativa y abrir bloque de código como cpp
    $relative = $file.FullName.Substring((Get-Location).Path.Length).TrimStart('\','/')
    $stream.WriteLine("archivo: $relative`r`n```cpp")

    # Leer el archivo con StreamReader (UTF8 sin BOM) y escribir línea por línea
    $reader = New-Object System.IO.StreamReader $file.FullName, $utf8WithoutBom
    while ($reader.Peek() -ge 0) {
        $stream.WriteLine($reader.ReadLine())
    }
    $reader.Close()

    # Cerrar el bloque de código
    $stream.WriteLine("```")
    $stream.WriteLine("")  # linea en blanco entre archivos (opcional)
}

# Cerrar el StreamWriter al final
$stream.Close()
