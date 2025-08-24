# KidSafe PC Admin Restart Script
# Run this script as Administrator

Write-Host "🛡️  KidSafe PC Admin Restart" -ForegroundColor Green
Write-Host "==============================" -ForegroundColor Green

# Check if running as administrator
if (-NOT ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole] "Administrator")) {
    Write-Host "❌ This script must be run as Administrator!" -ForegroundColor Red
    Write-Host "Right-click and select 'Run as Administrator'" -ForegroundColor Yellow
    pause
    exit 1
}

Write-Host "✅ Running with Administrator privileges" -ForegroundColor Green

# Step 1: Force kill all related processes
Write-Host "🔄 Stopping all KidSafe processes..." -ForegroundColor Yellow
try {
    Get-Process | Where-Object {$_.ProcessName -match "electron|parental-control"} | Stop-Process -Force
    Write-Host "✅ Processes stopped successfully" -ForegroundColor Green
} catch {
    Write-Host "⚠️  Some processes may have already been stopped" -ForegroundColor Yellow
}

# Step 2: Wait a moment
Write-Host "⏳ Waiting 3 seconds..." -ForegroundColor Yellow
Start-Sleep -Seconds 3

# Step 3: Navigate and start
Write-Host "🚀 Starting KidSafe PC..." -ForegroundColor Green
Set-Location "$PSScriptRoot\ui-admin"

if (Test-Path "package.json") {
    npm start
} else {
    Write-Host "❌ package.json not found in ui-admin folder" -ForegroundColor Red
    Write-Host "Current location: $(Get-Location)" -ForegroundColor Yellow
}

Write-Host "🎉 KidSafe PC should now start with Login Window!" -ForegroundColor Green

