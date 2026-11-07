# SSH transport (Issue #8)

`transport.SSHConnector` implements the existing Connector contract.
`cli.RunEngine` defaults a nil Connector to this implementation. Callers must
provide distribution-owned fixed wrapper lines and a module response decoder.
The executable continues to exit 3 until #9/#10 provide those modules; this
transport does not synthesize successful module responses.

Inventory private_key_file and known_hosts_file paths are resolved relative to
the inventory by the configuration layer. Only public-key authentication is
enabled: no password, agent, keyboard-interactive or interactive trust prompt.
Encrypted keys use `WINTABLE_KEY_PASSPHRASE`. Set it through your process
environment without placing its value in task JSON or command arguments.

Provision known_hosts beforehand using host keys verified through a trusted
channel. A nonstandard port uses `[host]:port` (including bracketed IPv6).
Missing/malformed/revoked trust entries fail with host_key_failed; an absent
host uses host_key_unknown and a mismatched key uses host_key_changed.
Trust files are never changed. Key read/parse/passphrase and rejected public-key
authentication failures expose authentication_failed. Other connection and
post-connect errors expose connection_failed and connection_lost respectively.
No underlying error strings or error chains are returned. The library's default
cryptographic algorithms are used without adding legacy algorithms.

Each task opens one exec channel without PTY or environment requests. The
trusted Request.Command is sent verbatim and the already prepared Request.Stdin
is transferred byte-for-byte, followed by EOF. Transport does not interpolate
task data or regenerate JSON. Request validation belongs to preflight/module.
One host connection is reused for successful tasks; there are no retries.

The engine supplies the 10-second connection and per-task deadlines. TCP dial,
SSH handshake, channel opening, exec acknowledgement, stdin write/EOF, output
drain and exit-status wait are cancellable. Interruption closes the underlying
connection, joins outstanding work and makes it unusable. Close is idempotent
and may run concurrently with Run. Runs on a host are serialized by the engine.
Normal task completion closes only its channel. Each output retains at most
1MiB and continues draining beyond that limit; exact-limit output is not marked
truncated. Nonzero process exit is a completed Response, distinct from transport
failure. A lost/unknown exit status remains null. The engine maps connection
failure to unreachable, execution failure to failed with unknown change, and
parent cancellation to cancelled. Default results omit captured output bodies.

Closing SSH does not guarantee that a Windows child process stops. Confirm
target state before manually repeating an interrupted task.

## Dependency audit (2026-11-07)

- golang.org/x/crypto v0.58.0, including ssh and ssh/knownhosts, fixed in go.mod
  and go.sum. Its go.mod requires Go 1.26; tested with project Go 1.27.3.
- golang.org/x/sys v0.49.0 is the indirect build dependency. x/term v0.47.0
  appears in go.sum for upstream tests. Licenses are BSD-3-Clause; full notices
  are included in docs/licenses for redistribution.
- Selection used `go get golang.org/x/crypto@latest` against the Go proxy.
  Upstream documentation: https://pkg.go.dev/golang.org/x/crypto/ssh and
  https://pkg.go.dev/golang.org/x/crypto/ssh/knownhosts .
- `GOTOOLCHAIN=go1.27.3 go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -show verbose ./...`:
  0 affected symbols, 0 affected imported packages. Module-only GO-2026-5932
  concerns the unimported openpgp package (https://pkg.go.dev/vuln/GO-2026-5932).
  The scanner reports a fix at v0.58.1-0.20261009181718-2f1f834330b2; the stable
  selected module is retained because this program does not use openpgp.
  The first tool invocation selected Go 1.26 and could not analyze Go 1.27 code;
  the recorded successful scan explicitly selected the project toolchain.

## Verification and remaining acceptance work

Automated tests use actual TCP/SSH protocol connections to an in-process Go SSH
server on Windows. They cover plaintext/encrypted keys, wrong/missing
passphrases, invalid key, unknown/changed trust, missing trust, bad public-key
authentication, fixed exec and zero PTY, ~2MiB Japanese byte-preserving transfer
with seven-byte reads, EOF, connection reuse after connect-context cancellation,
simultaneous oversized stdout/stderr, capture boundaries, nonzero exit,
exec rejection, handshake timeout, blocked stdin, output/wait timeout,
channel disconnection and concurrent repeated Close. Engine tests verify safe
public classifications and unreachable/failed/cancelled result rendering.

`go test ./...`, `go vet ./...`, CGO-disabled Windows amd64 build and
`git diff --check` passed. The race detector could not run: this environment
has CGO disabled by default and no gcc compiler when CGO is enabled.

These are transport protocol tests, not Windows OpenSSH/PowerShell acceptance.
V-05/06/11/18 remain unconfirmed on both Windows 11 and Server 2022 targets:
record OS/OpenSSH/PowerShell versions, source commit, exact commands, exit codes,
byte/EOF observations, no retries and released sessions/connections. Use an
isolated target with cmd.exe as DefaultShell, preverified host keys and a fixed
receiver wrapper; repeat bad authentication, unknown/changed keys, Japanese
split input, oversized dual output, send interruption and cancellation. Do not
use the unfinished product wrappers as evidence of successful module execution.
