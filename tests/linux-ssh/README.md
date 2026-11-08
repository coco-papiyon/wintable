# Windows → Linux real SSH fixture

Issue [#27](https://github.com/coco-papiyon/wintable/issues/27),
[ADR-0001](../../docs/adr/0001-linux-target-support.md) L02 foundation.
Controller: Windows amd64, PowerShell 7.3+, Go **1.27.3**, OpenSSH client,
Docker client/server **28+**. Target: Debian **13 amd64**, `/bin/sh` (dash),
OpenSSH and CPython **3.13.5**. No administrator/root SSH login or sudo is used.
The daemon runs as container root; sessions run as `runner` (nonzero UID).

## Local Linux containers

Select Docker Desktop's **Linux containers** mode first. Windows SSH fixture
jobs use Windows containers and run separately. Do not switch while other
containers are in use. This runner never changes Docker's current context.

```powershell
pwsh -NoProfile -File tests/linux-ssh/run.ps1 -DockerContext desktop-linux
# Clean rebuild + normal test + fault/lock negative tests:
pwsh -NoProfile -File tests/linux-ssh/verify.ps1 -DockerContext desktop-linux
```

`-OutputDirectory` must be a new directory. Default evidence is under
`.cache/linux-ssh/<run-id>`. `-NoCache` rebuilds all Dockerfile layers.
Run time is capped at **10 minutes including preflight and build**. First pull
or snapshot download may exhaust that budget; prefetch the pinned base if
necessary, then rerun with a new evidence directory. There is no rolling fallback.
Ready (host key and Windows-facing SSH banner) has a **60 second** cap from
container start. Cleanup has a separate **90 second** budget. Each external
command is bounded by the lesser of its own cap and the remaining common budget.

## Dedicated remote Linux Docker host

Use a dedicated amd64 host, Docker 28+, and an authenticated OpenSSH management
account authorized for Docker. Obtain its host public key through an existing
trusted administration channel and provision Windows `known_hosts` **before**
connecting. Do not automatically trust `ssh-keyscan` or use `accept-new`.
Configure an alias in the Windows user's `.ssh/config`:

```text
Host wintable-fixture
    HostName <dedicated-host>
    User <docker-management-user>
    IdentityFile <management-private-key-outside-this-repo>
    StrictHostKeyChecking yes
    BatchMode yes
```

```powershell
docker context create wintable-fixture --docker host=ssh://wintable-fixture
pwsh -NoProfile -File tests/linux-ssh/run.ps1 `
  -DockerContext wintable-fixture -TunnelHost wintable-fixture
```

The context's `ssh://` hostname must equal the alias. The runner checks effective
`ssh -G` strict checking and batch mode **before** contacting the daemon. Docker
and the separate `ssh -N -T` forwarding process use this authenticated management
identity. The container only publishes `127.0.0.1:<dynamic>:22` on that host.
The forwarding process binds `127.0.0.1:<dynamic>` on Windows, with strict host
checking, `ExitOnForwardFailure` and keepalives. A local port is selected by
briefly binding port 0; a competing bind causes failure, never a fallback to
another interface. The test verifies the container's own host key independently,
obtained by `docker exec`, at the Windows endpoint. Management and fixture SSH
keys are separate. Direct TCP Docker endpoints are rejected.

Local Linux mode is exercised by the checked-in verification record. Remote
tunnel mode requires a separately provisioned host and is **not yet exercised**
by that record; its documentation alone is not an acceptance result.

## Fixed image and lock

`fixture.lock.json` records registry acquisition time, multiarch index digest,
the actual **linux/amd64 manifest digest** used in `FROM`, immutable Debian and
Debian-security snapshot timestamp, exact Python patch/sshd strings and the
complete sorted `dpkg-query` package/version inventory. `packages.install`
explicitly pins that entire closure (including packages already in the base).
Snapshot Release signatures remain verified; only expiry is disabled for the
historical snapshot. Build context is an allowlist and never contains credentials.
Package post-install host keys are deleted in the same image layer; runtime
host keys are freshly generated in `/run/sshd` for every container.

To update intentionally: inspect `docker buildx imagetools inspect debian:13-slim`,
select its amd64 manifest, choose a signed snapshot, and use an isolated discovery
build to obtain `/usr/bin/python3 -I /fixture/versions.py`. Update `FROM`, snapshot,
`packages.install` and lock together. Review all version changes and rerun
`verify.ps1`. Never fabricate a digest or bypass a failed comparison.
`version-comparison.json` saves expected/actual inventory; `image_id` is the built
image identity, distinct from the registry base digest. A second clean rebuild
must produce the same dependency inventory; image IDs may differ due to metadata.

## What is tested

`internal/transport/ssh_linux_container_test.go` uses the product's
`SSHConnector`/`Session` directly from a Windows Go test executable. The independent
`probe.py` is not the product receiver. It reads synthetic valid UTF-8 in seven-byte
chunks, records exact length/SHA-256/EOF/UID and incomplete decoder boundaries.
This proves split reads, **not TCP packet segmentation**. Exact 2MiB stdin is
verified before any response. Flood mode concurrently writes 1MiB+65537 bytes
to each stream; captures must retain exactly the expected first 1MiB, set both
truncated flags, drain through EOF and preserve exit **23**. Three execs reuse
one SSH connection with exactly three probe starts, proving no retry.

Unknown/changed host keys, wrong client key, password and root login are rejected;
known_hosts is unchanged and probe start count remains zero. Effective sshd
configuration is checked too. Send-blocked and post-EOF deadlines must release
within five seconds. Explicit cancellation and connection close are observed
after an independently recorded EOF; no remote-child termination is inferred.
Finally the container is destroyed, including lingering probe children.

`verify.ps1` runs clean normal acceptance, partial-create, startup, test-stage,
ready deadline, common deadline, simulated cleanup failure and wrong-Python-lock
cases. Every negative must fail at its expected phase and show zero physical
residual resources/credentials. Ready timeout uses an expired ready clock;
common timeout runs a sleeping child under a one-second remaining budget. Cleanup
failure is injected **after actual removal** to verify nonzero/reporting without
deliberately leaving secrets behind. Real Docker removal errors are also recorded
and fail a run; daemon/host crashes cannot guarantee a successful finally.

Normal `go test ./...` skips this opt-in fixture. The runner rejects missing,
failed or skipped required subtests. `-Fault` is for the verification suite.

## Evidence, ownership and recovery

`manifest.json` contains source SHA/dirty status, fixture and test hashes,
controller OS/arch, Docker/Go versions, resource names/run ID, image ID, cases,
exit codes, timestamps and cleanup result. `versions.json` contains target
OS/arch/kernel/Python/sshd/package inventory; `version-comparison.json` holds
the comparison. `test.jsonl` contains only fixed synthetic probe observations,
expected/actual results and pass/fail events. `starts` and probe JSON contain
counts/hashes, never input contents. `sshd.log` and `build.log` are fixed fixture
diagnostics. Build failure or early preflight may leave later artifacts absent;
the manifest still records failure. Missing acceptance evidence cannot pass.

Temporary client credentials live outside evidence in an ACL-restricted directory
under the Windows temp directory, readable only by the current user. No passwords,
private keys, task input, environment dumps or arbitrary product stdout/stderr are
persisted. The runner deletes fixed credential filenames in finally and checks
directory absence. Never put real task secrets in this synthetic fixture.

Container, network and image tag have a GUID and label
`wintable.linux-ssh.run=<run-id>`. No volumes are needed; manifest records this.
Ownership is known **before** create, so partial failures are cleaned as well.
Cleanup checks the label for residual containers (including stopped containers),
networks and images. No shared resource prune is used.
If the runner is forcibly terminated or the host reboots, use manifest names:

```powershell
docker --context <context> container ls -a --filter label=wintable.linux-ssh.run=<run-id>
docker --context <context> network ls --filter label=wintable.linux-ssh.run=<run-id>
docker --context <context> image ls --filter label=wintable.linux-ssh.run=<run-id>
# Verify exact names/labels, then remove ONLY this run's resources:
docker --context <context> container rm -f wintable-linux-<run-id>
docker --context <context> network rm wintable-linux-<run-id>
docker --context <context> image rm wintable-linux-ssh:<run-id>
```

Check `tunnel_pid` against its actual executable/forwarding arguments before
stopping that exact process. Remove only `client`, `client.pub`, `known_hosts`,
`fixture.json`, `wrong-client` in `%TEMP%/wintable-linux-<run-id>`, then the empty
directory; inspect leftovers rather than recursively deleting arbitrary paths.
Inspect the same label again and confirm the credential directory is absent.

## Follow-on acceptance boundary

L01 belongs to #26. This fixture covers the independent **L02** foundation;
L03–L06 product executable/receiver and command acceptance belong to #28;
L07 Linux file_content is deferred. #29 must not publish support before fixture
and product acceptance are complete. A passing Go transport test is not a claim
that the Linux product backend is available.

#28 can pass `-ProductTestScript <repository-owned.ps1>`. It receives
`-FixtureDirectory` (ephemeral `fixture.json`, `client`, `known_hosts`) and
`-EvidenceDirectory`; the endpoint's Address/Port/User fields match inventory.
It must invoke the built **Windows product exe**, persist sanitized expectations
and observations itself, fail on missing/skipped cases, and exit nonzero on
failure. It shares the common run deadline and finally cleanup; the runner does
not persist its arbitrary output. Never copy credential files into evidence.
