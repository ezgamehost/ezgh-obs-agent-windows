# Windows Agent Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build the approved standalone Windows agent in ezgamehost/ezgh-obs-agent-windows.
**Architecture:** One Go SCM service, embedded Windows collectors, native Event Log and local-file sources, two transactional queues, OTLP/HTTPS.
**Tech Stack:** Go 1.27.1, windows_exporter, x/sys/windows, bbolt, OTLP protobuf, WiX MSI.
**Spec:** ../../design.md

## Global Constraints
- No Linux repository edits or shared-module extraction. Preserve Apache attribution for adapted portable code.
- 64 KiB log bodies, 1 MiB export batches, 50,000 points / 16 MiB per scrape.
- Separate 256/128 MiB queues, 64 MiB free reserve; synchronous checkpoints with records.
- Restricted service identity, config and secret ACLs; no public listener or authorization cache.
- Main is the requested workspace workflow. The new standalone repo provides isolation.
- Windows tests and MSI qualification must run on Windows; do not claim cross-compilation proves them.

## Review Focus
- Configuration/credential errors must not echo secrets.
- Native calls cannot cause overlapping collections or unbounded stop.
- Rotation and empty-channel first start cannot advance checkpoints past durable data.
- An malformed success must not discard queued records.
- Install/upgrade must preserve private state without broad ACLs.

### Task 1: Configuration, portable delivery and telemetry
Files: internal/config, internal/spool, internal/export, internal/telemetry, internal/identity, internal/platform.
Interfaces: config.Load(path) (Config,error); spool.Open(path,binding,limit,reserve) (*Queue,error), Enqueue(payloads,checkpoints), Peek/Ack/Replace/Checkpoint; telemetry.EncodeLogs/EncodeMetrics; export.Client.Send(ctx,signal,payload).
- [x] Port existing tested portable queue/OTLP behavior with attribution; write Windows config and platform tests first.
- [x] Prove unknown fields, credential redaction, atomic checkpoints, destination binding, FIFO splits, gzip, malformed successes, backpressure.
- [x] Implement Windows disk-space/locking/ACL adapters; transactional UUID.
- [x] Run go test -race ./... and cross-compile; commit.

### Task 2: Native sources
Files: internal/source/eventlog, internal/source/file, internal/source/windowsmetrics.
Interfaces: source checkpoint func(string)([]byte,error), sink func(context.Context,[]*telemetry.LogItem,map[string][]byte)error. Collector New(names), Gather(), Close().
- [x] Write mapping, bookmark-policy, framing/encoding/rotation tests; run red.
- [x] Implement native pull Event Log subscriptions, durable initial boundaries, strict stale recovery, bounded rendering.
- [x] Implement Windows file sharing/identity, UTF-8/UTF-16LE framing and stable checkpoints.
- [x] Pin exporter, implement private registry adapter, prove build and non-overlap lifecycle.
- [x] Run portable suite and Windows compile; add native tests; commit.

### Task 3: Supervision, SCM and diagnostics
Files: internal/app, internal/service, internal/diagnostics, cmd/ezgh-obs-agent-windows.
Interfaces: app.Run(ctx,config,ready), local named-pipe status/reload, SCM service host.
- [x] Write local sink integration and cancellation/auth-reload tests; run red.
- [x] Wire sources, queues, fair admission, collectors, independent self telemetry, bounded stop.
- [x] Implement SCM lifecycle and local pipe with explicit security descriptor and no remote clients.
- [x] Run tests, vet, executable cross-build; commit.

### Task 4: Packaging, qualification and review
Files: installer/Product.wxs, scripts/*.ps1, .github/workflows/ci.yml, README.md, docs/validation.md.
- [ ] Build MSI/ZIP with protected config/secrets/state, virtual service account, no automatic fresh-install start.
- [ ] Add Windows native CI tests and disposable install/upgrade/uninstall smoke test.
- [ ] Add signing gate, checksums and SBOM; unsigned artifacts explicitly development-only.
- [ ] Document operation and precise validation gaps, run checks, push and inspect Windows CI.
- [ ] Fresh reviewer, fix material findings, final status/upstream verification.

Execution: portable/native source code implemented; local race tests, vet and Windows cross-build passed. Review findings addressed (short-file flush, unexpected HTTP 2xx, independent self metrics). Windows CI qualification is recorded in docs/validation.md.
