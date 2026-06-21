$ErrorActionPreference = "Stop"

$projectRoot = "C:\laragon\www\ReadeRSS"
$serverExe = Join-Path $projectRoot "bin\readress.exe"
$url = "http://localhost:8080/"
$port = 8080

function Test-PortListening {
	param([int]$Port)
	try {
		$listener = Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction Stop
		return [bool]$listener
	} catch {
		return $false
	}
}

if (-not (Test-PortListening -Port $port)) {
	if (-not (Test-Path $serverExe)) {
		Add-Type -AssemblyName PresentationFramework
		[System.Windows.MessageBox]::Show(
			"ReadeRSS server binary not found at $serverExe",
			"ReadeRSS Launcher",
			"OK",
			"Error"
		) | Out-Null
		exit 1
	}

	Start-Process -FilePath $serverExe -WorkingDirectory $projectRoot -WindowStyle Hidden

	for ($i = 0; $i -lt 30; $i++) {
		Start-Sleep -Milliseconds 500
		if (Test-PortListening -Port $port) {
			break
		}
	}
}

Start-Process $url
