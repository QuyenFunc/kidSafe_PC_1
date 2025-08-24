@echo off
chcp 65001 >nul
echo ========================================
echo KIDSAFE PC BROWSER EXTENSION INSTALLER
echo ========================================
echo.
echo This extension will show friendly messages 
echo when children try to access blocked websites.
echo.

echo Step 1: Open Chrome and go to chrome://extensions/
echo.
echo Step 2: Enable "Developer mode" in top right corner
echo.
echo Step 3: Click "Load unpacked" and select folder:
echo %~dp0
echo.
echo Step 4: Extension will appear in the list
echo.
echo ========================================
echo DETAILED INSTRUCTIONS:
echo ========================================
echo.
echo 1. Open Google Chrome
echo 2. Type in address bar: chrome://extensions/
echo 3. Enable "Developer mode" toggle
echo 4. Click "Load unpacked" button
echo 5. Select this folder: %~dp0
echo 6. Extension "KidSafe PC - Protection Extension" will appear
echo 7. Make sure extension is enabled (toggle switch)
echo.
echo ========================================
echo IMPORTANT NOTES:
echo ========================================
echo.
echo - Extension needs access to all websites to work
echo - Allow extension when Chrome asks for permissions
echo - Extension works best with KidSafe PC application
echo - Restart Chrome after installation for best performance
echo.
echo Press any key to open extension folder...
pause >nul

explorer "%~dp0"

echo.
echo Extension folder opened!
echo Follow the instructions above to install in Chrome.
echo.
pause
