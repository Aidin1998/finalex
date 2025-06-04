# Script to safely clean up duplicate directories outside of core/
# This script verifies that directories have been migrated to core/ before deletion

$internalPath = "c:\Orbit CEX\pincex_unified\internal"
$corePath = "$internalPath\core"

# Define the migration mapping
$migrationMap = @{
    "analytics" = "marketmaking\analytics"
    "audit" = "risk\audit"
    "bookkeeper" = "accounts\bookkeeper"
    "cache" = "database\cache"
    "compliance" = "risk\compliance"
    "config" = "infrastructure\config"
    "consensus" = "trading\consensus"
    "consistency" = "trading\consistency"
    "coordination" = "trading\coordination"
    "integration" = "trading\integration"
    "kyc" = "userauth\kyc"
    "manipulation" = "risk\manipulation"
    "marketdata" = "marketmaking\marketdata"
    "marketfeeds" = "marketmaking\marketfeeds"
    "marketmaker" = "marketmaking\marketmaker"
    "messaging" = "infrastructure\messaging"
    "middleware" = "infrastructure\middleware"
    "monitoring" = "risk\monitoring"
    "orderqueue" = "trading\orderqueue"
    "redis" = "database\redis"
    "server" = "infrastructure\server"
    "settlement" = "trading\settlement"
    "transaction" = "accounts\transaction"
    "ws" = "infrastructure\ws"
}

# Directories that have exact matches in core (no sub-mapping)
$exactMatches = @("database", "fiat", "trading", "userauth", "wallet")

function Test-DirectoryMigrated {
    param(
        [string]$SourceDir,
        [string]$TargetDir
    )
    
    if (-not (Test-Path $TargetDir)) {
        Write-Host "ERROR: Target directory does not exist: $TargetDir" -ForegroundColor Red
        return $false
    }
    
    Write-Host "Checking migration: $SourceDir -> $TargetDir" -ForegroundColor Yellow
    return $true
}

function Remove-MigratedDirectory {
    param(
        [string]$DirPath,
        [string]$DirName
    )
    
    if (Test-Path $DirPath) {
        Write-Host "Removing migrated directory: $DirName" -ForegroundColor Green
        Remove-Item -Path $DirPath -Recurse -Force
        Write-Host "Successfully removed: $DirName" -ForegroundColor Green
    } else {
        Write-Host "Directory already removed: $DirName" -ForegroundColor Gray
    }
}

Write-Host "Starting cleanup of migrated directories..." -ForegroundColor Cyan
Write-Host "Internal path: $internalPath" -ForegroundColor Cyan
Write-Host "Core path: $corePath" -ForegroundColor Cyan

# Process mapped directories
foreach ($sourceDir in $migrationMap.Keys) {
    $sourcePath = "$internalPath\$sourceDir"
    $targetPath = "$corePath\$($migrationMap[$sourceDir])"
    
    if (Test-Path $sourcePath) {
        if (Test-DirectoryMigrated -SourceDir $sourcePath -TargetDir $targetPath) {
            Remove-MigratedDirectory -DirPath $sourcePath -DirName $sourceDir
        } else {
            Write-Host "Skipping $sourceDir - migration verification failed" -ForegroundColor Red
        }
    }
}

# Process exact match directories
foreach ($dirName in $exactMatches) {
    $sourcePath = "$internalPath\$dirName"
    $targetPath = "$corePath\$dirName"
    
    if (Test-Path $sourcePath) {
        if (Test-DirectoryMigrated -SourceDir $sourcePath -TargetDir $targetPath) {
            # For exact matches, we need to be more careful - only remove if core version has more content
            $sourceItems = Get-ChildItem -Path $sourcePath -Recurse | Measure-Object
            $targetItems = Get-ChildItem -Path $targetPath -Recurse | Measure-Object
            
            if ($targetItems.Count -ge $sourceItems.Count) {
                Remove-MigratedDirectory -DirPath $sourcePath -DirName $dirName
            } else {
                Write-Host "WARNING: Core version of $dirName appears incomplete. Manual review required." -ForegroundColor Yellow
            }
        }
    }
}

Write-Host "`nCleanup completed!" -ForegroundColor Cyan
Write-Host "Remaining directories in internal/:" -ForegroundColor Cyan
Get-ChildItem -Path $internalPath -Directory | Where-Object { $_.Name -ne "core" } | ForEach-Object { Write-Host "  - $($_.Name)" -ForegroundColor Yellow }
