# Define el PSPath de la aplicación que quieres desinstalar
$applicationPath = "HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\<Identificador de Aplicación>"

# Se obtiene el UninstallString del registro que se usa para desinstalar la aplicación
$uninstallString = (Get-ItemProperty -Path $applicationPath).UninstallString

# Ejecutar el comando de desinstalación
if ($uninstallString) {
    Start-Process cmd.exe -ArgumentList "/c $uninstallString" -Wait
} else {
    Write-Host "No se encontró el comando de desinstalación para la aplicación en $applicationPath"
}
