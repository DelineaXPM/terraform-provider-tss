# Changelog

Release notes for v3.1.1 and earlier are on the [GitHub Releases page](https://github.com/DelineaXPM/terraform-provider-tss/releases).

Format follows [Keep a Changelog](https://keepachangelog.com/); the project uses [Semantic Versioning](https://semver.org/).

## v4.0.0

### Breaking

- **Terraform 1.11 or later is now required.** The new write-only `password_value` attribute relies on Terraform core's WriteOnly support, which landed in Terraform 1.11. Users on older Terraform versions will get a clear "Unsupported Terraform Core version" error at `terraform init`. The v3.1.x line remains supported for pre-1.11 environments; customers unable to upgrade Terraform should stay on v3.1.x until they can.
- **`itemid`, `fieldid`, `slug`, and `fielddescription` on `tss_resource_secret.fields` are now computed-only.** Configurations that set any of these four attributes will fail plan with "Can't configure a value for `<name>`: its value will be decided automatically based on the result of applying this configuration." These values are server-assigned by Secret Server; the previous `Optional: true, Computed: true` declaration silently accepted user-supplied values in plan, which was misleading. Migration: delete these attributes from your `fields` blocks. `fileattachmentid` remains `Optional+Computed` because it is genuinely user-settable on file-type fields.

### Added

- **Write-only `password_value` attribute** on `tss_resource_secret` `fields` blocks. The value is never written to `terraform.tfstate` or emitted by `terraform show -json`. Use it for any field where the template has `IsPassword` set.
- **`password_wo_version` attribute** — an Int64 rotation trigger paired with `password_value`. Bump it to signal Terraform that `password_value` has changed and should be re-sent to Secret Server on the next apply. Any new integer works; only the change is significant.
- "Password handling" section in [README.md](README.md#password-handling-tss_resource_secret) covering setup, rotation, guarantees, and the upgrade path.
- Attribute documentation for `password_value` and `password_wo_version` in [docs/resources/resource_secret.md](docs/resources/resource_secret.md).
- "Computed fields on `tss_resource_secret.fields`" section in [README.md](README.md#computed-fields-on-tss_resource_secretfields) and matching content in [docs/resources/resource_secret.md](docs/resources/resource_secret.md), describing `itemid`, `fieldid`, `slug`, `fielddescription`, and the `fileattachmentid` exception. Addresses the customer-reported documentation gap on "auto-incrementing key fields."
- `Description` strings on the `itemid`, `fieldid`, `fileattachmentid`, `slug`, and `fielddescription` schema attributes so `terraform providers schema -json` and `tfplugindocs` output explain each one.

### Changed

- `itemvalue` on `tss_resource_secret` fields is now marked **Sensitive**. CLI output (`plan` / `apply` diffs) masks the value regardless of which attribute the user chose. Defense in depth — `password_value` is the primary state-hygiene mechanism, but `itemvalue` output masking closes a side channel for users who haven't migrated yet.
- `required_version` in all `examples/secrets/*.tf` bumped from `"1.12.1"` to `">= 1.11.0"`. README and `docs/index.md` usage snippets updated to show the 1.11 floor and the provider `version = ">= 4.0.0"` constraint.

### Fixed

- **"Provider produced inconsistent result after apply"** when a `tss_resource_secret` config specifies fewer `fields` blocks than the template defines. The block-count mismatch was causing `terraform apply` to exit non-zero even though the secret was created successfully in Secret Server. `flattenSecret` now filters and reorders the API response to match the user's configuration before returning state.
- Three pre-existing `go vet` failures in `delinea/provider.go` that were blocking clean `go test ./...` runs: two `log.Printf` calls with format strings missing directives (silently dropping their map payloads), and an unreachable `if serverConfig == nil` guard.
- **Misleading schema contract for server-assigned fields on `tss_resource_secret.fields`.** Before this release, `itemid`, `fieldid`, `slug`, and `fielddescription` were declared `Optional: true, Computed: true`, so Terraform silently accepted user-supplied values in plan even though Secret Server overwrites them on apply. They are now `Computed: true` only; setting them in config produces a plan-time error. `fileattachmentid` remains `Optional+Computed` as the documented exception for file-type fields.

### Security

- **Password field values are no longer written to Terraform state.** Addresses the customer-reported behavior where password field values for `tss_resource_secret` landed in `terraform.tfstate` and `terraform show -json` in plaintext. Passwords now pass through `password_value` on create/update and never land in state. Verified end-to-end by acceptance tests (`TestAccTSSSecret_PasswordValueNotInState`, `TestAccTSSSecret_PasswordRotation`).

### Migration from v3.1.x or earlier

- **State files contain plaintext passwords.** On your first `terraform apply` or `terraform refresh` against v4.0.0, the provider nulls `itemvalue` in state for any field the template marks `IsPassword`. No data loss — the password still lives in Secret Server — just state cleanup. Customers who script against `.fields[].itemvalue` for password fields will see empty strings where they previously got the password; switch those consumers to the `tss_secret` data source or `tss_secret` ephemeral resource if they need the value at runtime.
- **Configs that set `itemvalue` on password fields need to migrate.** The legacy path still works (the provider sends the value to Secret Server), but because `flattenSecret` now returns `""` for password `itemvalue`, every subsequent `terraform plan` will show a diff. Fix by switching the password field to `password_value` + `password_wo_version`:

  ```hcl
  # Before (v3.1.x)
  fields {
    fieldname = "Password"
    itemvalue = var.db_password
  }

  # After (v4.0.0)
  fields {
    fieldname           = "Password"
    password_value      = var.db_password
    password_wo_version = 1
  }
  ```

### Test infrastructure

- Seven new `TestAccTSSSecret_*` acceptance tests covering the partial-fields, all-fields, password-value, rotation, and refresh-no-drift scenarios. Gated by `TF_ACC=1`; require `TSS_SERVER_URL`, `TSS_TEST_FOLDER_ID`, and auth env vars (`TSS_USERNAME`+`TSS_PASSWORD` or `TSS_TOKEN`).
- Two schema-contract unit tests (`TestSchema_ServerAssignedFieldsAreComputedOnly`, `TestSchema_FileAttachmentIDIsOptionalAndComputed`) that assert the Computed-only declarations on `itemid`/`fieldid`/`slug`/`fielddescription` and the Optional+Computed declaration on `fileattachmentid`. Guard against someone re-adding `Optional: true` to the server-assigned attributes.
- Two additional env-gated acceptance tests for SSH key generation (`TestAccTSSSecret_SshKeyGeneration`) and mixed SSH+password secrets (`TestAccTSSSecret_SshKeyAndPasswordMixed`); skip unless `TSS_TEST_SSH_TEMPLATE_ID` / `TSS_TEST_MIXED_TEMPLATE_ID` point at suitable templates on the tenant.
- `terraform-plugin-framework` v1.16.1 → v1.19.0 (minor bump, production). `terraform-plugin-testing` v1.16.0 added (test-only).
