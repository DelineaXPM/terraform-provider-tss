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

### Fixed

- `delinea/provider.go`: corrected two malformed `log.Printf` format strings (the trailing `map[string]interface{}{...}` argument was being silently dropped) and removed an unreachable `serverConfig == nil` guard. Hygiene only — closes pre-existing `go vet` failures so the new CI workflow passes from day one.

[Unreleased]: https://github.com/DelineaXPM/terraform-provider-tss/compare/v3.1.1...HEAD
