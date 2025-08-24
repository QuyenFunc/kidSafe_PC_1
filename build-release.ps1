# KidSafe PC Build Script
# Tạo một executable duy nhất cho việc distribution

Write-Host "🛡️  Building KidSafe PC Release..." -ForegroundColor Green

# 1. Build Go Core Service
Write-Host "📦 Building Core Service..." -ForegroundColor Yellow
Set-Location "core-service"
go build -v -ldflags="-s -w" -o "../ui-admin/parental-control-core.exe"
if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Core service build failed!" -ForegroundColor Red
    exit 1
}
Set-Location ".."

# 2. Install Electron dependencies
Write-Host "📦 Installing Electron dependencies..." -ForegroundColor Yellow
Set-Location "ui-admin"
npm install
if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ npm install failed!" -ForegroundColor Red
    exit 1
}

# 3. Build Electron App
Write-Host "📦 Building Electron App..." -ForegroundColor Yellow
npx electron-builder --win
if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Electron build failed!" -ForegroundColor Red
    exit 1
}

Set-Location ".."

Write-Host "✅ Build completed successfully!" -ForegroundColor Green
Write-Host "📂 Check 'ui-admin/dist' folder for the installer" -ForegroundColor Cyan
Write-Host "🚀 Ready for distribution!" -ForegroundColor Green