
$ErrorActionPreference = "Stop"

$world = "YOUR_LOCATION"
$level = Join-Path $world "level.dat"
$backup = Join-Path $world ("level.dat.backup-" + (Get-Date -Format "yyyyMMdd-HHmmss"))
$temp = Join-Path $world "level.dat.tmp"

if (!(Test-Path -LiteralPath $level)) {
    throw "No se encontro level.dat: $level"
}

# 1. Backup antes de modificar nada
Copy-Item -LiteralPath $level -Destination $backup -ErrorAction Stop
Write-Host "Backup creado: $backup" -ForegroundColor Green

try {
    # 2. Descomprimir level.dat (NBT Java / GZIP)
    $compressed = [IO.File]::ReadAllBytes($level)
    $inputStream = [IO.MemoryStream]::new($compressed)
    $gzip = [IO.Compression.GZipStream]::new(
        $inputStream,
        [IO.Compression.CompressionMode]::Decompress
    )
    $outputStream = [IO.MemoryStream]::new()

    try {
        $gzip.CopyTo($outputStream)
        $data = $outputStream.ToArray()
    }
    finally {
        $gzip.Dispose()
        $inputStream.Dispose()
        $outputStream.Dispose()
    }

    # 3. Buscar TAG_String "randomTickSpeed"
    $name = [Text.Encoding]::ASCII.GetBytes("randomTickSpeed")
    $pattern = [byte[]]@(8, 0, $name.Length) + $name

    $matches = @()

    for ($i = 0; $i -le $data.Length - $pattern.Length - 2; $i++) {
        $found = $true

        for ($j = 0; $j -lt $pattern.Length; $j++) {
            if ($data[$i + $j] -ne $pattern[$j]) {
                $found = $false
                break
            }
        }

        if ($found) {
            $matches += $i
        }
    }

    if ($matches.Count -ne 1) {
        throw "Se esperaban 1 coincidencia de randomTickSpeed; encontradas: $($matches.Count)"
    }

    $position = $matches[0] + $pattern.Length

    # Longitud NBT big-endian
    $oldLength = ([int]$data[$position] -shl 8) -bor [int]$data[$position + 1]
    $valueStart = $position + 2

    if ($valueStart + $oldLength -gt $data.Length) {
        throw "Longitud NBT invalida"
    }

    $oldValue = [Text.Encoding]::UTF8.GetString(
        $data, $valueStart, $oldLength
    )

    if ($oldValue -notmatch '^\d+$') {
        throw "Valor inesperado: $oldValue"
    }

    Write-Host "randomTickSpeed anterior: $oldValue" -ForegroundColor Yellow

    # 4. Reconstruir los bytes del archivo
    $newValue = [Text.Encoding]::UTF8.GetBytes("3")
    $newData = [IO.MemoryStream]::new()

    try {
        $newData.Write($data, 0, $position)

        # Nueva longitud: 1 byte (big-endian)
        $newData.WriteByte(0)
        $newData.WriteByte(1)
        $newData.Write($newValue, 0, $newValue.Length)

        $remainingStart = $valueStart + $oldLength
        $newData.Write(
            $data,
            $remainingStart,
            $data.Length - $remainingStart
        )

        $fixedData = $newData.ToArray()
    }
    finally {
        $newData.Dispose()
    }

    # 5. Comprimir nuevamente en un archivo temporal
    $fileStream = [IO.File]::Create($temp)

    try {
        $gzipOut = [IO.Compression.GZipStream]::new(
            $fileStream,
            [IO.Compression.CompressionMode]::Compress
        )

        try {
            $gzipOut.Write($fixedData, 0, $fixedData.Length)
        }
        finally {
            $gzipOut.Dispose()
        }
    }
    finally {
        $fileStream.Dispose()
    }

    # 6. Reemplazar el archivo original
    [IO.File]::Copy($temp, $level, $true)

    Write-Host "randomTickSpeed restaurado a 3" -ForegroundColor Green
    Write-Host "Archivo corregido: $level"
    Write-Host "Backup disponible: $backup"
}
finally {
    if (Test-Path -LiteralPath $temp) {
        Remove-Item -LiteralPath $temp -Force
    }
}
