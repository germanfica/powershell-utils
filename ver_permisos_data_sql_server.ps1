$paths = @(
    "C:\Program Files\Microsoft SQL Server\MSSQL16.GERMA\MSSQL\DATA",
    "C:\Program Files\Microsoft SQL Server\MSSQL16.GERMA\MSSQL\OLD_DATA"
)

foreach ($path in $paths) {
    Write-Host "`n==========================" -ForegroundColor Cyan
    Write-Host "Permisos para: $path" -ForegroundColor Yellow
    Write-Host "==========================" -ForegroundColor Cyan

    $acl = Get-Acl $path
    $acl | Format-List
}
