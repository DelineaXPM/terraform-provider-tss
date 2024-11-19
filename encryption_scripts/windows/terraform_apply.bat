@echo off
setlocal

REM Set paths for the encrypted and decrypted state files
set STATE_FILE=terraform.tfstate
set STATE_BACKUP_FILE=terraform.tfstate.backup
set LOCK_FILE=lockfile.lock
set TFSTATE_PASSPHRASE=%TF_VAR_tss_username%%TF_VAR_tss_password%

if "%TFSTATE_PASSPHRASE%"=="" (
    echo Username and Password are not set in environment variable
	exit /b 1
)

for /r %%i in (terraform-provider-tss*.exe) do @if exist "%%i" set "TF_PLUGIN_PATH=%%~fi"

if "%TF_PLUGIN_PATH%"=="" (
    echo Terraform plugin path not found. Initialize terraform first.
	exit /b 1
)

REM Check if lock file exists (ensure no parallel operations)
if exist "%LOCK_FILE%" (
    echo State is currently locked. Another operation might be in progress.
    exit /b 1
)

REM Create lock file
echo Locked > "%LOCK_FILE%"

REM Decrypt state file before running Terraform
echo Decrypting state file...
"%TF_PLUGIN_PATH%" "decrypt" "%STATE_FILE%"

if %ERRORLEVEL% neq 0 (
    echo Failed to decrypt state file. Exiting.
    del /F "%LOCK_FILE%"
    exit /b 1
)

REM Decrypt state backup file before running Terraform
echo Decrypting state backup file...
"%TF_PLUGIN_PATH%" "decrypt" "%STATE_BACKUP_FILE%"

if %ERRORLEVEL% neq 0 (
    echo Failed to decrypt state backup file. Exiting.
    del /F "%LOCK_FILE%"
    exit /b 1
)


REM Run Terraform apply with the decrypted state
terraform apply %*

REM Check if Terraform apply succeeded
if %ERRORLEVEL% neq 0 (
    echo Terraform apply failed.
)

REM Encrypt the state file after Terraform apply
echo Encrypting state file...
"%TF_PLUGIN_PATH%" "encrypt" "%STATE_FILE%"

if %ERRORLEVEL% neq 0 (
    echo Failed to encrypt state file. Exiting.
    del /F "%LOCK_FILE%"
    exit /b 1
)

REM Encrypt the state backup file after Terraform apply
echo Encrypting state backup file...
"%TF_PLUGIN_PATH%" "encrypt" "%STATE_BACKUP_FILE%"

if %ERRORLEVEL% neq 0 (
    echo Failed to encrypt state backup file. Exiting.
    del /F "%LOCK_FILE%"
    exit /b 1
)

REM Remove decrypted state file and lock file
del /F "%LOCK_FILE%"

echo Operation completed successfully.
endlocal