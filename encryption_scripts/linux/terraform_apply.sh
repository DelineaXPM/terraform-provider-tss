#!/bin/bash

# Set paths for the encrypted and decrypted state files
export STATE_FILE="terraform.tfstate"
export STATE_BACKUP_FILE="terraform.tfstate.backup"
export LOCK_FILE="lockfile.lock"
export TFSTATE_PASSPHRASE="${TF_VAR_tss_username}${TF_VAR_tss_password}"

# Check if TFSTATE_PASSPHRASE is set
if [ -z "$TFSTATE_PASSPHRASE" ]; then
    echo "Username and Password are not set in environment variable"
    exit 1
fi

# Find the Terraform plugin path
TF_PLUGIN_PATH=$(find . -type f -name 'terraform-provider-tss*' -print | grep -E '^.*terraform-provider-tss$' | head -n 1) #".terraform/providers/terraform.delinea.com/delinea/tss/2.0.7/linux_amd64/terraform-provider-tss"

if [ -z "$TF_PLUGIN_PATH" ] || [ ! -f "$TF_PLUGIN_PATH" ]; then
    echo "Terraform plugin path not found. Initialize terraform first."
    exit 1
fi

# Check if lock file exists (ensure no parallel operations)
if [ -e "$LOCK_FILE" ]; then
    echo "State is currently locked. Another operation might be in progress."
    exit 1
fi

# Create lock file
echo "Locked" > "$LOCK_FILE"

# Decrypt state file before running Terraform
echo "Decrypting state file..."
"$TF_PLUGIN_PATH" decrypt "$STATE_FILE"

if [ $? -ne 0 ]; then
    echo "Failed to decrypt state file. Exiting."
    rm -f "$LOCK_FILE"
    exit 1
fi

# Decrypt state backup file before running Terraform
echo "Decrypting state backup file..."
"$TF_PLUGIN_PATH" decrypt "$STATE_BACKUP_FILE"

if [ $? -ne 0 ]; then
    echo "Failed to decrypt state backup file. Exiting."
    rm -f "$LOCK_FILE"
    exit 1
fi

# Run Terraform apply with the decrypted state
terraform apply "$@"

# Check if Terraform apply succeeded
if [ $? -ne 0 ]; then
    echo "Terraform apply failed."
fi

# Encrypt the state file after Terraform apply
echo "Encrypting state file..."
"$TF_PLUGIN_PATH" encrypt "$STATE_FILE"

if [ $? -ne 0 ]; then
    echo "Failed to encrypt state file. Exiting."
    rm -f "$LOCK_FILE"
    exit 1
fi

# Encrypt the state backup file after Terraform apply
echo "Encrypting state backup file..."
"$TF_PLUGIN_PATH" encrypt "$STATE_BACKUP_FILE"

if [ $? -ne 0 ]; then
    echo "Failed to encrypt state backup file. Exiting."
    rm -f "$LOCK_FILE"
    exit 1
fi

# Remove lock file
rm -f "$LOCK_FILE"

echo "Operation completed successfully."