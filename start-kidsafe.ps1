# KidSafe PC Launcher
# Khởi chạy ứng dụng desktop

Write-Host "🛡️  Starting KidSafe PC..." -ForegroundColor Green

# Check if admin rights needed
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole] "Administrator")

if (-not $isAdmin) {
    Write-Host "⚠️  KidSafe requires Administrator privileges to modify hosts file" -ForegroundColor Yellow
    Write-Host "🔄 Restarting with admin rights..." -ForegroundColor Yellow
    
    # Restart as administrator
    Start-Process PowerShell -Verb RunAs -ArgumentList "-NoProfile -ExecutionPolicy Bypass -File `"$PSCommandPath`""
    exit
}

Write-Host "✅ Running with Administrator privileges" -ForegroundColor Green

# Navigate to ui-admin folder and start
Set-Location "$PSScriptRoot\ui-admin"
Write-Host "🚀 Launching KidSafe PC Desktop App..." -ForegroundColor Cyan

npm start

