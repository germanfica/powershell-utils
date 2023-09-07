# Este script verifica el día de la semana
# y lanza la aplicación Mattermost si es un día laborable (de lunes a viernes).

# Obtener el día actual de la semana
$dayOfWeek = (Get-Date).DayOfWeek

# Verificar si el día actual es de lunes a viernes
if ($dayOfWeek -eq 'Monday' -or $dayOfWeek -eq 'Tuesday' -or $dayOfWeek -eq 'Wednesday' -or $dayOfWeek -eq 'Thursday' -or $dayOfWeek -eq 'Friday') {
  Start-Process "C:\Users\germa\AppData\Local\Programs\mattermost-desktop\Mattermost.exe"
}