$proxyUri = "http://127.0.0.1:8080"
$proxyCreds = Get-Credential  # Te pedirá usuario y contraseña para el proxy

$response = Invoke-WebRequest `
    -Uri "https://wsaa.afip.gov.ar/ws/services/LoginCms" `
    -Method POST `
    -Proxy $proxyUri `
    -ProxyCredential $proxyCreds `
    -Body "<soapenv:Envelope>...</soapenv:Envelope>" `
    -ContentType "text/xml"

$response.Content
