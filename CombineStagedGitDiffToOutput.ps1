# Eliminar el archivo output.txt si ya existe
if (Test-Path output.txt) {
    Remove-Item output.txt
}

# Crear una instancia StreamWriter con UTF8 sin BOM
$utf8WithoutBom = New-Object System.Text.UTF8Encoding $false
$stream = New-Object System.IO.StreamWriter "output.txt", $true, $utf8WithoutBom

# Ejecutar git diff --staged y guardar la salida
$diffOutput = & git diff --staged .

# Escribir la salida en output.txt
$stream.WriteLine("Git Diff --Staged Output:")
$stream.WriteLine("``````")
$stream.WriteLine($diffOutput)
$stream.WriteLine("``````")

# Cerrar el StreamWriter al final
$stream.Close()
