# Validation and release qualification

This is an initial implementation, not a production-qualified release.

## Executed locally

- `go test -race ./...`: portable configuration, queue, OTLP mapping/export,
  Event Log mapping, file encoding/checkpoint/rotation and end-to-end file delivery.
- `go vet ./...` and `GOOS=windows GOARCH=amd64 go build ./...`.
- Native Windows tests and MSI smoke tests are defined in CI; final outcomes are
  recorded below after the workflow completes.

## Release gates still requiring Windows qualification

- Server 2019, Server Core and supported Windows 11; non-English installations.
- Real restricted-account collection on every target, Security opt-in permissions,
  high-volume channel wrap/clear, empty-channel initialization and malformed providers.
- 24-hour soak, handle/private-byte measurements, physical disk exhaustion,
  abrupt OS power loss, antivirus interference and storage-corruption recovery.
- Failed-upgrade rollback and reboot/service-start behavior on each target.
- Trusted Authenticode certificate and signing infrastructure. Unsigned CI artifacts
  are explicitly development-only.

No backend/console schema changes or production requests are part of these tests.
The optional physical_disk/service collectors are deferred until qualification.
