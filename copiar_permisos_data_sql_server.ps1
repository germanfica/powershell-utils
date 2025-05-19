$sourcePath = "C:\Program Files\Microsoft SQL Server\MSSQL16.GERMA\MSSQL\DATA"
$targetPath = "C:\Program Files\Microsoft SQL Server\MSSQL16.GERMA\MSSQL\OLD_DATA"

$acl = Get-Acl $sourcePath
Set-Acl -Path $targetPath -AclObject $acl
Write-Host "Permisos copiados de '$sourcePath' a '$targetPath'."
