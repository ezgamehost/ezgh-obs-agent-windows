# Validation and release qualification

This is an initial implementation, not a production-qualified release.

## Executed locally

- `go test -race ./...`: portable configuration, queue, OTLP mapping/export,
  Event Log mapping, file encoding/checkpoint/rotation and end-to-end file delivery.
- `go vet ./...` and `GOOS=windows GOARCH=amd64 go build ./...`.

## CI verified on 2026-10-12 (2026-10-13 UTC)

Implementation revision `c215fea` passed [CI run 38343207179](https://github.com/ezgamehost/ezgh-obs-agent-windows/actions/runs/38343207179):

| Runner | Checks | Result |
| --- | --- | --- |
| Ubuntu | Full race suite, vet | Passed |
| Windows Server 2022 | Full race suite, vet, MSI/ZIP build, installed-service lifecycle | Passed |
| Windows Server 2025 | Full race suite, vet, MSI/ZIP build, installed-service lifecycle | Passed |

The Windows checks cover native Event Log subscription/bookmark creation, idle
polling followed by new events, default metrics, file checkpoints/rotation and
restricted credential access. Installed-service tests verify all six default
collectors and Application/System subscription health under the virtual service
account; fresh installation remains stopped until activation. Upgrade and repair
restore a running service with backlog present, and uninstall preserves config,
credentials and state. Fixture traffic targets an unreachable loopback endpoint.

Unsigned development MSI/ZIP packages, CycloneDX module inventories, dependency
license texts, checksums and installer logs are available in that run's artifacts.
They are not a signed production release.

Review identified and fixed short-first-file line loss, unexpected HTTP 2xx
acknowledgment, and self-metrics dependence on optional host collection. Native CI
also caught MSI component authoring defects and empty Event Log reads returning
Win32 4317; the latter has a regression test covering idle/resumed delivery.
Individual collector failure now marks local metric health as degraded.

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
