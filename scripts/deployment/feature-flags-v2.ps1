#!/usr/bin/env pwsh

# PinCEX Unified Exchange - Advanced Feature Flags and Dynamic Configuration Management
# Zero-downtime configuration updates with gradual rollouts and instant rollbacks

param(
    [Parameter(Mandatory=$false)]
    [string]$Action = "deploy",
    
    [Parameter(Mandatory=$false)]
    [string]$Environment = "production",
    
    [Parameter(Mandatory=$false)]
    [string]$FeatureName = "",
    
    [Parameter(Mandatory=$false)]
    [string]$ConfigFile = "",
    
    [Parameter(Mandatory=$false)]
    [int]$RolloutPercentage = 0,
    
    [Parameter(Mandatory=$false)]
    [switch]$Force,
    
    [Parameter(Mandatory=$false)]
    [switch]$DryRun
)

# Configuration
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ConfigDir = Join-Path $ScriptDir "..\configs"
$LogDir = Join-Path $ScriptDir "..\logs"
$DeploymentLog = Join-Path $LogDir "feature-flags-$(Get-Date -Format 'yyyyMMdd-HHmmss').log"

# Application endpoints
$AdminEndpoint = "http://localhost:8080/admin"
$ConfigEndpoint = "http://localhost:8080/config"
$FeatureFlagsEndpoint = "http://localhost:8080/features"
$MetricsEndpoint = "http://localhost:8080/metrics"

# Feature flag configuration
$SupportedFeatures = @(
    "advanced-order-types",
    "ml-price-prediction",
    "real-time-risk-assessment",
    "automated-market-making",
    "cross-chain-trading",
    "sentiment-analysis",
    "dynamic-fee-adjustment",
    "position-auto-close",
    "liquidity-aggregation",
    "social-trading"
)

# Configuration templates
$DefaultConfigs = @{
    "trading" = @{
        "max_order_size" = 1000000
        "min_order_size" = 0.001
        "order_timeout" = 300
        "price_precision" = 8
        "quantity_precision" = 6
        "trading_hours" = @{
            "enabled" = $true
            "start" = "00:00"
            "end" = "23:59"
            "timezone" = "UTC"
        }
        "risk_limits" = @{
            "max_position_size" = 10000000
            "max_daily_volume" = 100000000
            "stop_loss_threshold" = 0.05
        }
    }
    "features" = @{
        "advanced-order-types" = @{
            "enabled" = $false
            "rollout_percentage" = 0
            "user_groups" = @("beta-testers")
            "config" = @{
                "stop_loss_enabled" = $true
                "take_profit_enabled" = $true
                "trailing_stop_enabled" = $false
            }
        }
        "ml-price-prediction" = @{
            "enabled" = $false
            "rollout_percentage" = 0
            "user_groups" = @("premium-users")
            "config" = @{
                "model_version" = "v1.2.0"
                "prediction_horizon" = 300
                "confidence_threshold" = 0.8
            }
        }
    }
    "monitoring" = @{
        "metrics_collection" = $true
        "log_level" = "info"
        "trace_sampling_rate" = 0.1
        "alert_thresholds" = @{
            "error_rate" = 0.05
            "latency_p95" = 500
            "cpu_usage" = 0.8
            "memory_usage" = 0.85
        }
    }
}

# Logging functions
function Write-Log {
    param(
        [string]$Level,
        [string]$Message
    )
    
    $timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    $logEntry = "$timestamp [$Level] $Message"
    Write-Host $logEntry
    Add-Content -Path $DeploymentLog -Value $logEntry
}

function Write-Info { param([string]$Message) Write-Log "INFO" $Message }
function Write-Warn { param([string]$Message) Write-Log "WARN" $Message }
function Write-Error { param([string]$Message) Write-Log "ERROR" $Message }
function Write-Success { param([string]$Message) Write-Log "SUCCESS" $Message }

# Setup environment
function Initialize-Environment {
    Write-Info "Initializing feature flags and configuration management environment"
    
    # Create directories
    @($LogDir, $ConfigDir) | ForEach-Object {
        if (-not (Test-Path $_)) {
            New-Item -Path $_ -ItemType Directory -Force | Out-Null
        }
    }
    
    Write-Info "Deployment log: $DeploymentLog"
    Write-Success "Environment initialized successfully"
}

# Validate prerequisites
function Test-Prerequisites {
    Write-Info "Validating deployment prerequisites"
    
    # Check required PowerShell modules
    $requiredModules = @("PSYaml")
    foreach ($module in $requiredModules) {
        if (-not (Get-Module -ListAvailable -Name $module)) {
            Write-Warn "Installing missing module: $module"
            Install-Module -Name $module -Force -Scope CurrentUser
        }
    }
    
    # Test application connectivity
    try {
        $response = Invoke-RestMethod -Uri "$AdminEndpoint/health" -Method Get -TimeoutSec 10
        if ($response.status -eq "ok") {
            Write-Success "Application connectivity verified"
        } else {
            Write-Error "Application health check failed"
            return $false
        }
    }
    catch {
        Write-Error "Cannot connect to application: $_"
        return $false
    }
    
    Write-Success "Prerequisites validation completed"
    return $true
}

# Get current feature flag status
function Get-FeatureStatus {
    param([string]$FeatureName = "")
    
    try {
        if ($FeatureName) {
            $uri = "$FeatureFlagsEndpoint/$FeatureName"
        } else {
            $uri = "$FeatureFlagsEndpoint"
        }
        
        $response = Invoke-RestMethod -Uri $uri -Method Get
        return $response
    }
    catch {
        Write-Error "Failed to get feature status: $_"
        return $null
    }
}

# Get current configuration
function Get-CurrentConfig {
    param([string]$ConfigSection = "")
    
    try {
        if ($ConfigSection) {
            $uri = "$ConfigEndpoint/$ConfigSection"
        } else {
            $uri = "$ConfigEndpoint"
        }
        
        $response = Invoke-RestMethod -Uri $uri -Method Get
        return $response
    }
    catch {
        Write-Error "Failed to get current configuration: $_"
        return $null
    }
}

# Update feature flag
function Set-FeatureFlag {
    param(
        [string]$FeatureName,
        [bool]$Enabled,
        [int]$RolloutPercentage = 0,
        [string[]]$UserGroups = @(),
        [hashtable]$Config = @{}
    )
    
    Write-Info "Updating feature flag: $FeatureName"
    
    $featureConfig = @{
        "enabled" = $Enabled
        "rollout_percentage" = $RolloutPercentage
        "user_groups" = $UserGroups
        "config" = $Config
        "updated_at" = (Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ")
        "updated_by" = $env:USERNAME
    }
    
    if ($DryRun) {
        Write-Info "DRY RUN: Would update feature flag with:"
        $featureConfig | ConvertTo-Json -Depth 10 | Write-Host
        return $true
    }
    
    try {
        $body = $featureConfig | ConvertTo-Json -Depth 10
        $response = Invoke-RestMethod -Uri "$FeatureFlagsEndpoint/$FeatureName" -Method Put -Body $body -ContentType "application/json"
        
        Write-Success "Feature flag updated successfully: $FeatureName"
        return $true
    }
    catch {
        Write-Error "Failed to update feature flag: $_"
        return $false
    }
}

# Update configuration
function Set-Configuration {
    param(
        [string]$ConfigSection,
        [hashtable]$ConfigData
    )
    
    Write-Info "Updating configuration section: $ConfigSection"
    
    $configUpdate = @{
        "section" = $ConfigSection
        "data" = $ConfigData
        "updated_at" = (Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ")
        "updated_by" = $env:USERNAME
    }
    
    if ($DryRun) {
        Write-Info "DRY RUN: Would update configuration with:"
        $configUpdate | ConvertTo-Json -Depth 10 | Write-Host
        return $true
    }
    
    try {
        $body = $configUpdate | ConvertTo-Json -Depth 10
        $response = Invoke-RestMethod -Uri "$ConfigEndpoint/$ConfigSection" -Method Put -Body $body -ContentType "application/json"
        
        Write-Success "Configuration updated successfully: $ConfigSection"
        return $true
    }
    catch {
        Write-Error "Failed to update configuration: $_"
        return $false
    }
}

# Gradual rollout of feature flag
function Start-GradualRollout {
    param(
        [string]$FeatureName,
        [int]$TargetPercentage,
        [int]$StepSize = 10,
        [int]$StepDuration = 300  # 5 minutes
    )
    
    Write-Info "Starting gradual rollout for feature: $FeatureName"
    Write-Info "Target: $TargetPercentage%, Step Size: $StepSize%, Duration: $StepDuration seconds"
    
    $currentStatus = Get-FeatureStatus -FeatureName $FeatureName
    if (-not $currentStatus) {
        Write-Error "Cannot get current feature status"
        return $false
    }
    
    $currentPercentage = $currentStatus.rollout_percentage
    Write-Info "Current rollout percentage: $currentPercentage%"
    
    while ($currentPercentage -lt $TargetPercentage) {
        $nextPercentage = [Math]::Min($currentPercentage + $StepSize, $TargetPercentage)
        
        Write-Info "Rolling out to $nextPercentage%"
        
        if (-not (Set-FeatureFlag -FeatureName $FeatureName -Enabled $true -RolloutPercentage $nextPercentage)) {
            Write-Error "Failed to update rollout percentage"
            return $false
        }
        
        # Monitor metrics during rollout
        if (-not (Test-RolloutMetrics -FeatureName $FeatureName -Duration $StepDuration)) {
            Write-Error "Rollout metrics check failed - aborting rollout"
            Start-Rollback -FeatureName $FeatureName
            return $false
        }
        
        $currentPercentage = $nextPercentage
        
        if ($currentPercentage -lt $TargetPercentage) {
            Write-Info "Waiting $StepDuration seconds before next step..."
            Start-Sleep -Seconds $StepDuration
        }
    }
    
    Write-Success "Gradual rollout completed successfully for feature: $FeatureName"
    return $true
}

# Test rollout metrics
function Test-RolloutMetrics {
    param(
        [string]$FeatureName,
        [int]$Duration
    )
    
    Write-Info "Monitoring rollout metrics for feature: $FeatureName"
    
    $startTime = Get-Date
    $endTime = $startTime.AddSeconds($Duration)
    $checkInterval = 30
    
    while ((Get-Date) -lt $endTime) {
        try {
            # Get application metrics
            $metrics = Invoke-RestMethod -Uri "$MetricsEndpoint/features/$FeatureName" -Method Get
            
            $errorRate = $metrics.error_rate
            $latencyP95 = $metrics.latency_p95
            $cpuUsage = $metrics.cpu_usage
            
            Write-Info "Metrics - Error Rate: $errorRate%, Latency P95: $latencyP95ms, CPU: $cpuUsage%"
            
            # Check thresholds
            if ($errorRate -gt 5) {
                Write-Error "Error rate too high: $errorRate% > 5%"
                return $false
            }
            
            if ($latencyP95 -gt 1000) {
                Write-Error "Latency too high: $latencyP95ms > 1000ms"
                return $false
            }
            
            if ($cpuUsage -gt 85) {
                Write-Error "CPU usage too high: $cpuUsage% > 85%"
                return $false
            }
        }
        catch {
            Write-Warn "Failed to get metrics: $_"
        }
        
        Start-Sleep -Seconds $checkInterval
    }
    
    Write-Success "Rollout metrics check passed"
    return $true
}

# Rollback feature flag
function Start-Rollback {
    param([string]$FeatureName)
    
    Write-Warn "Rolling back feature flag: $FeatureName"
    
    # Get previous configuration from backup
    $backupFile = Join-Path $LogDir "feature-backup-$FeatureName.json"
    
    if (Test-Path $backupFile) {
        try {
            $backupConfig = Get-Content $backupFile | ConvertFrom-Json
            
            if (Set-FeatureFlag -FeatureName $FeatureName -Enabled $backupConfig.enabled -RolloutPercentage $backupConfig.rollout_percentage) {
                Write-Success "Feature flag rolled back successfully: $FeatureName"
                return $true
            }
        }
        catch {
            Write-Error "Failed to restore from backup: $_"
        }
    }
    
    # Fallback: disable feature
    Write-Warn "No backup found, disabling feature: $FeatureName"
    if (Set-FeatureFlag -FeatureName $FeatureName -Enabled $false -RolloutPercentage 0) {
        Write-Success "Feature flag disabled: $FeatureName"
        return $true
    }
    
    Write-Error "Failed to rollback feature flag: $FeatureName"
    return $false
}

# Backup current configuration
function Backup-Configuration {
    param([string]$FeatureName = "")
    
    Write-Info "Backing up current configuration"
    
    $timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
    
    if ($FeatureName) {
        $currentConfig = Get-FeatureStatus -FeatureName $FeatureName
        $backupFile = Join-Path $LogDir "feature-backup-$FeatureName-$timestamp.json"
        $currentConfig | ConvertTo-Json -Depth 10 | Set-Content $backupFile
        Write-Success "Feature backup saved: $backupFile"
    } else {
        $currentConfig = Get-CurrentConfig
        $backupFile = Join-Path $LogDir "config-backup-$timestamp.json"
        $currentConfig | ConvertTo-Json -Depth 10 | Set-Content $backupFile
        Write-Success "Full configuration backup saved: $backupFile"
    }
}

# Load configuration from file
function Import-ConfigurationFile {
    param([string]$FilePath)
    
    Write-Info "Loading configuration from file: $FilePath"
    
    if (-not (Test-Path $FilePath)) {
        Write-Error "Configuration file not found: $FilePath"
        return $false
    }
    
    try {
        $extension = [System.IO.Path]::GetExtension($FilePath).ToLower()
        
        switch ($extension) {
            ".json" {
                $config = Get-Content $FilePath | ConvertFrom-Json -AsHashtable
            }
            ".yaml" -or ".yml" {
                $config = Get-Content $FilePath | ConvertFrom-Yaml
            }
            default {
                Write-Error "Unsupported file format: $extension"
                return $false
            }
        }
        
        # Apply configuration
        foreach ($section in $config.Keys) {
            if ($section -eq "features") {
                foreach ($feature in $config.features.Keys) {
                    $featureConfig = $config.features[$feature]
                    Set-FeatureFlag -FeatureName $feature -Enabled $featureConfig.enabled -RolloutPercentage $featureConfig.rollout_percentage -Config $featureConfig.config
                }
            } else {
                Set-Configuration -ConfigSection $section -ConfigData $config[$section]
            }
        }
        
        Write-Success "Configuration applied successfully from: $FilePath"
        return $true
    }
    catch {
        Write-Error "Failed to load configuration file: $_"
        return $false
    }
}

# Display current status
function Show-Status {
    Write-Info "Current Feature Flags and Configuration Status"
    Write-Host "================================================" -ForegroundColor Cyan
    
    # Feature flags status
    Write-Host "`nFeature Flags:" -ForegroundColor Yellow
    $features = Get-FeatureStatus
    if ($features) {
        foreach ($feature in $features.PSObject.Properties) {
            $status = if ($feature.Value.enabled) { "ENABLED" } else { "DISABLED" }
            $percentage = $feature.Value.rollout_percentage
            Write-Host "  $($feature.Name): $status ($percentage%)" -ForegroundColor $(if ($feature.Value.enabled) { "Green" } else { "Red" })
        }
    }
    
    # Configuration status
    Write-Host "`nConfiguration Sections:" -ForegroundColor Yellow
    $config = Get-CurrentConfig
    if ($config) {
        foreach ($section in $config.PSObject.Properties) {
            Write-Host "  $($section.Name): Loaded" -ForegroundColor Green
        }
    }
    
    Write-Host "`n================================================" -ForegroundColor Cyan
}

# Export configuration
function Export-Configuration {
    param(
        [string]$OutputPath,
        [string]$Format = "json"
    )
    
    Write-Info "Exporting current configuration"
    
    $config = @{
        "features" = Get-FeatureStatus
        "trading" = Get-CurrentConfig -ConfigSection "trading"
        "monitoring" = Get-CurrentConfig -ConfigSection "monitoring"
        "exported_at" = (Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ")
        "exported_by" = $env:USERNAME
    }
    
    try {
        switch ($Format.ToLower()) {
            "json" {
                $config | ConvertTo-Json -Depth 10 | Set-Content $OutputPath
            }
            "yaml" {
                $config | ConvertTo-Yaml | Set-Content $OutputPath
            }
            default {
                Write-Error "Unsupported export format: $Format"
                return $false
            }
        }
        
        Write-Success "Configuration exported to: $OutputPath"
        return $true
    }
    catch {
        Write-Error "Failed to export configuration: $_"
        return $false
    }
}

# A/B testing setup
function Start-ABTest {
    param(
        [string]$FeatureName,
        [int]$TestPercentage = 50,
        [int]$Duration = 3600,  # 1 hour
        [hashtable]$VariantConfig = @{}
    )
    
    Write-Info "Starting A/B test for feature: $FeatureName"
    Write-Info "Test percentage: $TestPercentage%, Duration: $Duration seconds"
    
    # Backup current state
    Backup-Configuration -FeatureName $FeatureName
    
    # Enable feature with test percentage
    if (-not (Set-FeatureFlag -FeatureName $FeatureName -Enabled $true -RolloutPercentage $TestPercentage -Config $VariantConfig)) {
        Write-Error "Failed to start A/B test"
        return $false
    }
    
    Write-Info "A/B test started. Monitoring for $Duration seconds..."
    
    # Monitor test metrics
    $testResult = Test-RolloutMetrics -FeatureName $FeatureName -Duration $Duration
    
    if ($testResult) {
        Write-Success "A/B test completed successfully for feature: $FeatureName"
        Write-Info "Consider promoting feature to full rollout"
    } else {
        Write-Error "A/B test failed for feature: $FeatureName"
        Start-Rollback -FeatureName $FeatureName
    }
    
    return $testResult
}

# Main execution logic
function Invoke-Main {
    Initialize-Environment
    
    if (-not (Test-Prerequisites)) {
        Write-Error "Prerequisites validation failed"
        exit 1
    }
    
    switch ($Action.ToLower()) {
        "deploy" {
            if ($ConfigFile) {
                Import-ConfigurationFile -FilePath $ConfigFile
            } elseif ($FeatureName) {
                if ($RolloutPercentage -gt 0) {
                    Start-GradualRollout -FeatureName $FeatureName -TargetPercentage $RolloutPercentage
                } else {
                    Set-FeatureFlag -FeatureName $FeatureName -Enabled $true
                }
            } else {
                Write-Error "Either ConfigFile or FeatureName must be specified for deploy action"
                exit 1
            }
        }
        
        "rollback" {
            if (-not $FeatureName) {
                Write-Error "FeatureName must be specified for rollback action"
                exit 1
            }
            Start-Rollback -FeatureName $FeatureName
        }
        
        "status" {
            Show-Status
        }
        
        "export" {
            $outputPath = if ($ConfigFile) { $ConfigFile } else { "config-export-$(Get-Date -Format 'yyyyMMdd-HHmmss').json" }
            Export-Configuration -OutputPath $outputPath
        }
        
        "test" {
            if (-not $FeatureName) {
                Write-Error "FeatureName must be specified for test action"
                exit 1
            }
            Start-ABTest -FeatureName $FeatureName -TestPercentage $RolloutPercentage
        }
        
        "backup" {
            Backup-Configuration -FeatureName $FeatureName
        }
        
        default {
            Write-Host @"
Usage: .\feature-flags-v2.ps1 [OPTIONS]

Actions:
  deploy    Deploy feature flags or configuration
  rollback  Rollback feature flag to previous state
  status    Show current feature flags and configuration status
  export    Export current configuration to file
  test      Start A/B test for feature
  backup    Backup current configuration

Options:
  -FeatureName <string>        Name of the feature flag
  -ConfigFile <string>         Path to configuration file
  -RolloutPercentage <int>     Percentage for gradual rollout
  -Environment <string>        Target environment (default: production)
  -Force                       Force deployment without confirmations
  -DryRun                      Show what would be done without executing

Examples:
  .\feature-flags-v2.ps1 -Action deploy -FeatureName "ml-price-prediction" -RolloutPercentage 25
  .\feature-flags-v2.ps1 -Action test -FeatureName "advanced-order-types" -RolloutPercentage 50
  .\feature-flags-v2.ps1 -Action export -ConfigFile "current-config.json"
  .\feature-flags-v2.ps1 -Action status
  .\feature-flags-v2.ps1 -Action rollback -FeatureName "social-trading"
"@
            exit 1
        }
    }
}

# Execute main function
try {
    Invoke-Main
    Write-Success "Feature flags deployment completed successfully"
}
catch {
    Write-Error "Feature flags deployment failed: $_"
    exit 1
}
