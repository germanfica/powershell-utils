# Ejecutar como Administrador
$folder = 'D:\Ruta\Al\Directorio'
$user   = 'MI_DOMINIO\miUsuario'

# Creamos un objeto NTAccount para el nuevo propietario
$newOwner = New-Object System.Security.Principal.NTAccount($user)

# Recorremos todo
Get-ChildItem -Path $folder -Recurse -Force | ForEach-Object {
    try {
        $path = $_.FullName
        $acl  = Get-Acl -Path $path
        $acl.SetOwner($newOwner)
        Set-Acl -Path $path -AclObject $acl
        Write-Host "Owner cambiado en $($path)"
    }
    catch {
        Write-Warning "No se pudo en $($path): $($_.Exception.Message)"
    }
}