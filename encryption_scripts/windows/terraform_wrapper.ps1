param(
    [Parameter(Mandatory = $true, Position = 0)]
    [ValidateSet("init", "apply", "destroy")]
    [string]$Operation,

    [Parameter(Position = 1, ValueFromRemainingArguments = $true)]
    [string[]]$TerraformArgs
)

$ErrorActionPreference = "Stop"
$stateFile = "terraform.tfstate"
$stateBackupFile = "terraform.tfstate.backup"
$lockDirectory = "lockfile.lock"

function Test-TerraformCLIArgsContainOption {
    param(
        [AllowEmptyString()][string]$Arguments,
        [Parameter(Mandatory = $true)][string]$OptionPattern
    )

    $token = ""
    $quote = 0
    $escaped = $false
    $tokenStarted = $false
    foreach ($character in $Arguments.ToCharArray()) {
        if ($escaped) {
            $token += $character
            $tokenStarted = $true
            $escaped = $false
        }
        elseif ($quote -eq 39) {
            if ([int]$character -eq 39) {
                $quote = 0
            }
            else {
                $token += $character
            }
            $tokenStarted = $true
        }
        elseif ($quote -eq 34) {
            if ([int]$character -eq 34) {
                $quote = 0
            }
            elseif ([int]$character -eq 92) {
                $escaped = $true
            }
            else {
                $token += $character
            }
            $tokenStarted = $true
        }
        elseif ([int]$character -eq 39 -or [int]$character -eq 34) {
            $quote = [int]$character
            $tokenStarted = $true
        }
        elseif ([int]$character -eq 92) {
            $escaped = $true
            $tokenStarted = $true
        }
        elseif ([char]::IsWhiteSpace($character)) {
            if ($tokenStarted) {
                if ($token -cmatch "^--?$OptionPattern(=|$)") {
                    return $true
                }
                $token = ""
                $tokenStarted = $false
            }
        }
        else {
            $token += $character
            $tokenStarted = $true
        }
    }
    if ($quote -ne 0 -or $escaped) {
        throw "Terraform CLI arguments contain incomplete quoting or escaping."
    }
    return $tokenStarted -and $token -cmatch "^--?$OptionPattern(=|$)"
}

$statePassphrase = $env:TFSTATE_PASSPHRASE
if ([string]::IsNullOrEmpty($statePassphrase)) {
    if (-not [string]::IsNullOrEmpty($env:TF_VAR_tss_username) -and -not [string]::IsNullOrEmpty($env:TF_VAR_tss_password)) {
        $statePassphrase = "$($env:TF_VAR_tss_username)$($env:TF_VAR_tss_password)"
    }
}
Remove-Item Env:TFSTATE_PASSPHRASE -ErrorAction SilentlyContinue
if ([string]::IsNullOrEmpty($statePassphrase)) {
    [Console]::Error.WriteLine("Set TFSTATE_PASSPHRASE, or TF_VAR_tss_username and TF_VAR_tss_password, in the environment")
    exit 1
}

foreach ($argument in $TerraformArgs) {
    if ($argument -match '^--?from-module(=|$)') {
        [Console]::Error.WriteLine("The encrypted-state init wrapper does not support -from-module because Terraform requires an empty target directory, while the wrapper and its state lock occupy this directory. Run plain terraform init -from-module=... in an empty target first, then copy the wrappers into that module and run this wrapper normally.")
        exit 1
    }
    if ($Operation -eq "init" -and $argument -match '^--?backend-config(=|$)') {
        [Console]::Error.WriteLine("The encrypted-state init wrapper does not support -backend-config because it could redirect state outside the paths this wrapper encrypts. Put the default local backend configuration in the root module instead.")
        exit 1
    }
    if ($Operation -ne "init" -and ($argument -match '^--?(state|state-out|backup)(=|$)')) {
        [Console]::Error.WriteLine("The encrypted-state wrapper does not support -state, -state-out, or -backup because it encrypts only the default local state paths.")
        exit 1
    }
}

$commonCLIArgs = $env:TF_CLI_ARGS
$operationCLIArgs = [Environment]::GetEnvironmentVariable("TF_CLI_ARGS_$Operation")
try {
    if ($Operation -eq "init" -and ((Test-TerraformCLIArgsContainOption -Arguments $commonCLIArgs -OptionPattern '(backend-config|from-module)') -or (Test-TerraformCLIArgsContainOption -Arguments $operationCLIArgs -OptionPattern '(backend-config|from-module)'))) {
        [Console]::Error.WriteLine("TF_CLI_ARGS or TF_CLI_ARGS_init supplies -backend-config or -from-module, which this encrypted-state wrapper does not support; remove it.")
        exit 1
    }
    if ($Operation -ne "init" -and ((Test-TerraformCLIArgsContainOption -Arguments $commonCLIArgs -OptionPattern '(state|state-out|backup)') -or (Test-TerraformCLIArgsContainOption -Arguments $operationCLIArgs -OptionPattern '(state|state-out|backup)'))) {
        [Console]::Error.WriteLine("TF_CLI_ARGS redirects state outside the paths this encrypted-state wrapper protects; remove -state, -state-out, and -backup.")
        exit 1
    }
}
catch {
    [Console]::Error.WriteLine("TF_CLI_ARGS contains incomplete quoting or escaping; correct it before using the encrypted-state wrapper.")
    exit 1
}

$script:providerDiscoveryError = $null

function Find-ProviderBinary {
    $script:providerDiscoveryError = $null
    $providerRoot = Join-Path (Get-Location) ".terraform/providers/registry.terraform.io/delineaxpm/tss"
    if (-not (Test-Path -LiteralPath $providerRoot -PathType Container)) {
        return $null
    }

    try {
        $terraformVersion = (& terraform version -json 2>$null | Out-String | ConvertFrom-Json)
        if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrEmpty($terraformVersion.platform) -or $terraformVersion.platform -notmatch '^[A-Za-z0-9_]+$') {
            throw "invalid platform"
        }
        $currentPlatform = $terraformVersion.platform
    }
    catch {
        $script:providerDiscoveryError = "Could not determine the current Terraform platform; run terraform version -json and repair the Terraform installation."
        return $null
    }

    $searchRoot = $providerRoot
    $lockedVersion = $null
    $lockPath = Join-Path (Get-Location) ".terraform.lock.hcl"
    if (Test-Path -LiteralPath $lockPath -PathType Leaf) {
        $inProvider = $false
        foreach ($line in Get-Content -LiteralPath $lockPath) {
            if (-not $inProvider -and $line -match '^\s*provider\s+"registry\.terraform\.io/delineaxpm/tss"\s*\{') {
                $inProvider = $true
                continue
            }
            if ($inProvider -and $line -match '^\s*version\s*=\s*"([^"]+)"') {
                $lockedVersion = $Matches[1]
                if ($lockedVersion -notmatch '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?(\+[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$') {
                    $script:providerDiscoveryError = "The tss provider version in .terraform.lock.hcl is invalid; run terraform init to repair the lock file."
                    return $null
                }
                $searchRoot = Join-Path $providerRoot $lockedVersion
                break
            }
            if ($inProvider -and $line -match '^\s*\}') {
                break
            }
        }
    }
    if (-not (Test-Path -LiteralPath $searchRoot -PathType Container)) {
        return $null
    }

    if ([string]::IsNullOrEmpty($lockedVersion)) {
        $providerPattern = Join-Path $searchRoot "*\$currentPlatform\terraform-provider-tss*.exe"
    }
    else {
        $providerPattern = Join-Path $searchRoot "$currentPlatform\terraform-provider-tss*.exe"
    }
    $providers = @(Get-ChildItem -Path $providerPattern -File -ErrorAction SilentlyContinue)
    if ($providers.Count -eq 0) {
        return $null
    }
    if ($providers.Count -gt 1) {
        if ([string]::IsNullOrEmpty($lockedVersion)) {
            $script:providerDiscoveryError = "Multiple installed tss provider versions were found and .terraform.lock.hcl does not select one; run terraform init to create or repair the lock file."
        }
        else {
            $script:providerDiscoveryError = "Multiple tss provider binaries match version $lockedVersion selected by .terraform.lock.hcl; remove the duplicate platform installation and run terraform init again."
        }
        return $null
    }
    return $providers[0].FullName
}

function Get-ProviderBootstrapArgs {
    param([string[]]$Arguments)

    $result = @()
    $takesValue = $false
    foreach ($argument in $Arguments) {
        if ($takesValue) {
            $result += $argument
            $takesValue = $false
            continue
        }
        switch -Regex ($argument) {
            '^--?(plugin-dir|lockfile|get|input)$' {
                $result += $argument
                $takesValue = $true
                continue
            }
            '^--?(plugin-dir|lockfile|get|input)=' {
                $result += $argument
                continue
            }
            '^--?upgrade(=.*)?$' {
                $result += $argument
                continue
            }
            '^--?(no-color|json)$' {
                $result += $argument
                continue
            }
        }
    }
    return $result
}

function Test-StateHelperCapability {
    param([Parameter(Mandatory = $true)][string]$Path)

    try {
        $output = & $Path state-helper-version 2>$null
        return $LASTEXITCODE -eq 0 -and (($output -join "`n").Trim() -eq "1")
    }
    catch {
        return $false
    }
}

function Assert-StateLayoutSupported {
    param([Parameter(Mandatory = $true)][string]$Path)

    & $Path state-helper-check-layout
    if ($LASTEXITCODE -ne 0) {
        throw "The Terraform state layout is not supported by this encrypted-state wrapper. No state was decrypted."
    }
}

function Invoke-StateHelper {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][ValidateSet("encrypt", "decrypt")][string]$Action,
        [Parameter(Mandatory = $true)][string]$Target
    )

    try {
        $env:TFSTATE_PASSPHRASE = $statePassphrase
        & $Path $Action $Target
        return $LASTEXITCODE
    }
    finally {
        Remove-Item Env:TFSTATE_PASSPHRASE -ErrorAction SilentlyContinue
    }
}

try {
    $null = New-Item -ItemType Directory -Path $lockDirectory -ErrorAction Stop
}
catch {
    [Console]::Error.WriteLine("State is currently locked. Another operation might be in progress.")
    exit 1
}

$stateDecrypted = $false
$backupDecrypted = $false
$bootstrapStatePath = $null
$bootstrapOutputPath = $null
$helperPath = $null
$exitCode = 1
$stateCleanupFailed = $false

try {
    $providerPath = Find-ProviderBinary
    if ($Operation -eq "init" -and ([string]::IsNullOrEmpty($providerPath) -or -not (Test-StateHelperCapability -Path $providerPath))) {
        Write-Host "A compatible tss state helper was not found. Fetching the provider using a backend-disabled terraform init"
        $bootstrapCollision = $false
        if (Test-Path -LiteralPath $stateFile) {
            $bootstrapStatePath = "$stateFile.encrypted-bootstrap.$PID.$([guid]::NewGuid().ToString('N'))"
            Move-Item -LiteralPath $stateFile -Destination $bootstrapStatePath -ErrorAction Stop
        }
        try {
            $bootstrapArgs = @(Get-ProviderBootstrapArgs -Arguments $TerraformArgs)
            & terraform init -backend=false @bootstrapArgs
            if ($LASTEXITCODE -ne 0) {
                throw "Terraform provider bootstrap failed with exit code $LASTEXITCODE."
            }
        }
        finally {
            if (-not [string]::IsNullOrEmpty($bootstrapStatePath) -and (Test-Path -LiteralPath $bootstrapStatePath)) {
                if (Test-Path -LiteralPath $stateFile) {
                    $bootstrapOutputPath = "$stateFile.bootstrap-output.$PID.$([guid]::NewGuid().ToString('N'))"
                    Move-Item -LiteralPath $stateFile -Destination $bootstrapOutputPath -ErrorAction Stop
                    $bootstrapCollision = $true
                }
                Move-Item -LiteralPath $bootstrapStatePath -Destination $stateFile -ErrorAction Stop
                $bootstrapStatePath = $null
            }
        }
        if ($bootstrapCollision) {
            $stateCleanupFailed = $true
            throw "Terraform bootstrap unexpectedly created $stateFile; it was preserved as $bootstrapOutputPath."
        }
        $providerPath = Find-ProviderBinary
        if ([string]::IsNullOrEmpty($providerPath) -or -not (Test-StateHelperCapability -Path $providerPath)) {
            throw "terraform init did not install a tss provider with state-helper protocol 1; require provider v5 or later and check required_providers and the dependency lock file."
        }
    }
    if ([string]::IsNullOrEmpty($providerPath)) {
        if (-not [string]::IsNullOrEmpty($script:providerDiscoveryError)) {
            throw $script:providerDiscoveryError
        }
        throw "Terraform plugin path not found. Initialize Terraform first."
    }
    if (-not (Test-StateHelperCapability -Path $providerPath)) {
        throw "The installed tss provider does not support state-helper protocol 1. Install provider v5 or later with terraform init before using this wrapper."
    }

    $helperPath = Join-Path ([IO.Path]::GetTempPath()) "terraform-provider-tss-helper-$PID-$([guid]::NewGuid().ToString('N')).exe"
    Copy-Item -LiteralPath $providerPath -Destination $helperPath -ErrorAction Stop
    if (-not (Test-StateHelperCapability -Path $helperPath)) {
        throw "The copied tss provider does not support state-helper protocol 1; require provider v5 or later."
    }
    Assert-StateLayoutSupported -Path $helperPath

    Write-Host "Decrypting state file..."
    $helperExitCode = Invoke-StateHelper -Path $helperPath -Action decrypt -Target $stateFile
    if ($helperExitCode -ne 0) {
        throw "Failed to decrypt state file."
    }
    $stateDecrypted = $true

    Write-Host "Decrypting state backup file..."
    $helperExitCode = Invoke-StateHelper -Path $helperPath -Action decrypt -Target $stateBackupFile
    if ($helperExitCode -ne 0) {
        $stateCleanupFailed = $true
        throw "Failed to decrypt state backup file."
    }
    $backupDecrypted = $true

    & terraform $Operation @TerraformArgs
    $exitCode = $LASTEXITCODE
    if ($exitCode -ne 0) {
        [Console]::Error.WriteLine("Terraform $Operation failed.")
    }
}
catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    $exitCode = 1
}
finally {
    try {
        if (-not [string]::IsNullOrEmpty($bootstrapStatePath) -and (Test-Path -LiteralPath $bootstrapStatePath)) {
            if (Test-Path -LiteralPath $stateFile) {
                $bootstrapOutputPath = "$stateFile.bootstrap-output.$PID.$([guid]::NewGuid().ToString('N'))"
                Move-Item -LiteralPath $stateFile -Destination $bootstrapOutputPath -ErrorAction Stop
                [Console]::Error.WriteLine("Terraform bootstrap unexpectedly created $stateFile; it was preserved as $bootstrapOutputPath.")
                $exitCode = 1
                $stateCleanupFailed = $true
            }
            Move-Item -LiteralPath $bootstrapStatePath -Destination $stateFile -ErrorAction Stop
            $bootstrapStatePath = $null
        }
    }
    catch {
        [Console]::Error.WriteLine("Failed to restore encrypted state from ${bootstrapStatePath}: $($_.Exception.Message)")
        $exitCode = 1
        $stateCleanupFailed = $true
    }
    if ($stateDecrypted) {
        try {
            Write-Host "Encrypting state file..."
            $helperExitCode = Invoke-StateHelper -Path $helperPath -Action encrypt -Target $stateFile
            if ($helperExitCode -ne 0) {
                throw "Encryption helper exited with code $helperExitCode."
            }
        }
        catch {
            [Console]::Error.WriteLine("Failed to encrypt state file: $($_.Exception.Message)")
            $exitCode = 1
            $stateCleanupFailed = $true
        }
    }
    if ($backupDecrypted) {
        try {
            Write-Host "Encrypting state backup file..."
            $helperExitCode = Invoke-StateHelper -Path $helperPath -Action encrypt -Target $stateBackupFile
            if ($helperExitCode -ne 0) {
                throw "Encryption helper exited with code $helperExitCode."
            }
        }
        catch {
            [Console]::Error.WriteLine("Failed to encrypt state backup file: $($_.Exception.Message)")
            $exitCode = 1
            $stateCleanupFailed = $true
        }
    }
    if ($stateCleanupFailed) {
        [Console]::Error.WriteLine("State cleanup is incomplete; retaining $lockDirectory to prevent another wrapper run until the state files are recovered.")
        if (-not [string]::IsNullOrEmpty($helperPath)) {
            [Console]::Error.WriteLine("The stable encryption helper was retained at $helperPath for recovery.")
        }
    }
    else {
        try {
            if (-not [string]::IsNullOrEmpty($helperPath)) {
                Remove-Item -LiteralPath $helperPath -Force -ErrorAction Stop
            }
        }
        catch {
            [Console]::Error.WriteLine("Failed to remove temporary encryption helper: $($_.Exception.Message)")
            $exitCode = 1
        }
        try {
            Remove-Item -LiteralPath $lockDirectory -Force -ErrorAction Stop
        }
        catch {
            [Console]::Error.WriteLine("Failed to remove state lock directory: $($_.Exception.Message)")
            $exitCode = 1
        }
    }
}

if ($exitCode -eq 0) {
    Write-Host "Operation completed successfully."
}
exit $exitCode
