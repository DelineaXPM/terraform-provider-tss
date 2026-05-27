# Changelog

All notable changes to this project will be documented in this file. This project adheres to [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and (from v4.0.0 onward) [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

For the release history prior to v4.0.0, see the [GitHub Releases](https://github.com/DelineaXPM/terraform-provider-tss/releases) page.

## [Unreleased]

### Breaking

- Documented Terraform floor raised from 0.13 to **1.11**. `tss_resource_secret`'s new write-only password attribute requires Terraform 1.11. `README.md`, `docs/index.md`, and `examples/secrets/*.tf` (7 example configurations) updated accordingly. Users on Terraform &lt; 1.11 will see an `Unsupported Terraform Core version` error at `terraform init` once on v4.0.0.
- `itemid`, `fieldid`, `slug`, and `fielddescription` on `tss_resource_secret.fields` are now `Computed`-only (closes PBI 718755). Configurations that set any of these four attributes in `fields` blocks will fail at plan time with `Can't configure a value for X: its value will be decided automatically based on the result of applying this configuration`. These values are server-assigned by Secret Server; the previous `Optional+Computed` declaration silently accepted user input and then overwrote it. **Migration:** delete those attributes from your `fields` blocks. `fileattachmentid` remains `Optional+Computed` because it is genuinely user-settable for file-type fields.

### Security

- Bumped `google.golang.org/grpc` v1.75.1 → v1.79.3 to close [CVE-2026-33186](https://github.com/advisories/GHSA-p77j-4mvh-x3m3) (gRPC-Go authorization bypass via missing leading slash in `:path`).
- Bumped `go` directive and CI/release workflow Go version from 1.25.1 → 1.26.2 to pick up Go stdlib fixes spanning `crypto/x509`, `crypto/tls`, and `html/template` (GO-2026-4865, GO-2026-4866, GO-2026-4870, GO-2026-4946, GO-2026-4947).
- Added `.snyk` policy ignoring CVE-2026-2454 in `github.com/vmihailenco/msgpack/v5` v5.4.1. Rationale captured in the policy file: msgpack is used only inside the trusted gRPC plugin protocol with the Terraform CLI parent process; the advisory's network-input attack surface does not apply to a Terraform provider.

### Added

- New `.github/workflows/ci.yml` — runs `go build`, `go vet`, and `go test ./...` on every pull request and push to `main` or `dev/v4.0.0`.
- Acceptance test scaffolding (`delinea/resource_secret_acceptance_test.go`) using `ProtoV6ProviderFactories`, `testAccPreCheck`, and `testAccSecretConfig`. Three `TestAcc*` functions exercise `tss_resource_secret` against a live tenant (partial-fields, all-fields, refresh-no-drift). Gated on `TF_ACC=1`. New dep: `github.com/hashicorp/terraform-plugin-testing v1.15.0` (which transitively bumps `terraform-plugin-framework` to v1.19.0 and `terraform-plugin-go` to v0.31.0).
- `delinea/otel_init_test.go` — disables OpenTelemetry trace export (`OTEL_TRACES_EXPORTER=none`, `OTEL_SDK_DISABLED=true`) unconditionally in `init()`. Terraform 1.12+ tries to export OTLP traces to `localhost:4317`, costing ~10s per terraform subprocess; under terraform-plugin-testing that compounds into minutes.
- New `password_value` write-only attribute on the `fields` block of `tss_resource_secret` (`StringAttribute`, Optional, WriteOnly, Sensitive). Framework-enforced never-in-state. Supplies the password on Create/Update.
- New `password_wo_version` Int64 attribute on the `fields` block — rotation trigger. Bumping it during `terraform apply` rotates the password to whatever `password_value` currently holds.
- New "Password handling" section in `README.md` and attribute documentation in `docs/resources/resource_secret.md` covering setting/rotating passwords and the state-safety guarantee.
- `TestAccTSSSecret_PasswordNotInState`, `TestAccTSSSecret_RotateViaVersion`, `TestAccTSSSecret_ChangePwWithoutBumpIsNoop`, `TestAccTSSSecret_RefreshNoDrift` acceptance tests in `delinea/resource_secret_acceptance_test.go`.
- `TestAccTSSSecret_SshKeyGeneration` and `TestAccTSSSecret_SshKeyAndPasswordMixed` acceptance tests, env-gated on `TSS_TEST_SSH_TEMPLATE_ID` and `TSS_TEST_MIXED_TEMPLATE_ID`. Verify `sshKeyFieldPlanModifier`'s server-side computation path and confirm non-interference with the password-handling changes for templates that contain both Password and SSH-key fields.
- New `generate` Bool attribute on the `fields` block of `tss_resource_secret` — closes [gh #110](https://github.com/DelineaXPM/terraform-provider-tss/issues/110). When set to `true` on a password field, the provider asks Secret Server for a password matching the template's password-requirement policy (via `POST /api/v1/secret-templates/generate-password/{fieldId}`) and uses it as the field's value. The generated password reaches Secret Server through the normal create/update flow and is never written to Terraform state. Mutually exclusive with `password_value` and `itemvalue`. Rotates via `password_wo_version` bumps the same way `password_value` does.
- New "Server-side password generation" section in `README.md` and `generate` attribute documentation in `docs/resources/resource_secret.md`.
- `TestAccTSSSecret_GeneratePasswordFromTemplatePolicy`, `TestAccTSSSecret_GeneratePasswordRotation`, and `TestAccTSSSecret_GenerateNoBumpIsNoOp` acceptance tests covering create-with-generate, rotate-with-version-bump, and idempotent-no-rotate.
- `Description` strings on the `itemid`, `fieldid`, `fileattachmentid`, `slug`, and `fielddescription` schema attributes so `terraform providers schema -json` and `tfplugindocs` output explain each one.
- "Computed fields on `tss_resource_secret.fields`" section in `README.md` and matching content (Read-Only attribute list + "Computed Fields" subsection) in `docs/resources/resource_secret.md`. Addresses the customer-reported documentation gap on "auto-incrementing key fields" (closes PBI 718755).
- `TestSchema_ServerAssignedFieldsAreComputedOnly` and `TestSchema_FileAttachmentIDIsOptionalAndComputed` schema-contract unit tests guarding the new `Computed`-only declarations.

### Changed

- `itemvalue` is now `Sensitive: true` on the `fields` block. Defense in depth — passwords supplied via the legacy `itemvalue` path are masked in CLI output, even though the recommended path is the new `password_value` attribute.

### Fixed

- `delinea/provider.go`: corrected two malformed `log.Printf` format strings (the trailing `map[string]interface{}{...}` argument was being silently dropped) and removed an unreachable `serverConfig == nil` guard. Hygiene only — closes pre-existing `go vet` failures so the new CI workflow passes from day one.
- `flattenSecret` block-count mismatch on partial-fields configs (`delinea/resource_secret.go`). Threaded a reference-fields list through `readSecretByID`; callers pass `plan.Fields` (Create/Update) or `state.Fields` (Read); case-insensitive match; nil reference disables filtering. Resolves "Provider produced inconsistent result after apply" (block count N → M) on configs that specify only a subset of the template's fields. Adds unit tests and three `Example_*` functions documenting the scenarios.
- Password values no longer land in `terraform.tfstate` or `terraform show -json` (PBI 700142). `flattenSecret` now writes `types.StringNull()` for `itemvalue` when the API response indicates `IsPassword == true`; `getSecretData` routes `password_value` → API payload (with `itemvalue` as legacy fallback), and omits the field from the Update payload entirely when no new password is supplied so TSS preserves the existing server-side value. Removes a stray `FileAttachmentID = strconv.Atoi(ItemValue)` parse that could corrupt `FileAttachmentID` on numeric passwords. `alignFieldsToReference` preserves `password_wo_version` from reference (TSS does not round-trip it).

[Unreleased]: https://github.com/DelineaXPM/terraform-provider-tss/compare/v3.1.1...HEAD
