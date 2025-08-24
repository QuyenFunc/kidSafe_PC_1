# KidSafe PC Database Tools
# Utility script to manage database operations

param(
    [Parameter(Mandatory=$true)]
    [ValidateSet("check", "sync", "help")]
    [string]$Command
)

function Show-Help {
    Write-Host "KidSafe PC Database Tools" -ForegroundColor Cyan
    Write-Host "=========================" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "Usage: .\db-tools.ps1 <command>" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "Commands:" -ForegroundColor Green
    Write-Host "  check    - Check database contents and show all rules" -ForegroundColor White
    Write-Host "  sync     - Sync sample Firebase URLs to database" -ForegroundColor White
    Write-Host "  help     - Show this help message" -ForegroundColor White
    Write-Host ""
    Write-Host "Examples:" -ForegroundColor Green
    Write-Host "  .\db-tools.ps1 check" -ForegroundColor Gray
    Write-Host "  .\db-tools.ps1 sync" -ForegroundColor Gray
}

function Invoke-DatabaseCheck {
    Write-Host "Checking database contents..." -ForegroundColor Cyan

    # Create a temporary Go file to call the utility function
    $tempFile = "temp-check.go"
    $goCode = @'
package main

func main() {
    CheckDatabaseContents()
}
'@

    Set-Content -Path "core-service\$tempFile" -Value $goCode

    try {
        Set-Location "core-service"
        go run $tempFile db-utils.go
    }
    finally {
        Set-Location ".."
        Remove-Item "core-service\$tempFile" -ErrorAction SilentlyContinue
    }
}

function Invoke-FirebaseSync {
    Write-Host "Syncing sample Firebase URLs..." -ForegroundColor Cyan

    # Create a temporary Go file to call the utility function
    $tempFile = "temp-sync.go"
    $goCode = @'
package main

func main() {
    SyncSampleFirebaseUrls()
}
'@

    Set-Content -Path "core-service\$tempFile" -Value $goCode

    try {
        Set-Location "core-service"
        go run $tempFile db-utils.go
    }
    finally {
        Set-Location ".."
        Remove-Item "core-service\$tempFile" -ErrorAction SilentlyContinue
    }
}

# Main execution
switch ($Command) {
    "check" {
        Invoke-DatabaseCheck
    }
    "sync" {
        Invoke-FirebaseSync
    }
    "help" {
        Show-Help
    }
    default {
        Write-Host "Unknown command: $Command" -ForegroundColor Red
        Show-Help
    }
}
