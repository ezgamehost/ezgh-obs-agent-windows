# EZGH Windows observability agent

Status: **proposed design; not implemented or release-qualified**. Written 2026-10-12.
Product/repository name: `ezgh-obs-agent-windows`. This is the Windows counterpart
to the [Linux agent](linux-agent.md), designed independently while Linux development
continues. This document does not change platform contracts or authorize deployment.

## Intent and scope

Install one Windows service to send a machine's host metrics and system logs to
EZGH Observability using one project ingest key. Preserve useful Windows metric
names, survive service restarts and temporary outages, and make failures visible
locally. Operators should not need Prometheus, a separate exporter process, an
OpenTelemetry Collector, or a database service.

Proposed support: x64 Windows Server 2019, 2022 and 2025 (Server Core and Desktop
Experience), plus Windows 11 releases still supported by Microsoft at release
time. Workstation inclusion is an assumption pending owner feedback. Server 2016,
Windows 10, ARM64, Windows containers and domain controllers are not v1 qualified
targets. Upstream OS support alone does not establish agent support.

V1 collects Application and System event channels, optional named Admin/Operational
channels, explicit text files, host metrics and agent health. Security is opt-in.
No general ETW sessions, kernel driver, event forwarding server, remote machine
collection, application OTLP receiver, traces, automatic instrumentation, remote
configuration, self-updater, fleet control plane or new console pages. IIS, SQL
Server, Hyper-V, process and custom performance-counter collectors are deferred.

## Approach and alternatives

Recommend **Go, embedded windows_exporter collectors, native Windows Event Log API**.
Use the public `pkg/collector` adapter, a private Prometheus registry and OTLP
protobuf messages. Upstream currently exposes construction, enablement, build and
close methods; isolate those calls because this is not a guaranteed stable API.
Pin and record an upstream release/commit and Go toolchain during implementation,
including its licenses and transitive dependencies. Do not import its `internal`
packages or silently run its executable. Verify the pinned build under the actual
service account before accepting the dependency. [Collector source](https://github.com/prometheus-community/windows_exporter/blob/master/pkg/collector/collection.go).

Alternatives considered:

| Approach | Benefit | Cost / decision |
| --- | --- | --- |
| Custom OpenTelemetry Collector distribution | Existing Windows receivers and exporter machinery | Larger configuration/runtime surface; source checkpoint and queue semantics still need qualification. Reserve as fallback if embedding proves unsuitable. |
| windows_exporter subprocess plus shipper | Clear process boundary, standard exporter | Two process lifecycles, HTTP scraping and packaging overhead; does not meet the preferred single-process experience. |
| Rewrite Windows metric collectors | Complete API control | Duplicates upstream compatibility work; reject for v1. |

Do not move Linux code or introduce a shared module while that thread is active.
Align identity, queue guarantees, error classification and conformance fixtures.
After both implementations stabilize, a separate change may extract portable
spooling/OTLP code with explicit owners and tests. Windows filesystem, service and
source behavior must remain platform-specific.

## Components and flow

```text
Event Log subscriptions ----+-- mapping --> durable log queue -------+
Explicit local text files --+                                       |
                                                                   +--> OTLP/HTTPS
Embedded host collectors ----- mapping --> durable metric queue ----+    EZGH ingest
Agent self metrics -----------+

SCM lifecycle + private local named pipe --> supervision / diagnostics
```

| Package | Responsibility |
| --- | --- |
| `cmd/ezgh-obs-agent-windows` | CLI, exit codes and foreground entry point |
| `internal/service` | SCM state, stop/shutdown, recovery policy integration |
| `internal/config` | Strict versioned YAML, bounds and source identities |
| `internal/identity` | Durable host UUID and destination binding |
| `internal/source/eventlog` | Native handles, filters, bookmarks and recovery |
| `internal/source/file` | File identity, encoding, rotation and checkpoints |
| `internal/source/windowsmetrics` | Pinned upstream lifecycle and private registry |
| `internal/transform` | OTLP mapping, metadata and encoded-size bounds |
| `internal/spool` | Transactional queues, acknowledgments and source positions |
| `internal/export` | TLS, credentials, bounded HTTP and retry policy |
| `internal/diagnostics` | Local status, Event Log diagnostics and self metrics |

Sources submit bounded records plus candidate checkpoints. Only a synchronous
durable queue commit advances a committed checkpoint. HTTP never runs inside a
storage transaction. One process holds an exclusive state-directory lock.

## Installation, service identity and configuration

Ship a signed x64 MSI and a signed ZIP for managed deployments. Both contain the
same executable; the ZIP is not a different runtime. Prefer WiX for MSI authoring.
Use service name `ezgh-obs-agent-windows`, display name `EZGH Observability Agent`.

Install binaries under `%ProgramFiles%\EZGH\ObsAgent`; resolve the actual Windows
known-folder locations. Store config, credential and state under
`%ProgramData%\EZGH\ObsAgent`, in separate `config`, `secrets` and `state`
directories. Support paths containing spaces and Unicode. Quote the SCM binary
path correctly. State must be on a local fixed NTFS volume; reject UNC paths,
remote shares and reparse-point substitutions in managed storage paths.

Run as virtual account `NT SERVICE\ezgh-obs-agent-windows`, with a service SID.
Grant only required local Event Log Readers / Performance Monitor Users access,
using SIDs rather than localized group names. Test actual channel and collector
access: group membership is not proof that every provider permits reads.
Do not default to LocalSystem, grant administrator membership, rewrite all channel
ACLs, or enable `SeSecurityPrivilege` automatically. If a collector needs more
privilege, keep it disabled or make a reviewed, narrowly documented grant.
Installation itself requires elevation. Domain-controller account behavior is
outside the initial support claim.

Directory DACLs protect against unprivileged replacement: Administrators and
SYSTEM administer all files; the service reads config and secrets, and modifies
only state. It cannot replace its executable or configuration. The credential
file inherits no broad Users read access. Validate owner/DACL and opened file
identity on start/reload. A protected secret file is the v1 storage mechanism;
do not claim that machine-scope DPAPI alone isolates secrets between local users.
Queue contents are customer data; disk encryption is an operator policy.

Illustrative configuration (all fields below belong to the proposed v1 schema):

```yaml
version: 1
target: production-windows
endpoint: https://obs-ingest.ezghcloud.com
environment: production
credential_file: 'C:\ProgramData\EZGH\ObsAgent\secrets\ingest-key'
state_dir: 'C:\ProgramData\EZGH\ObsAgent\state'
metrics:
  enabled: true
  interval: 30s
  collectors: [cpu, memory, logical_disk, net, os, system]
logs:
  eventlog:
    channels:
      - {id: application, name: Application, start_at: end}
      - {id: system, name: System, start_at: end}
  files: []
storage:
  log_bytes: 268435456
  metric_bytes: 134217728
  reserve_bytes: 67108864
```

Unknown keys, duplicate source IDs, unsupported collectors, invalid globs, invalid
durations, invalid environment names and overlapping duplicate event-channel
subscriptions fail validation. One subscription per channel, with an optional
XPath filter, bounds duplication. Filter changes require an explicit source
generation change; never reuse a checkpoint silently for a different query.
Other configuration changes require restart. Credential rotation alone supports
`reload-credential` through the secured local control pipe.

No secret in YAML, MSI properties, command arguments, diagnostic bundles or logs.
Provision the restricted file with deployment tooling, then validate and start.
Fresh MSI installation registers the service but does not begin collection before
configuration and credential provisioning. Enable delayed automatic start when
the operator activates it. Expose commands `run`, `validate`, `doctor`, `status`
and `reload-credential`; MSI owns installation/removal. `validate` is offline
schema validation; `doctor` requests service-context access checks when running,
and labels caller-context checks explicitly when stopped. Neither sends test data.

## Host identity and routing

Generate a random UUID once and commit it durably before accepting data, or use an
explicitly configured unique `host_id`. Never derive identity from MachineGuid,
hardware serial numbers, domain credentials or IP addresses. Preserve the ID
through upgrades. Golden images contain no credential, UUID, queue or checkpoint;
clones initialize fresh state before service activation.

All data carries `host.id`, current `host.name`, `os.type=windows`,
`service.instance.id=<host.id>`, `ezgh.metadata.application=windows` and configured
`ezgh.metadata.environment`. Validate the environment with
`[a-z0-9][a-z0-9._-]{0,62}`. Host metrics use `service.name=windows-exporter`;
self metrics use `service.name=ezgh-obs-agent-windows`.

Event logs use an explicitly configured channel service name or the bounded
default `windows-eventlog`. Provider name remains a log attribute, not an
automatically created service per provider. File sources require a service name.
Resource identity must not be overridden by source fields or custom attributes.

The ingest key selects the project. Persist a destination binding of normalized
endpoint, operator-assigned `target`, environment and host ID. Reject changes to
that binding with pending data. `target` identifies the operator's destination,
not a backend resource or credential. A key is opaque: the agent cannot establish
that a replacement belongs to the same project. Operators may reload keys only
for the same destination; changing project requires draining with the old key or
using a new state directory. Never silently redirect an old backlog.

## Host metrics

Proposed default allowlist: `cpu`, `memory`, `logical_disk`, `net`, `os`, `system`.
Allow optional `physical_disk` and `service` after privilege and cardinality tests;
do not enable all upstream defaults implicitly. Restrict service metrics with
explicit configured names/filters. An empty optional filter must not accidentally
select every service. Keep per-process and arbitrary WMI/script collectors out.
These names are drawn from the [upstream collector catalog](https://github.com/prometheus-community/windows_exporter#collectors);
the final pinned dependency must be checked against them.

Gather every 30 seconds with initial jitter. Preserve upstream `windows_*` names,
HELP text and datapoint labels; never rename them to `node_*`. Map gauges to OTLP
gauges, counters to cumulative monotonic sums. Do not calculate rates, fabricate
counter start times, or infer units from suffixes. Leave absent units empty to
avoid unwanted backend name translation. Unsupported families are diagnosed and
omitted. Preserve collector success/duration metrics and successful collector
results when another collector fails.

One gather at a time. Native calls may outlive a context timeout; inspect the
pinned adapter's worker lifecycle and do not launch replacement collections while
any prior worker is active. If its API cannot enforce that invariant, block release
until the adapter is corrected. Diagnose a gather older than 60 seconds through
independent status. SCM stop remains bounded even if a native call is stuck.

Defaults: at most 50,000 points and 16 MiB serialized per scrape. Reject an
over-limit scrape whole, count it, and keep other signals running. These are
admission limits, not measured throughput claims. Conformance fixtures must show
two hosts remain distinct through EZGH PromQL's `job`/`instance` projection and
counter names do not acquire a second `_total` suffix. Do not promote arbitrary
resource/log fields to metric labels.

## Event Log acquisition and recovery

Use `wevtapi.dll` directly via bounded Go Windows bindings. Prefer a pull
subscription (`EvtSubscribe`, signaled event, `EvtNext`) to make backpressure
explicit. Own handles in the source worker and close every event, bookmark,
subscription and publisher handle on all paths. No PowerShell or `wevtutil`
subprocess in the collection loop. Native subscriptions cover Admin/Operational
channels, not Analytic/Debug channels. [Microsoft subscription API](https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtsubscribe).

Application and System begin at the live tail on first activation; `beginning`
explicitly requests retained history. A saved bookmark takes precedence over
`start_at`. Persist the initial tail boundary before starting normal collection,
so a restart before the first batch does not repeatedly skip ahead. If a channel
is empty, persist an initialized-empty marker and resume at oldest on restart;
do not treat it as an uninitialized tail subscription again. Use strict bookmark
resume after the committed event. Atomically
store bookmark XML, source generation and last event identity with each admitted
batch; a read or successful format operation is not a checkpoint commit.
[Microsoft bookmarks](https://learn.microsoft.com/en-us/windows/win32/wes/bookmarking-events).

If the bookmark is stale/missing because of overwrite or channel clearing, emit
a durable local gap diagnostic/counter and recreate the subscription from the
oldest retained matching event. Possible replay is preferable to silently jumping
to the newest event. Do not estimate a lost-record count from record-ID gaps:
filters and resets make that unreliable. Record IDs are channel-local and can
reset. On repeated stale results, back off and keep recovery visible. Ordinary
access-denied/missing-channel errors do not discard the checkpoint; retry boundedly
and mark that source degraded. Disabled sources retain their checkpoints.

When the queue fills, stop reads; if needed close and later resume from the
committed bookmark. The Windows channel remains the upstream buffer and can wrap.
Fairly schedule bounded batches across channels/files so one busy channel does
not starve others. Cap configured channels at 32 by default.

Security collection requires an explicit entry and access check. Explain denied
access without automatically modifying Security channel policy or enabling audit
policies. Security event records can contain account and network details. Do not
copy them to the agent's own diagnostics.

Map native event timestamp to OTLP event time and acquisition time to observed
time. Map Windows levels 1/2/3/4/5 to FATAL/ERROR/WARN/INFO/DEBUG, respectively;
level 0 or unknown leaves severity unset. Audit success/failure keywords are not
severity. Include a bounded allowlist: channel, provider name/GUID, event ID,
record ID, version, level, task, opcode, keywords, process/thread ID and activity
ID. An activity GUID is not automatically an OTLP trace ID.

Render the message with provider metadata and record its locale. If the provider
or message resource is missing, emit a bounded event-XML body with a format-failed
attribute; preserve the event instead of blocking the channel. Do not flatten
arbitrary EventData/UserData into metric labels or unrestricted attributes.
Cap native render allocations at 1 MiB per event, output body at 64 KiB UTF-8,
individual attributes at 4 KiB and serialized log record at 128 KiB. Truncate on
valid UTF-8 boundaries and mark it. If native rendering exceeds its cap, enqueue
a metadata-only oversized-event marker with its bookmark; explicitly count lost
content instead of retrying that poison record forever. Handle malformed UTF-16
with replacement and diagnostics. Never allocate the provider's requested length
without checking the bound first.

## Optional text files

Read explicit local globs, with source ID, service, exclusions and `start_at`.
Default new files to tail; `beginning` opts into history. Support UTF-8 and explicit
UTF-16LE, plus BOM-based detection; do not guess legacy code pages. Frame CRLF/LF
correctly across read boundaries, including split UTF-16 code units and BOMs.
Checkpoints store raw byte offsets, encoding and file generation, not character
counts. Multiline parsing and structured JSON extraction are deferred.

Open with read/write/delete sharing so the agent does not prevent application
rotation. Track volume identity, Windows file ID and generation evidence from the
opened handle, not just path/mtime. Drain renamed open handles and discover their
replacement independently; detect truncation and restart the new generation at
zero. File-ID reuse needs generation checks. Copytruncate and an old renamed file
no longer discoverable after restart can lose data; document those limits.
Reject directories, devices, UNC sources and reparse points; verify handles to
avoid discovery/open substitution. Do not automatically read every disk.

Cap open files at 128 and bodies at 64 KiB. Consume an oversized line fully before
committing its offset; do not split it into falsely independent log records.
Emit a trailing incomplete line after five idle seconds; subsequent writes become
a new record. No default application-file source duplicates Event Log collection.

## Durable storage and export

Use separate embedded bbolt databases for logs and metrics, synchronous commits,
with records and source checkpoints in the same log transaction. This is durable
source/delivery state, not a service authorization cache. Default database budgets
are 256 MiB logs and 128 MiB metrics, within a recommended 512 MiB state allocation;
leave 128 MiB for metadata/maintenance and enforce a separate 64 MiB filesystem
free reserve. Account for physical pages, reuse and transaction growth before
admission. Do not promise a hard disk cap based only on live payload bytes.

Full log storage pauses sources; full metric storage skips new scrapes. Keep
independent local status available. Reuse pages without automatic compaction that
doubles disk requirements. Unknown storage versions or corruption stop the
affected pipeline with repair guidance; never delete data automatically. Windows
locking, flush durability and crash recovery require native tests, not inference
from Linux behavior. Store identity/binding transactionally; avoid assuming POSIX
rename/directory-fsync semantics on Windows.

Export FIFO, one request in flight per signal, to `/v1/logs` and `/v1/metrics`
using gzip OTLP/HTTP protobuf. Bound requests to 1 MiB uncompressed serialized
OTLP, independently check compressed size, and flush available batches within a
second. Reuse original timestamps/payloads on retry. Use a 15-second request timeout
and 64 KiB response cap. HTTPS certificate/hostname validation is mandatory; use
the machine trust store with an optional explicit CA bundle. Reject redirects and
URL credentials. Support an explicitly configured HTTP CONNECT proxy; do not
promise automatic WinINET/PAC discovery or interactive/domain proxy authentication.
V1 proxy endpoints must not contain credentials.

| Result | Required action |
| --- | --- |
| Valid full OTLP success | Durably remove acknowledged batch |
| Valid partial success | Acknowledge batch, count rejected items, do not replay accepted items |
| Network failure or HTTP 429/502/503/504 | Retry with full jitter, exponential 1–60 second backoff; honor a longer valid Retry-After (seconds or date) |
| HTTP 401/403 | Retain backlog, pause affected exports until explicit credential reload |
| HTTP 404/405 or unsupported endpoint | Retain backlog and require configuration correction |
| HTTP 413 | Transactionally replace batch with smaller batches; count/drop an indivisible refused item |
| Permanent payload rejection such as HTTP 400 | Count lost items and acknowledge the rejected batch; record only safe reason/status |
| Malformed/oversized success response or unexpected server failure | Retain batch, bounded backoff and visible protocol failure; no false acknowledgment |

No claim of exactly-once or unlimited retention. A crash after server acceptance
but before local acknowledgment can duplicate data; expired source retention,
disk failure and permanent rejection can lose it. Backend timestamps/retention
also constrain how useful a long backlog is. Credential reload validates and
opens the new file before swapping the in-memory secret, never writes the secret
to queue records, and clears authentication pause only after successful reload.
The remote backend remains authoritative for each request.

## Lifecycle, diagnostics and upgrades

SCM reports start pending while validating local state, running after pipelines
are initialized, and stop pending during bounded shutdown. Network outages do
not prevent service startup. Fatal config/state errors return a clear nonzero
exit without an infinite restart loop. Configure bounded delayed restart recovery
for crashes; do not reboot the host. Stop new collection, commit admitted work,
cancel exports, then close handles/stores within 20 seconds. Unsent committed
data stays queued. A stuck native worker cannot make shutdown wait forever.

Private named pipe access is limited to Administrators, SYSTEM and the service
SID, rejects remote clients and protects against pipe-name squatting. It exposes
bounded status/access-check and credential-reload operations, not arbitrary paths,
commands or telemetry contents. No inbound HTTP listener/firewall exception.

Status reports configured sources, access failures, collection age, queue bytes
and oldest age, last successful export, auth/protocol state, skipped scrapes,
truncation and source gaps. Self metrics export every 60 seconds through the
metric queue with bounded reason/signal labels. Agent operational diagnostics
use its registered Event Log provider, excluded from its own Application channel
subscription by default. Local diagnostic output must not contain customer event
bodies, credentials or raw server error bodies.

MSI upgrades stop the service before replacing binaries, preserve config/secrets/
UUID/checkpoints/backlog and restore prior running state only after validation.
Support repair and uninstall on Server Core. Uninstall stops/removes the service
and provider registration but preserves ProgramData and reports its location;
cleanup is an explicit separate operator action. Do not remove shared group
permissions or directories the installer does not own.

Version storage formats explicitly. A migration must be recoverable within the
configured disk budget; otherwise fail before changing state. Never roll back
only the executable across an incompatible storage migration. Initial releases
should avoid irreversible migrations. Sign executable/MSI with a trusted code
signing identity and timestamp; publish checksums, SBOM and dependency notices.
If signing is unavailable, label artifacts development-only, not production-ready.

## Compatibility evidence and implementation qualification

Inspected platform pins: obs-stack `bce4bba06a0a87eea6982b3eb20b0d2788c84a50`
and console `72138da4afe3c82c21c33b5f9d049fc4c2be8ec7`. The pinned backend's
`internal/ingest/limits.go` allows 5 MiB wire / 64 MiB decoded requests;
`internal/query/prom/names.go` defines name and resource-label translation.
The pinned console's `services/obs/src/api.ts` decodes existing API response
envelopes. This design sends existing OTLP and proposes no response changes.
Recheck pins before implementation/release; no production request was made.

Use [identity and authorization](../../contracts/identity-and-authz.md),
[OTLP ingest](otlp-ingest.md) and the actual backend as interface references.
The historical ingest design and identity contract contain ingest-key cache
provisions; they conflict with the workspace's explicit no-cache rule. This agent
adds no authorization cache, does not depend on those provisions and does not
modify backend authorization as part of this work.

Before calling v1 releasable:

1. Build the pinned exporter adapter on native Windows; prove enabled collectors
   work with the proposed virtual account, including localized Windows. Check
   non-overlap, handle cleanup and shutdown with a deliberately stuck provider.
2. Test native event subscriptions: first-start tail/history, committed bookmarks,
   restart/crash, wrap, clear, unavailable channels, denied Security access, XPath
   generation changes, provider-message failures and oversized Unicode records.
3. Test native file rotation, delete sharing, truncation, file-ID reuse, reboot,
   encodings, trailing lines, path substitution, DACL failures and file limits.
4. Kill the process between enqueue/checkpoint/ack steps; restart and prove no
   checkpoint outruns durable data. Test full disk, physical queue limits, state
   locks, corruption and unknown versions without deleting customer state.
5. Use a disposable HTTPS OTLP sink for gzip, headers, both Retry-After forms,
   auth pause/reload, TLS errors, redirects, 413 splitting, partial success,
   malformed responses and bounded memory under outage.
6. Round-trip metric fixtures through the pinned EZGH translation and query
   implementation: names/units, resets, two distinct hosts, service metrics,
   log metadata and missing-provider fallback. Use local disposable dependencies,
   never production for destructive tests.
7. Exercise clean MSI install, provisioning/start, repair, upgrade with backlog,
   failed upgrade rollback, restart, uninstall preservation, path quoting and
   signature verification on every claimed OS variant. Verify no public listener.
8. Run Go formatting, vet, portable tests, native Windows tests/race where supported,
   fuzz parsers and fault-inject the spool. CI needs Windows runners plus explicit
   Server Core/OS-matrix qualification; a successful cross-compile is insufficient.
9. Measure a 24-hour soak with representative high-volume Event Logs and 1,000
   log records/sec plus 10,000 metric points/30 sec, including outage/recovery.
   Record CPU, private bytes, handle/goroutine count, disk growth and drops. Initial
   target is under 200 MiB steady private bytes with no unbounded growth; this is
   an engineering target, not a capacity claim or validated benchmark.

Implementation sequence: native collector/permissions feasibility; service and
secure configuration; durable store and OTLP fixtures; event channels; files;
packaging and full OS qualification. This is a design handoff, not an approved
implementation plan. The source repository, dependency pins and release artifacts
are created in a subsequent implementation task.
