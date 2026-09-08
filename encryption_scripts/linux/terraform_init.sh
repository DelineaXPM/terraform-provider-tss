#!/bin/bash

# Set paths for the encrypted and decrypted state files
export STATE_FILE="terraform.tfstate"
export STATE_BACKUP_FILE="terraform.tfstate.backup"
export LOCK_DIR="lockfile.lock"
# Prefer a dedicated passphrase; fall back to the legacy username+password
# derivation so existing encrypted state files keep decrypting.
if [ -n "${TFSTATE_PASSPHRASE:-}" ]; then
    state_passphrase_value="$TFSTATE_PASSPHRASE"
elif [ -n "${TF_VAR_tss_username:-}" ] && [ -n "${TF_VAR_tss_password:-}" ]; then
    state_passphrase_value="${TF_VAR_tss_username}${TF_VAR_tss_password}"
else
    state_passphrase_value=""
fi
unset TFSTATE_PASSPHRASE STATE_PASSPHRASE
STATE_PASSPHRASE="$state_passphrase_value"
unset state_passphrase_value

# Check if TFSTATE_PASSPHRASE is set
if [ -z "$STATE_PASSPHRASE" ]; then
    echo "Set TFSTATE_PASSPHRASE, or TF_VAR_tss_username and TF_VAR_tss_password, in the environment"
    exit 1
fi

for argument in "$@"; do
    case "$argument" in
        -from-module|--from-module|-from-module=*|--from-module=*)
            echo "The encrypted-state init wrapper does not support -from-module because Terraform requires an empty target directory, while the wrapper and its state lock occupy this directory. Run plain terraform init -from-module=... in an empty target first, then copy the wrappers into that module and run this wrapper normally." >&2
            exit 1
            ;;
        -backend-config|--backend-config|-backend-config=*|--backend-config=*)
            echo "The encrypted-state init wrapper does not support -backend-config because it could redirect state outside the paths this wrapper encrypts. Put the default local backend configuration in the root module instead." >&2
            exit 1
            ;;
    esac
done
terraform_cli_args_contain_backend_option() {
    local input="$1"
    local token=""
    local quote=""
    local escaped=0
    local token_started=0
    local character
    local index

    for ((index = 0; index < ${#input}; index++)); do
        character="${input:index:1}"
        if [ "$escaped" -eq 1 ]; then
            token+="$character"
            token_started=1
            escaped=0
        elif [ "$quote" = "'" ]; then
            if [ "$character" = "'" ]; then
                quote=""
            else
                token+="$character"
            fi
            token_started=1
        elif [ "$quote" = '"' ]; then
            if [ "$character" = '"' ]; then
                quote=""
            elif [ "$character" = "\\" ]; then
                escaped=1
            else
                token+="$character"
            fi
            token_started=1
        else
            case "$character" in
                "'"|'"') quote="$character"; token_started=1 ;;
                "\\") escaped=1; token_started=1 ;;
                [[:space:]])
                    if [ "$token_started" -eq 1 ]; then
                        if [[ "$token" =~ ^--?(backend-config|from-module)(=|$) ]]; then
                            return 0
                        fi
                        token=""
                        token_started=0
                    fi
                    ;;
                *) token+="$character"; token_started=1 ;;
            esac
        fi
    done
    if [ -n "$quote" ] || [ "$escaped" -eq 1 ]; then
        return 2
    fi
    if [ "$token_started" -eq 1 ] && [[ "$token" =~ ^--?(backend-config|from-module)(=|$) ]]; then
        return 0
    fi
    return 1
}

terraform_cli_args_contain_backend_option "${TF_CLI_ARGS:-}"
COMMON_ARGS_STATUS=$?
terraform_cli_args_contain_backend_option "${TF_CLI_ARGS_init:-}"
OPERATION_ARGS_STATUS=$?
if [ "$COMMON_ARGS_STATUS" -eq 0 ] || [ "$OPERATION_ARGS_STATUS" -eq 0 ]; then
    echo "TF_CLI_ARGS or TF_CLI_ARGS_init supplies -backend-config or -from-module, which this encrypted-state wrapper does not support; remove it." >&2
    exit 1
fi
if [ "$COMMON_ARGS_STATUS" -eq 2 ] || [ "$OPERATION_ARGS_STATUS" -eq 2 ]; then
    echo "TF_CLI_ARGS or TF_CLI_ARGS_init contains incomplete quoting or escaping; correct it before using the encrypted-state wrapper." >&2
    exit 1
fi
unset COMMON_ARGS_STATUS OPERATION_ARGS_STATUS

if ! mkdir "$LOCK_DIR" 2>/dev/null; then
    echo "State is currently locked. Another operation might be in progress."
    exit 1
fi

STATE_DECRYPTED=0
BACKUP_DECRYPTED=0
BOOTSTRAP_STATE_PATH=""
TF_HELPER_PATH=""
CLEANUP_INCOMPLETE=0

find_provider_binary() {
    local provider_root=".terraform/providers/registry.terraform.io/delineaxpm/tss"
    local search_root="$provider_root"
    local locked_version=""
    local current_platform=""
    local candidate
    local provider_path=""

    record_provider_candidate() {
        candidate="$1"
        if [ -f "$candidate" ] && [ -x "$candidate" ]; then
            if [ -n "$provider_path" ]; then
                if [ -n "$locked_version" ]; then
                    echo "Multiple tss provider binaries match version $locked_version selected by .terraform.lock.hcl; remove the duplicate platform installation and run terraform init again." >&2
                else
                    echo "Multiple installed tss provider versions were found and .terraform.lock.hcl does not select one; run terraform init to create or repair the lock file." >&2
                fi
                return 2
            fi
            provider_path="$candidate"
        fi
    }

    current_platform=$(terraform version -json 2>/dev/null | awk -F'"' '/"platform"/ { print $4; exit }')
    case "$current_platform" in
        ""|*[!A-Za-z0-9_]*)
            echo "Could not determine the current Terraform platform; run terraform version -json and repair the Terraform installation." >&2
            return 1
            ;;
    esac
    if [ -f ".terraform.lock.hcl" ]; then
        locked_version=$(awk '
            $1 == "provider" && tolower($2) == "\"registry.terraform.io/delineaxpm/tss\"" && $3 == "{" { in_provider = 1; next }
            in_provider && $1 == "}" { exit }
            in_provider && $1 == "version" && $2 == "=" { gsub(/\"/, "", $3); print $3; exit }
        ' .terraform.lock.hcl)
    fi
    if [ -n "$locked_version" ] && [[ ! "$locked_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?(\+[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$ ]]; then
        echo "The tss provider version in .terraform.lock.hcl is invalid; run terraform init to repair the lock file." >&2
        return 1
    fi
    if [ -n "$locked_version" ]; then
        search_root="$provider_root/$locked_version/$current_platform"
        for candidate in "$search_root"/terraform-provider-tss "$search_root"/terraform-provider-tss_v*; do
            record_provider_candidate "$candidate" || return $?
        done
    else
        for candidate in "$provider_root"/*/"$current_platform"/terraform-provider-tss "$provider_root"/*/"$current_platform"/terraform-provider-tss_v*; do
            record_provider_candidate "$candidate" || return $?
        done
    fi
    if [ -z "$provider_path" ]; then
        return 1
    fi
    printf '%s\n' "$provider_path"
}

run_state_helper() {
    TFSTATE_PASSPHRASE="$STATE_PASSPHRASE" "$TF_HELPER_PATH" "$@"
}

state_helper_supported() {
    [ "$("$1" state-helper-version 2>/dev/null)" = "1" ]
}

prepare_init_arguments() {
    local takes_bootstrap_value=0
    local argument
    BOOTSTRAP_ARGS=()
    for argument in "$@"; do
        if [ "$takes_bootstrap_value" -eq 1 ]; then
            BOOTSTRAP_ARGS+=("$argument")
            takes_bootstrap_value=0
            continue
        fi
        case "$argument" in
            -plugin-dir|--plugin-dir|-lockfile|--lockfile|-get|--get|-input|--input)
                BOOTSTRAP_ARGS+=("$argument")
                takes_bootstrap_value=1
                ;;
            -plugin-dir=*|--plugin-dir=*|-lockfile=*|--lockfile=*|-get=*|--get=*|-input=*|--input=*|-upgrade|--upgrade|-upgrade=*|--upgrade=*|-no-color|--no-color|-json|--json)
                BOOTSTRAP_ARGS+=("$argument")
                ;;
        esac
    done
}

run_provider_bootstrap() {
    terraform init -backend=false "${BOOTSTRAP_ARGS[@]}"
}

restore_bootstrap_state() {
    local restore_exit=0
    local bootstrap_output
    if [ -z "$BOOTSTRAP_STATE_PATH" ] || [ ! -e "$BOOTSTRAP_STATE_PATH" ]; then
        BOOTSTRAP_STATE_PATH=""
        return 0
    fi
    if [ -e "$STATE_FILE" ]; then
        bootstrap_output="${STATE_FILE}.bootstrap-output.$$"
        if mv "$STATE_FILE" "$bootstrap_output"; then
            echo "terraform bootstrap unexpectedly created $STATE_FILE; preserved it as $bootstrap_output."
        else
            echo "Could not preserve the state file created during terraform bootstrap."
            return 1
        fi
        restore_exit=1
    fi
    if mv "$BOOTSTRAP_STATE_PATH" "$STATE_FILE"; then
        BOOTSTRAP_STATE_PATH=""
    else
        echo "Could not restore encrypted state from $BOOTSTRAP_STATE_PATH."
        return 1
    fi
    return "$restore_exit"
}

finish() {
    local requested_exit="$1"
    local encryption_exit="$CLEANUP_INCOMPLETE"
    trap - EXIT HUP INT TERM

    if ! restore_bootstrap_state; then
        encryption_exit=1
    fi
    if [ "$STATE_DECRYPTED" -eq 1 ]; then
        echo "Encrypting state file..."
        if [ -z "$TF_HELPER_PATH" ] || ! run_state_helper encrypt "$STATE_FILE"; then
            echo "Failed to encrypt state file."
            encryption_exit=1
        fi
    fi
    if [ "$BACKUP_DECRYPTED" -eq 1 ]; then
        echo "Encrypting state backup file..."
        if [ -z "$TF_HELPER_PATH" ] || ! run_state_helper encrypt "$STATE_BACKUP_FILE"; then
            echo "Failed to encrypt state backup file."
            encryption_exit=1
        fi
    fi
    if [ "$encryption_exit" -ne 0 ]; then
        echo "State cleanup is incomplete; retaining $LOCK_DIR to prevent another wrapper run until the state files are recovered."
        if [ -n "$TF_HELPER_PATH" ]; then
            echo "The stable encryption helper was retained at $TF_HELPER_PATH for recovery."
        fi
        exit 1
    fi
    if [ -n "$TF_HELPER_PATH" ]; then
        rm -f "$TF_HELPER_PATH"
    fi
    if ! rmdir "$LOCK_DIR" 2>/dev/null; then
        echo "Failed to remove state lock directory $LOCK_DIR."
        exit 1
    fi
    if [ "$requested_exit" -ne 0 ]; then
        exit "$requested_exit"
    fi
    echo "Operation completed successfully."
    exit 0
}

trap 'finish $?' EXIT
trap 'finish 129' HUP
trap 'finish 130' INT
trap 'finish 143' TERM

# Find the Terraform plugin path. When it is not installed yet, hide only the
# encrypted primary state while a backend-disabled init installs providers;
# the real init runs with the caller's arguments after state is decrypted.
TF_PLUGIN_PATH=$(find_provider_binary) # e.g. ".terraform/providers/registry.terraform.io/delineaxpm/tss/5.0.0/linux_amd64/terraform-provider-tss_v5.0.0"
if [ -z "$TF_PLUGIN_PATH" ] || [ ! -f "$TF_PLUGIN_PATH" ] || ! state_helper_supported "$TF_PLUGIN_PATH"; then
    echo "A compatible tss state helper was not found. Fetching the provider using a backend-disabled terraform init"
    prepare_init_arguments "$@"
    if [ -e "$STATE_FILE" ]; then
        BOOTSTRAP_STATE_PATH="${STATE_FILE}.encrypted-bootstrap.$$"
        if [ -e "$BOOTSTRAP_STATE_PATH" ] || ! mv "$STATE_FILE" "$BOOTSTRAP_STATE_PATH"; then
            echo "Could not stage encrypted state for provider bootstrap."
            finish 1
        fi
    fi
    BOOTSTRAP_EXIT=0
    run_provider_bootstrap || BOOTSTRAP_EXIT=$?
    if ! restore_bootstrap_state; then
        BOOTSTRAP_EXIT=1
        CLEANUP_INCOMPLETE=1
    fi
    if [ "$BOOTSTRAP_EXIT" -ne 0 ]; then
        echo "Terraform provider bootstrap failed."
        finish "$BOOTSTRAP_EXIT"
    fi
    TF_PLUGIN_PATH=$(find_provider_binary)
    if [ -z "$TF_PLUGIN_PATH" ] || [ ! -f "$TF_PLUGIN_PATH" ] || ! state_helper_supported "$TF_PLUGIN_PATH"; then
        echo "terraform init did not install a tss provider with state-helper protocol 1; require provider v5 or later and check required_providers and the dependency lock file."
        finish 1
    fi
fi

# Terraform init may replace files under .terraform. Keep the encryption helper
# at a stable path until cleanup has re-encrypted both state files.
TF_HELPER_PATH=$(mktemp "${TMPDIR:-/tmp}/terraform-provider-tss-helper.XXXXXX") || finish 1
if ! cp "$TF_PLUGIN_PATH" "$TF_HELPER_PATH" || ! chmod 700 "$TF_HELPER_PATH"; then
    echo "Could not prepare a stable state-encryption helper."
    finish 1
fi
if ! state_helper_supported "$TF_HELPER_PATH"; then
    echo "The copied tss provider does not support state-helper protocol 1; require provider v5 or later."
    finish 1
fi
if ! "$TF_HELPER_PATH" state-helper-check-layout; then
    echo "The Terraform state layout is not supported by this encrypted-state wrapper. No state was decrypted." >&2
    finish 1
fi

# Decrypt state file before running Terraform
echo "Decrypting state file..."
if ! run_state_helper decrypt "$STATE_FILE"; then
    echo "Failed to decrypt state file. Exiting."
    finish 1
fi
STATE_DECRYPTED=1

# Decrypt state backup file before running Terraform
echo "Decrypting state backup file..."
if ! run_state_helper decrypt "$STATE_BACKUP_FILE"; then
    echo "Failed to decrypt state backup file. Exiting."
    CLEANUP_INCOMPLETE=1
    finish 1
fi
BACKUP_DECRYPTED=1

# Run Terraform init
TERRAFORM_EXIT=0
terraform init "$@" || TERRAFORM_EXIT=$?

# Check if Terraform init succeeded
if [ "$TERRAFORM_EXIT" -ne 0 ]; then
    echo "Terraform init failed."
fi
finish "$TERRAFORM_EXIT"
