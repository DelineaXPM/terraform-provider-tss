# Changelog

All notable changes to this project will be documented in this file. This project adheres to [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and (from v4.0.0 onward) [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

For the release history prior to v4.0.0, see the [GitHub Releases](https://github.com/DelineaXPM/terraform-provider-tss/releases) page.

## [Unreleased]

### Security

- Bumped `google.golang.org/grpc` v1.75.1 → v1.79.3 to close [CVE-2026-33186](https://github.com/advisories/GHSA-p77j-4mvh-x3m3) (gRPC-Go authorization bypass via missing leading slash in `:path`).
- Bumped `go` directive and CI/release workflow Go version from 1.25.1 → 1.26.2 to pick up Go stdlib fixes spanning `crypto/x509`, `crypto/tls`, and `html/template` (GO-2026-4865, GO-2026-4866, GO-2026-4870, GO-2026-4946, GO-2026-4947).
- Added `.snyk` policy ignoring CVE-2026-2454 in `github.com/vmihailenco/msgpack/v5` v5.4.1. Rationale captured in the policy file: msgpack is used only inside the trusted gRPC plugin protocol with the Terraform CLI parent process; the advisory's network-input attack surface does not apply to a Terraform provider.

### Added

- New `.github/workflows/ci.yml` — runs `go build`, `go vet`, and `go test ./...` on every pull request and push to `main` or `dev/v4.0.0`.
- Acceptance test scaffolding (`delinea/resource_secret_acceptance_test.go`) using `ProtoV6ProviderFactories`, `testAccPreCheck`, and `testAccSecretConfig`. Three `TestAcc*` functions exercise `tss_resource_secret` against a live tenant (partial-fields, all-fields, refresh-no-drift). Gated on `TF_ACC=1`. New dep: `github.com/hashicorp/terraform-plugin-testing v1.15.0` (which transitively bumps `terraform-plugin-framework` to v1.19.0 and `terraform-plugin-go` to v0.31.0).
- `delinea/otel_init_test.go` — disables OpenTelemetry trace export (`OTEL_TRACES_EXPORTER=none`, `OTEL_SDK_DISABLED=true`) unconditionally in `init()`. Terraform 1.12+ tries to export OTLP traces to `localhost:4317`, costing ~10s per terraform subprocess; under terraform-plugin-testing that compounds into minutes.

### Fixed

- `delinea/provider.go`: corrected two malformed `log.Printf` format strings (the trailing `map[string]interface{}{...}` argument was being silently dropped) and removed an unreachable `serverConfig == nil` guard. Hygiene only — closes pre-existing `go vet` failures so the new CI workflow passes from day one.
- `flattenSecret` block-count mismatch on partial-fields configs (`delinea/resource_secret.go`). Threaded a reference-fields list through `readSecretByID`; callers pass `plan.Fields` (Create/Update) or `state.Fields` (Read); case-insensitive match; nil reference disables filtering. Resolves "Provider produced inconsistent result after apply" (block count N → M) on configs that specify only a subset of the template's fields. Adds unit tests and three `Example_*` functions documenting the scenarios.

[Unreleased]: https://github.com/DelineaXPM/terraform-provider-tss/compare/v3.1.1...HEAD
