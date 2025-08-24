@echo off
echo Building KidSafe PC - Simple Hosts Blocker...

echo.
echo Step 1: Building Core Service...
cd core-service
go mod tidy
go build -ldflags="-s -w" -o ..\ui-admin\parental-control-core.exe
if %errorlevel% neq 0 (
    echo ERROR: Failed to build core service
    pause
    exit /b 1
)
echo ✓ Core service built successfully

cd ..\ui-admin
echo.
echo Step 2: Installing UI Dependencies...
call npm install
if %errorlevel% neq 0 (
    echo ERROR: Failed to install npm dependencies
    pause
    exit /b 1
)
echo ✓ UI dependencies installed

echo.
echo ========================================
echo ✓ BUILD COMPLETE!
echo ========================================
echo.
echo To run KidSafe PC:
echo 1. Open PowerShell as Administrator
echo 2. cd ui-admin
echo 3. npm start
echo.
echo Features:
echo • Simple hosts file blocking (like Cold Turkey)
echo • Clean and minimal interface
echo • Administrator privileges required
echo.

cd ..
pause
