# ⚠️ AVISO IMPORTANTE SI FALLA EL ATTACH
# ============================================================================
# AVISO IMPORTANTE SI FALLA EL ATTACH
# ============================================================================

Write-Host ""
Write-Host "══════════════════════════════════════════════════════════════════════" -ForegroundColor Yellow
Write-Host " ADVERTENCIA: Problemas al hacer ATTACH en 'C:\Program Files'" -ForegroundColor Yellow
Write-Host "══════════════════════════════════════════════════════════════════════" -ForegroundColor Yellow
Write-Host ""
Write-Host " 'C:\Program Files' es una zona protegida por Windows." -ForegroundColor Yellow
Write-Host " Puede causar errores como 'Access Denied' incluso con permisos correctos." -ForegroundColor Yellow
Write-Host ""
Write-Host " SOLUCIÓN RECOMENDADA:" -ForegroundColor Cyan
Write-Host "  Mover los archivos .mdf y .ldf a una ruta menos protegida, por ejemplo:" -ForegroundColor Cyan
Write-Host ""
Write-Host "     C:\SQLData\<DatabaseName>.mdf" -ForegroundColor White
Write-Host "     C:\SQLData\<DatabaseName>.ldf" -ForegroundColor White
Write-Host ""
Write-Host " Luego hacé el 'Attach' desde SSMS o con T-SQL directamente." -ForegroundColor Cyan
Write-Host "══════════════════════════════════════════════════════════════════════" -ForegroundColor Yellow
Write-Host ""

# ============================================================================
# CONFIGURACIÓN DE RUTAS Y PERMISOS
# ============================================================================

$sourcePath = "C:\Program Files\Microsoft SQL Server\MSSQL16.GERMA\MSSQL\DATA"
$targetPath = "C:\Program Files\Microsoft SQL Server\MSSQL16.GERMA\MSSQL\OLD_DATA"

# 1. Copiar ACL de carpeta
$folderAcl = Get-Acl $sourcePath
Set-Acl -Path $targetPath -AclObject $folderAcl
Write-Host "Permisos copiados de '$sourcePath' a '$targetPath'."

# 2. Propietario y grupo estándar
$account = New-Object System.Security.Principal.NTAccount("NT SERVICE\MSSQLSERVER")

# 3. Aplicar cambios a archivos .mdf y .ldf
$extensions = "*.mdf", "*.ldf"
foreach ($ext in $extensions) {
    Get-ChildItem -Path $targetPath -Filter $ext | ForEach-Object {
        $filePath = $_.FullName

        # Obtener ACL actual
        $acl = Get-Acl $filePath

        # Cambiar propietario y grupo
        $acl.SetOwner($account)
        $acl.SetGroup($account)

        # Agregar OWNER RIGHTS si no existe
        $hasOwnerRights = $acl.Access | Where-Object {
            $_.IdentityReference -eq "OWNER RIGHTS" -and $_.FileSystemRights -eq "FullControl"
        }

        if (-not $hasOwnerRights) {
            $ownerRule = New-Object System.Security.AccessControl.FileSystemAccessRule(
                "OWNER RIGHTS", "FullControl", "Allow"
            )
            $acl.AddAccessRule($ownerRule)
        }

        # Aplicar cambios
        Set-Acl -Path $filePath -AclObject $acl
        Write-Host "Permisos ajustados en: $filePath"
    }
}
