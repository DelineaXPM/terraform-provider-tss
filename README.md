# Delinea Secret Server - Terraform Provider

The [Delinea](https://delinea.com/) [Secret Server](https://delinea.com/products/secret-server/) [Terraform](https://www.terraform.io/) Provider allows you to access and reference Secrets in your vault for use in Terraform configurations. Detail documentation of this integration is available [here](https://docs.delinea.com/online-help/integrations/terraform/configure.htm)

## Install via Registry

> Preferred way to install

The latest release can be [downloaded from the terraform registry](https://registry.terraform.io/providers/DelineaXPM/tss/latest). The documentation can be found [here](https://registry.terraform.io/providers/DelineaXPM/tss/latest/docs).

If wish to install straight from source, follow the steps below.

## Install from Source

### Terraform 1.11 and later

This provider requires **Terraform 1.11 or later**. The write-only attribute support used by `password_value` on `tss_resource_secret` is a Terraform 1.11 core feature; earlier versions will reject the schema.

Build with `go build -o terraform-provider-tss_v5.0.0 .` and place the binary under the implied local mirror directory for the canonical registry address (`registry.terraform.io/delineaxpm/tss`), so that `source = "DelineaXPM/tss"` resolves to it:

#### Windows

```text
%APPDATA%\terraform.d\plugins
└───registry.terraform.io
    └───delineaxpm
        └───tss
            └───5.0.0
                └───windows_<arch>
                    └───terraform-provider-tss_v5.0.0.exe
```

Use `windows_amd64` on 64-bit Intel/AMD Windows or `windows_arm64` on 64-bit ARM Windows.

#### Linux / macOS / FreeBSD

```text
~/.terraform.d/plugins
└───registry.terraform.io
    └───delineaxpm
        └───tss
            └───5.0.0
                └───<os>_<arch>
                    └───terraform-provider-tss_v5.0.0
```

Replace `<os>_<arch>` with the platform reported by `terraform version -json`, such as `linux_amd64`, `linux_arm64`, `linux_arm`, `darwin_arm64`, or `freebsd_amd64`. The directory must match the Terraform CLI's platform, not merely another provider binary retained in `.terraform` for a different machine.

For development, a `dev_overrides` block in the CLI configuration (see the `Examples are valid HCL` step in `.github/workflows/ci.yml`) points Terraform at a build directory without any versioned layout.

## Usage

Include the `terraform` block in your configuration, or plan, that specifies the Terraform version and provider:

```terraform
terraform {
  required_version = ">= 1.11.0"
  required_providers {
    tss = {
      source  = "DelineaXPM/tss"
      version = ">= 5.0.0"
    }
  }
}
```

Standalone example configurations live under `examples/secrets/<name>/` (one root module per directory) with matching variable files under `vars/secrets/`, for example:

```sh
cd examples/secrets/secret_get
terraform init
terraform apply -var-file=../../../vars/secrets/secret_get.tfvars
```

The variable files below show the values each example expects.

If you want to access the secret server via the platform, provide the platform URL as an input to tss_server_url, and the platform user credentials to tss_username and tss_password.

The server URL must use `https://` for any remote host. As of v5.0.0, a plaintext `http://` URL to a non-loopback host is rejected when the provider is configured — plan and apply fail immediately with an error naming the opt-in — because it would expose the credential on the wire; `http://` remains usable only for loopback addresses such as a local test server. If you operate a remote Secret Server behind TLS termination and genuinely need plaintext HTTP from the provider, set `allow_insecure_http = true` on the provider block (or `TSS_ALLOW_INSECURE_HTTP=true`) after accepting that risk.

With username/password authentication, v5.0.0 probes the configured origin before sending credentials so the SDK can distinguish Secret Server from Delinea Platform. A reverse proxy must allow an unauthenticated `GET` to `<server_url>/api/v1/healthcheck` for Secret Server (or `<server_url>/health` for Platform) to return a direct 2xx healthy response; redirects are not followed. Static Secret Server token authentication skips this probe.

Get Secret By ID:

```hcl
tss_username   = "my_app_user"
tss_password   = "Passw0rd."
tss_server_url = "https://example/SecretServer"
tss_secret_id  = "1"
```
Get Secrets By ID:

```hcl
tss_username   = "my_app_user"
tss_password   = "Passw0rd."
tss_server_url = "https://example/SecretServer"
tss_secret_ids  = ["1", "2", "3"]
```
Create/Update Secret (`examples/secrets/secret_create`):

```hcl
tss_username          = "my_app_user"
tss_password          = "Passw0rd."
tss_server_url        = "https://example/SecretServer"
tss_secret_name       = "Windows Account"
tss_secret_siteid     = 1
tss_secret_folderid   = -1
tss_secret_templateid = 6003
password_wo_version   = 1
fields = [
  {
    fieldname = "Machine"
    itemvalue = "hostname/ip"
  },
  {
    fieldname = "Username"
    itemvalue = "my_app_user"
  },
  {
    # Password fields are write-only: supply password_value, never itemvalue.
    fieldname      = "Password"
    is_password    = true
    password_value = "Passw0rd."
  },
  {
    fieldname = "Notes"
    itemvalue = ""
  }
]
```
The variables above target the Windows Account secret template. To support a different template:

1. Open the template in Secret Server under Admin => Secret Templates.
2. Replace `tss_secret_templateid` with the template ID shown in the URL when the template is open.
3. Open the Fields tab.
4. Add one entry per template field to `fields`, using the field name (or slug) and its value; mark password fields with `is_password = true` and supply `password_value`. Templates differ in the number of fields, so add or remove entries accordingly.

Delete Secret:

This functionality deactivates the secret in Delinea Secret Server.

## Password handling (`tss_resource_secret`)

Password fields on `tss_resource_secret` use Terraform's write-only attribute mechanism to keep the value out of Terraform state. Use the `password_value` attribute for any field where the template has `IsPassword` set (e.g. the `Password` field on the built-in Password template).

### Setting a password on a new secret

```hcl
resource "tss_resource_secret" "example" {
  name             = "example"
  folderid         = "5"
  siteid           = "1"
  secrettemplateid = "2"

  fields {
    fieldname = "Username"
    itemvalue = "myuser"
  }
  fields {
    fieldname           = "Password"
    password_value      = var.db_password
    password_wo_version = 1
  }
}
```

`password_value` is write-only: it is sent to Secret Server on create/update but never written to `terraform.tfstate` or emitted by `terraform show -json`. It must be non-empty and always paired with `password_wo_version`, which the provider uses as an explicit rotation trigger because Terraform cannot compare a value that isn't in state.

### Rotating a password

Change both attributes together and apply:

```hcl
fields {
  fieldname           = "Password"
  password_value      = var.db_password_new     # new value
  password_wo_version = 2                        # bumped from 1
}
```

On plan, Terraform sees the `password_wo_version` change and schedules an update; on apply, the provider reads `password_value` from config and sends it to Secret Server. Any new integer works — only the change is significant.

### Server-side password generation

If you don't want to supply a password value yourself, set `generate = true` on the field. The provider asks Secret Server for a password matching the template's password-requirement policy and uses that value:

```hcl
resource "tss_resource_secret" "db" {
  name             = "db-prod"
  folderid         = "42"
  siteid           = "1"
  secrettemplateid = "2"

  fields {
    fieldname = "Username"
    itemvalue = "dbadmin"
  }
  fields {
    fieldname           = "Password"
    generate            = true
    password_wo_version = 1
  }
}
```

`generate` is mutually exclusive with `password_value` and (for password fields) `itemvalue` — the provider rejects configs that set both. Rotation works the same way as for `password_value`: bump `password_wo_version` to ask for a new generated password on the next apply. Re-applying with the same `password_wo_version` is a no-op (no API call to the generate endpoint, no rotation).

The generated password reaches Secret Server through the normal create/update flow. It never lands in `terraform.tfstate` because the provider records an empty `itemvalue` for every field the template marks `IsPassword`.

### Guarantees and caveats

- `terraform.tfstate` contains no plaintext password. Post-apply, `itemvalue` is an empty string for any field the template marks `IsPassword`.
- `terraform show -json` does not emit the password value.
- CLI plan/apply output masks the value because `password_value` and `itemvalue` are both marked `Sensitive`.
- The password value still lives in your Terraform config. Use env vars, a TF Cloud sensitive variable, or a secret-backend data source for the actual input.
- Configs that set `itemvalue` on a password field are rejected at apply time with an error naming the migration. State never holds a password, so a configured `itemvalue` could never match the value read back and Terraform would otherwise fail with "Provider produced inconsistent result after apply". Move those fields to `password_value` + `password_wo_version`.
- Optional secret-level settings you do not configure (`checkoutenabled`, `requirescomment`, `enableinheritpermissions`, `secretpolicyid`, …) are preserved across updates; an update never replaces an omitted setting with a zero value. On create, the SDK's scalar request fields encode omitted settings as `false`/`0`; set any non-zero creation policy explicitly. The refreshed Secret Server response supplies the computed state. `active` is the exception and defaults to `true`.
- Upgrading from v3.1.x or earlier: existing state files still contain plaintext passwords. The first `terraform apply` or `terraform refresh` after the upgrade will replace them with empty strings; no data loss, just state cleanup.

## Computed fields on `tss_resource_secret.fields`

Each `fields` block has attributes that Secret Server assigns automatically after `terraform apply`. They appear in Terraform state but are not user-settable:

- `itemid` — database ID of this field-value record. Auto-assigned by Secret Server; sequential per newly-created secret.
- `fieldid` — the template field ID. Stable per template, shared across every secret that uses the template. Not sequential.
- `fileattachmentid` — the server-assigned attachment ID for a file-type field.
- `slug` — the field's URL slug, assigned by the template.
- `fielddescription` — the field description, set by the template.
- `isfile`, `islist`, `isnotes`, and `ispassword` — field kinds assigned by the template.
- `listtype` — the template's list type, when returned by Secret Server.

Setting any of these ten in your config produces a plan error ("Can't configure a value for `itemid`: its value will be decided automatically based on the result of applying this configuration"). That's the signal that these are server- or template-assigned — leave them out.

## Upgrading state from earlier provider versions

### From v4.0.x to v5.0.0

State is compatible (schema version 1 is unchanged), but four behaviors differ:

- **`itemvalue` on a password field is rejected.** v4 accepted it and then failed apply with an "inconsistent result" error; v5 fails earlier with a message pointing to `password_value` + `password_wo_version`.
- **Reads fail closed.** A secret Secret Server reports as inaccessible (its ambiguous HTTP 400, or any 404) keeps the resource in state and errors, rather than planning a replacement, because those responses cannot distinguish a deleted secret from missing permission or a missing attachment. After confirming a secret was deleted outside Terraform, run `terraform state rm` and apply again.
- **Server-owned settings are preserved.** Earlier versions sent `false`/`0` for every optional setting the configuration omitted whenever anything else changed, silently disabling checkout, comment requirements, permission inheritance, and similar settings. v5 preserves the values Secret Server holds.
- **Template field metadata is read-only.** Remove `isfile`, `islist`, `isnotes`, `ispassword`, and `listtype` from `fields` blocks; the provider derives available metadata from Secret Server instead of accepting values it cannot write or reliably round-trip.

See the CHANGELOG for the full list of v5 breaking changes.

### From v3.x to v4.0.0

`terraform-provider-tss` v4.0.0 versions the schema (`Version: 1`) and ships a state upgrader that runs automatically the first time `terraform plan` is invoked against existing v3.x state. No user action is required. The upgrader carries every existing field across and leaves the new v4 attributes (`password_value`, `password_wo_version`, `generate`) at their null defaults.

If you used `itemvalue` to supply a password on a `tss_resource_secret` field, the first post-upgrade refresh replaces that plaintext state value with an empty string for `IsPassword` fields. This state cleanup does not change the password in TSS. Move the password to `password_value` + `password_wo_version` before the next apply; v5 rejects `itemvalue` on password fields.

### From v2.x to v4.0.0

**v2.x and v4.0.0 are not directly compatible at the state level.** v2.x used Terraform's older Plugin SDKv2; v3+ switched to the modern Plugin Framework, which uses a different on-disk state shape. v4.0.0 does not include a v2 → v4 upgrader — translating SDKv2 state to framework state is substantial separate work that we have deferred.

If you are still on v2.x, choose one of the following paths:

1. **Stay on v2.x.** Reasonable for stable production where the resource isn't actively churning and the security/feature additions in v3+ aren't required. v2.x continues to function indefinitely against TSS.

2. **Drop and recreate state** (recommended for most v2 → v4 migrations):
   - For each affected resource: `terraform state rm tss_resource_secret.X`
   - Adjust your config so the recreated resource doesn't collide on `name` with the existing secret in TSS, **or** delete the corresponding secret from TSS first.
   - `terraform apply` recreates the resource fresh, recorded under v4 state.
   - **Caveat:** any secret you delete from TSS to avoid a name collision is permanently gone; back up first.

3. **Manual state surgery.** Edit `terraform.tfstate` to convert SDKv2 shape to framework shape and bump `schema_version` to `1`. Risky, unsupported, requires understanding both shapes — only suitable for experienced operators with reliable backups.

There is no automatic path from v2.x state to v4 today. If your scenario isn't served by 1–3 above, please file a GitHub issue.

## Delete Secret by ID

The `tss_secret_deletion` resource allows you to delete secrets by their ID, even if they are not managed by Terraform state.

### Delete a Single Secret

```hcl
resource "tss_secret_deletion" "delete_secret" {
  secret_id = 12345
}
```

Apply this configuration to delete the secret with ID `12345`. After deletion, run `terraform destroy` to remove the resource from state before deleting another secret.

### Delete Multiple Secrets

```hcl
resource "tss_secret_deletion" "delete_secrets" {
  for_each = toset(["1001", "1002", "1003"])
  secret_id = tonumber(each.key)
}
```

This will delete all secrets listed in the set. Each deletion is tracked separately in state.

**Best Practice:**
- After deleting, run `terraform destroy` to clean up the state before deleting new secrets.
- For batch deletions, use `for_each` or unique resource names.

**Note:** The resource performs deletion during the `terraform apply` phase. The resource is tracked in state to prevent repeated deletion attempts. "Creating..." in logs means the deletion is being performed.

If a deleted secret is later restored, refresh reports a warning but retains the completed operation in state. Terraform does not delete the restored secret again automatically. Remove the operation from state and apply again only when another deletion is intended.

## Environment variables

### Provider env-var fallback

The provider resolves each setting in the order *explicit provider attribute > environment variable > unset*. With the env vars exported, the provider block can be left empty:

| Env var          | Provider attribute |
|------------------|--------------------|
| `TSS_SERVER_URL` | `server_url`       |
| `TSS_USERNAME`   | `username`         |
| `TSS_PASSWORD`   | `password`         |
| `TSS_TOKEN`      | `token`            |
| `TSS_DOMAIN`     | `domain`           |
| `TSS_ALLOW_INSECURE_HTTP` | `allow_insecure_http` |

```hcl
provider "tss" {}
```

Username/password example (Linux/macOS):

```sh
export TSS_SERVER_URL="https://localhost/SecretServer"
export TSS_USERNAME="my_app_user"
export TSS_PASSWORD="Passw0rd."
terraform plan
```

OAuth token instead of username/password:

```sh
export TSS_SERVER_URL="https://localhost/SecretServer"
export TSS_TOKEN="PASTE_TOKEN_HERE"
terraform plan
```

Windows (`cmd.exe`) uses `set` instead of `export`; PowerShell uses `$Env:TSS_SERVER_URL = "..."`.

After the env-var fallback runs, the provider enforces:

- `server_url` must be set.
- Exactly one of `(username + password)` or `token` must be set.

Configurations that violate these rules produce a plan-time error naming the missing or conflicting attribute.

### Using Terraform input variables

As an alternative, expose credentials via Terraform input variables. Each `variable "x"` block is populated from the corresponding `TF_VAR_x` environment variable:

```hcl
provider "tss" {
  username   = var.tss_username
  password   = var.tss_password
  server_url = var.tss_server_url
}
```

Linux/macOS:

```sh
export TF_VAR_tss_username="my_app_user"
export TF_VAR_tss_password="Passw0rd."
export TF_VAR_tss_server_url="https://localhost/SecretServer"
terraform plan
```

Windows (`cmd.exe`):

```bat
set TF_VAR_tss_username=my_app_user
set TF_VAR_tss_password=Passw0rd.
set TF_VAR_tss_server_url=https://localhost/SecretServer
terraform plan
```

This is Terraform's general variable mechanism and is independent of the provider's `TSS_*` fallback above — pick whichever fits your environment.

### Provider attributes

All provider attributes are `Optional` at the schema level; the auth rules above are enforced at plan time after the env-var fallback resolves.

- `server_url` (String) — Secret Server base URL, e.g. `https://localhost/SecretServer`. Falls back to `TSS_SERVER_URL`.
- `username` (String) — Secret Server username. Falls back to `TSS_USERNAME`.
- `password` (String, Sensitive) — Secret Server password. Falls back to `TSS_PASSWORD`.
- `token` (String, Sensitive) — OAuth token (alternative to username/password). Falls back to `TSS_TOKEN`.
- `domain` (String) — Domain for AD-backed accounts. Falls back to `TSS_DOMAIN`.
- `allow_insecure_http` (Boolean) — Permit a plaintext `http://` server URL to a non-loopback host; default `false`. Plaintext HTTP exposes the credential on the wire, so set it only after accepting that risk. Falls back to `TSS_ALLOW_INSECURE_HTTP`.

## Domain user accounts

Domain users, such as Active Directory accounts, authenticate by setting the provider's `domain` attribute alongside `username` and `password`:

```hcl
provider "tss" {
  server_url = var.tss_server_url
  username   = var.tss_username
  password   = var.tss_password
  domain     = "mycompany.com"
}
```

Alternatively, the domain can be provided with an environment variable:

```sh
$ export TSS_DOMAIN="mycompany.com"
```

## Encrypt terraform state file using script wrapper

Terraform supports multiple backends to securely store state files, such as AWS S3, Azure Blob Storage, and others. These backends also include built-in state locking mechanisms. However, when storing state files on a local machine drive, you need to manually encrypt the state file data to keep it secure.

To use encrypted local state during the Terraform workflow, decrypt it before executing Terraform commands and encrypt it again afterward. This can be achieved by creating script wrappers around Terraform commands like terraform init, terraform apply, and terraform destroy.

To use these script wrappers, copy them into the directory that holds your Terraform configuration (copy `terraform_wrapper.ps1` alongside the three Windows `.bat` launchers). They locate the installed provider binary under `.terraform/` themselves. The scripts read `TFSTATE_PASSPHRASE`; when it is unset and both legacy variables are present, they fall back to concatenating `TF_VAR_tss_username` and `TF_VAR_tss_password`, which existing encrypted state files were produced with. Prefer a dedicated `TFSTATE_PASSPHRASE` so the state encryption key is not the Secret Server credential.

The provider binary performs the encryption (`terraform-provider-tss encrypt <file>` / `decrypt <file>`, AES-256-GCM with a PBKDF2-derived key) and exits non-zero on failure. It writes through a synced temporary file before replacing the destination; that final rename is atomic on Unix, while Windows replacement has the operating system's normal crash semantics. The wrappers acquire an atomic lock directory before decrypting, and cleanup re-encrypts files even when Terraform fails or is interrupted. The dedicated passphrase is exposed only to the short-lived encryption helper, not to Terraform or its other provider subprocesses. Provider discovery is restricted to Terraform's canonical `.terraform/providers/registry.terraform.io/delineaxpm/tss/` installation tree, a syntactically valid version selected by the dependency lock file, and the platform reported by the current Terraform CLI. The wrappers reject malformed or path-like lock-file versions before invoking a helper. They require state-helper protocol 1, provided by tss v5 and later, and refuse an older provider before changing state. If state restoration or encryption fails, the wrapper retains its lock directory until an operator recovers the files. On Unix, decrypted and encrypted files are written with mode `0600`; on Windows, they receive a protected DACL granting access only to the current user instead of inheriting the directory's ACL.

These reference wrappers support only the default workspace with the local backend's default `terraform.tfstate` and `terraform.tfstate.backup` paths. They fail before decrypting when Terraform selects a named workspace, a remote backend, a custom local-backend `path` or `workspace_dir`, or an apply/destroy `-state`, `-state-out`, or `-backup` option. Remote backends should use their own encryption at rest. For a custom local layout, adapt the scripts to enumerate every state path before relying on them; running the stock wrappers would otherwise risk leaving the actual state plaintext.

When the provider is missing or too old, the init wrapper performs a backend-disabled provider bootstrap first. Provider-installation flags such as `-upgrade` and `-plugin-dir` are forwarded to that bootstrap. The wrapper rejects `-from-module` because Terraform requires its target directory to be empty, while the wrapper itself and its state lock necessarily occupy that directory. To use `-from-module`, run a plain `terraform init -from-module=...` in an empty target first, then copy the wrappers into the populated module and run the init wrapper normally.

Scripts for reference and more detailed information are available [here](https://docs.delinea.com/online-help/integrations/terraform/index.htm). You can modify file paths in these scripts as needed. Scripts for reference are also available [here](encryption_scripts).

You can then execute the script wrappers as shown below:

Usage (For Linux / macOS / FreeBSD)
```
$ bash terraform_init.sh
$ bash terraform_apply.sh
$ bash terraform_destroy.sh
```
Usage (For Windows)
```
> terraform_init.bat
> terraform_apply.bat
> terraform_destroy.bat
```

## Ephemeral Resource

This ephemeral resource fetches secret values from Delinea Secret Server at runtime without storing them in Terraform state. It is useful for handling sensitive secret data dynamically without persisting them. An ephemeral resource can be used as shown below.
Ephemeral resources require Terraform 1.10 or later; this provider's overall floor is Terraform 1.11 (see above), which satisfies that requirement.

Get Secret By ID:

```hcl
ephemeral "tss_secret" "my_password" {
  id    = var.tss_secret_id
  field = "password"
}
```
Get Secrets By ID:

```hcl
ephemeral "tss_secrets" "my_passwords" {
  ids    = var.tss_secret_ids
  field = "password"
}
```

# SSH Key Generation in Terraform Provider for TSS

This guide explains how to properly configure and use SSH key generation in the Terraform Provider for TSS.

## How to Define SSH Key Arguments

### In `examples/secrets/secret_create/main.tf`

The example resource configuration handles password fields, SSH key generation, and plain fields from one `fields` variable:

```hcl
resource "tss_resource_secret" "secret_name" {
  name             = var.tss_secret_name
  folderid         = var.tss_secret_folderid
  siteid           = var.tss_secret_siteid
  secrettemplateid = var.tss_secret_templateid
  active           = true
  dynamic "fields" {
    for_each = var.fields
    content {
      fieldname = fields.value.fieldname
      # Password fields: send via write-only password_value (never lands in state).
      # SSH key fields when generating: leave null so the server fills in.
      # Everything else: pass through itemvalue unchanged.
      itemvalue = (
        fields.value.is_password ? null :
        (var.generate_ssh_keys && contains(var.ssh_key_fields, fields.value.fieldname)) ? null :
        fields.value.itemvalue
      )
      password_value      = fields.value.is_password ? fields.value.password_value : null
      password_wo_version = fields.value.is_password ? var.password_wo_version : null
    }
  }

  sshkeyargs {
    generatepassphrase = var.generate_passphrase
    generatesshkeys    = var.generate_ssh_keys
  }
}
```

Important notes:
1. Use a `dynamic "fields"` block to conditionally set field values
2. Set SSH key field values to `null` when using generation
3. Include the `sshkeyargs` block with appropriate boolean settings

SSH key generation is a create-only Secret Server operation. Changing `sshkeyargs` replaces the secret so the new generation settings take effect.

### In `vars/secrets/secret_ssh.tfvars`

The `secret_ssh.tfvars` file defines the variable values:

```hcl
fields = [
  {
    fieldname = "Public Key"
    itemvalue = null
  },
  {
    fieldname = "Private Key"
    itemvalue = null
  },
  {
    fieldname = "Private Key Passphrase"
    itemvalue = null
  }
]

# SSH Key Generation Settings
generate_passphrase = true
generate_ssh_keys   = true
```

Important notes:
1. Set `itemvalue` to `null` for SSH key fields
2. Set the appropriate boolean values for `generate_passphrase` and `generate_ssh_keys`

## Limitations and Considerations

1. **Creation Only**: SSH key generation is only supported during secret creation, not during updates
2. **Field Values**: When updating a secret with previously generated SSH keys, the provider will automatically preserve the generated values
