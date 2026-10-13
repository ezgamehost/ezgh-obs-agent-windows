# ezgh-obs-agent-windows

A standalone Windows service that sends host metrics and logs to EZGH Observability.

- Embedded `windows_exporter` 0.31.9 collectors; no exporter process or HTTP listener.
- Native Windows Event Log subscriptions with durable bookmarks.
- Optional local UTF-8/UTF-16LE text files, with rotation and byte checkpoints.
- Separate bounded, synchronous log and metric queues; OTLP/HTTPS with gzip.
- Virtual service account, protected credentials, local named-pipe diagnostics.

**Initial implementation; release qualification is incomplete.** See
[validation](docs/validation.md). Unsigned build artifacts are development-only.
No production deployment, automatic update service or remote control plane is included.

## Install and activate

The intended x64 targets are Windows Server 2019/2022/2025 and supported Windows 11
releases. CI covers Server 2022/2025 runners; Server Core, Server 2019 and Windows 11
must be qualified before claiming release support. No ARM64, Windows 10 or domain
controller qualification is claimed.

In an elevated PowerShell session, install a verified MSI:

```powershell
msiexec /i .\ezgh-obs-agent-windows-0.1.0-x64.msi /qn
```

Production packages must have valid Authenticode signatures and checksums from
the release. Current unsigned artifacts use `-unsigned-development` in their names.
Do not relabel them as signed releases.

The installer creates `ezgh-obs-agent-windows` as a stopped, manual-start service
running as `NT SERVICE\ezgh-obs-agent-windows`. It grants Event Log Readers and
Performance Monitor Users membership, using localized group lookup from SIDs.
It opens no firewall port and grants no administrator/Security-audit privilege.

Copy the example to the protected config directory, edit it, and provision a
project ingest key with `obs.logs.ingest` and `obs.metrics.ingest` permissions
(agent health uses metrics even when host metric collection is disabled):

```powershell
$agent = "$env:ProgramFiles\EZGH\ObsAgent\ezgh-obs-agent-windows.exe"
$base = "$env:ProgramData\EZGH\ObsAgent"
Copy-Item "$env:ProgramFiles\EZGH\ObsAgent\config.example.yaml" "$base\config\config.yaml"
# Edit config.yaml; provision the secret file using your secret manager/editor.
# Never pass a key as a command-line argument or MSI property.
& $agent repair-permissions
& $agent --config "$base\config\config.yaml" validate
& $agent activate
& $agent status
& $agent doctor
```

The key file is `$base\secrets\ingest-key`. `repair-permissions` reapplies the
installer's owner and DACL to the managed directories/files; it requires elevation.
Stop the service first when repairing existing state permissions. Custom file
sources need explicit read permission for the service account. Custom credential
locations need equally restricted DACLs; `repair-permissions` only manages the
standard ProgramData tree.

`activate` validates the standard config and credential, enables delayed automatic
startup and starts the service. Other commands accept `--config PATH` **before**
the command. `validate` checks syntax without sending data. `status` reads the
running service's snapshot; `doctor` checks credentials, Event Log access and
state writability in the service context. They never send probe telemetry.

## Configuration and collection

See [examples/config.yaml](examples/config.yaml). Unknown fields and unsupported
collector names fail validation. Default metrics are `cpu`, `memory`, `logical_disk`,
`net`, `os`, `system`, every 30 seconds. Upstream `windows_*` names and labels are
preserved. Counters remain cumulative. Optional physical-disk and service collectors
remain outside the implemented allowlist pending their privilege/cardinality tests.

Application and System channels begin at their live tail on first start. Use
`start_at: beginning` to ingest retained history. A persisted bookmark takes
precedence. Cleared/overwritten bookmarks recover at the oldest available matching
event and report a gap; duplicates are possible. Example additional channel:

```yaml
logs:
  eventlog:
    channels:
      - id: application
        name: Application
        start_at: end
      - id: powershell
        name: Microsoft-Windows-PowerShell/Operational
        start_at: end
        generation: 0
  files:
    - id: myapp
      paths: ['C:\Apps\MyApp\logs\*.log']
      exclude: ['C:\Apps\MyApp\logs\debug.log']
      service: myapp
      start_at: end
      encoding: auto
```

A `query` is an optional native XPath filter. Increase a channel's `generation`
when changing its name/query. Security requires an explicit channel and suitable
read access; the installer does not enable auditing or broaden that channel's
policy. Admin/Operational channels are supported; Analytic/Debug and general ETW
sessions are not.

`encoding` accepts `auto` (UTF-8 unless a UTF-16LE BOM is present), `utf-8`, or
`utf-16le`. File patterns are absolute, local and nonrecursive; use `*`/`?`, not
bracket character classes. No UNC sources, reparse points, multiline parsing or
structured field extraction. At most 128 files are held open by default.
Rotation does not block the writer's rename/delete. Copytruncate can lose writes;
a rotated file outside configured globs cannot be recovered after restart.
An unterminated line flushes after five idle seconds; subsequent writes form a
new record. Oversized bodies are truncated to 64 KiB while their full source
position is consumed.

Resources carry persistent random `host.id`, current `host.name`, `os.type=windows`,
`service.instance.id`, `ezgh.metadata.application=windows` and configured environment.
Metrics use `service.name=windows-exporter`; Event Logs use `windows-eventlog`
unless a channel sets `service`; files require a service. No hardware serial or
MachineGuid is used as identity. The ingest key selects the project and environment
selects the dataset.

## Delivery and operation

Queues default to 256 MiB logs and 128 MiB metrics, with a separate 64 MiB free disk
reserve. Provision at least 512 MiB for state. Admission accounts for database page
and transaction overhead; usable telemetry space is lower than those limits.
When full, logs pause at their committed positions and metric scrapes are skipped.
Source retention can expire during an outage. Unknown/corrupt state is not deleted.

Logs/bookmarks commit atomically. Delivery is best effort with durable retries;
a crash after server acceptance but before local acknowledgment can duplicate data.
Permanent payload rejection loses the affected batch and is counted. Partial success
is acknowledged without replaying its accepted records. HTTP 429/502/503/504 and
network failures retry with jitter and `Retry-After`; malformed success responses
retain the batch. HTTP 413 splits batches. Authentication or endpoint errors retain
backlog and pause the affected exporter.

For same-project key rotation, atomically replace the protected secret file, retain
its owner/DACL, then run:

```powershell
& $agent reload-credential
```

Other config changes require service restart. Never redirect backlog by swapping in
a key for another project. The opaque key cannot reveal that mistake to the agent.
Changing endpoint, target, environment or host ID with pending data is refused;
use a new state directory for a new destination. Do not copy state or secrets into
VM templates. Preserve state through upgrades. Explicit host IDs must be unique.

The `proxy` setting supports an explicit HTTP/HTTPS CONNECT proxy origin without
credentials. No automatic PAC/WinINET discovery or domain proxy authentication.
TLS verification cannot be disabled; `ca_file` can add an explicit CA bundle.

MSI upgrades/repairs preserve managed ProgramData, restore prior startup mode, and
restart a previously running service after config/key validation. Uninstall removes
the service/binaries/provider but preserves config, secrets and state. Remove those
explicitly when retiring an installation and revoke its ingest key. No automatic
compaction or storage-format migration is currently implemented.

## Build and test

Go 1.27.1 is pinned in `go.mod`. On Windows, use PowerShell with .NET 8 SDK available:

```powershell
go test -race -timeout 3m ./...
go vet ./...
.\scripts\build.ps1
```

The script installs pinned WiX 5.0.2 into `dist/tools`, builds the x64 executable,
MSI and ZIP, and emits a CycloneDX module inventory and SHA-256 checksums. Native
race tests require a supported Windows C compiler. For signed artifacts, provision
a code-signing certificate in the Windows store, set `EZGH_SIGNING_THUMBPRINT`,
ensure `signtool` is on PATH, and pass `-RequireSignature`. Certificate provisioning
and release publication are intentionally not automated by CI.

Portable tests run on macOS/Linux; cross-compilation checks Windows APIs but cannot
validate their runtime behavior. `scripts/msi-smoke.ps1` is restricted to disposable
GitHub Actions runners because it installs/upgrades/removes the service. It uses
an unreachable loopback endpoint and fixture credentials, never production.

Architecture: [design](docs/design.md). Apache-2.0; see [NOTICE](NOTICE) for adapted
code attribution. Dependencies retain their own licenses.
