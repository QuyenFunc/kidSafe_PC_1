# KidSafe PC Service Restart Script
# Stops current service and starts new version

Write-Host "Restarting KidSafe PC Service..." -ForegroundColor Cyan

# Stop any running KidSafe processes
Write-Host "Stopping existing KidSafe processes..." -ForegroundColor Yellow
Get-Process | Where-Object {$_.ProcessName -like "*kidsafe*"} | Stop-Process -Force -ErrorAction SilentlyContinue
Get-Process | Where-Object {$_.ProcessName -like "*parental*"} | Stop-Process -Force -ErrorAction SilentlyContinue

# Wait a moment
Start-Sleep -Seconds 2

# Check if port 8081 is still in use
$portInUse = Get-NetTCPConnection -LocalPort 8081 -ErrorAction SilentlyContinue
if ($portInUse) {
    Write-Host "Port 8081 still in use, waiting..." -ForegroundColor Yellow
    Start-Sleep -Seconds 3
}

# Start new service
Write-Host "Starting updated KidSafe PC service..." -ForegroundColor Green
Set-Location "$PSScriptRoot"

# Start the service with admin privileges
Start-Process PowerShell -Verb RunAs -ArgumentList "-NoProfile -ExecutionPolicy Bypass -Command `"cd '$PSScriptRoot'; .\core-service\kidsafe-pc.exe --no-ui`""

Write-Host "Service restart initiated!" -ForegroundColor Green
Write-Host "API should be available at: http://127.0.0.1:8081" -ForegroundColor Cyan
