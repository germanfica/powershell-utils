# Obtener todos los directorios en el directorio actual, incluyendo los ocultos
$directories = Get-ChildItem -Directory -Force

# Imprimir los atributos de cada directorio
foreach ($dir in $directories) {
  $isHidden = [bool]($dir.Attributes -band [System.IO.FileAttributes]::Hidden)
  Write-Output "Nombre: $($dir.Name)"
  Write-Output "Es oculto: $($isHidden)"
  Write-Output "Ruta completa: $($dir.FullName)"
  Write-Output "Fecha de última modificación: $($dir.LastWriteTime)"
  Write-Output "---------"
}

# Preparar los nombres de los directorios no ocultos
$content = $directories | Where-Object { -not ($_.Attributes -band [System.IO.FileAttributes]::Hidden) } |
ForEach-Object { "`"node_modules/$($_.Name)/**/*`"" }

# Unir las cadenas con comas y saltos de línea
$joinedContent = $content -join ",`n  "

# Formatear el contenido para la estructura JSON
$jsonContent = @"
"files": [
  $joinedContent
]
"@

# Escribir en el archivo output.json
$jsonContent | Out-File -FilePath "output.json" -Encoding UTF8
