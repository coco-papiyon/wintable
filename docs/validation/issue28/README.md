# Issue #28 Linux command validation

Implementation under test: `3e9df567829a905f85556557853998616df10193`.
Base: `8ef31b73ef0c260a60febcad795d48946332f1e4`.
Controller: Windows amd64, Go 1.27.3. Agent: Codex; PowerShell, Go, Docker,
git and authenticated gh/gh api. Later evidence-only commits do not change
the executable, receiver or tests. The manifest's only untracked change is this
evidence directory; implementation and tests are committed at the SHA above.

| Layer | Evidence and result |
| --- | --- |
| L01 | `go test ./...` PASS. Existing config/backend/engine/CLI cases cover all-host preflight, OS selection, unsupported module, Windows preservation, argument count/byte/launch boundaries. Linux production selection replaces the prior unregistered-backend expectation. |
| L02 | `linux/test.jsonl`: independent probe over real SSH, authentication/host-key negatives, exact 2MiB length/hash/EOF, dual stream capture, one-connection reuse, blocked-send and post-EOF timeout/cancel/disconnect; PASS, no required skip. |
| L03 | `linux/product.jsonl`, `product-argv/output/exits`: Windows product exe through real sshd/receiver, independent length+hex argv probe (empty argv/elements, Japanese/supplementary Unicode, quotes, whitespace, newline, operators); binary stdout/stderr; nonzero and -9/-15 signals; JSON types/null and CLI 0/1; PASS. |
| L04 | Raw receiver tests: malformed UTF-8/BOM/JSON/duplicates/unknown keys/types/surrogates/NUL/deep/oversized input; OS/implementation/version shim rejection and no child marker; missing runtime produces unknown change. Product tests: ENOENT, EACCES, executable text ENOEXEC (no fallback), explicit shebang. E2BIG has both a raw receiver test with observed errno 7 and a Windows product CLI fixture variant verifying failed/false, null exit_code, subsequent skipped and CLI exit 1. All PASS. |
| L05 | Common engine/CLI unit tests cover forks/order/reuse, skipped, cancellation classifications and exit priority. Real product CLI timeout yields failed/null, subsequent skipped; real receiver Session deadline/cancel/close releases within five seconds; independent L02 probe covers send interruption and no replay. Product two-task runs exercise same-host sequencing/reuse. PASS. |
| L06 | Raw receiver synthetic request 2MiB -1/equal/+1, exe/argument 32KiB -1/equal/+1, NUL-inclusive total 64KiB -1/equal/+1, argv4096 -1/equal/+1. Product simultaneous binary streams each 1MiB -1/equal/+1 with exact retained prefixes and truncation. Decoder synthetic 3MiB -1/equal/+1 and malformed envelopes; launch synthetic 8KiB boundaries. Actual fixed launch 3474 bytes, checked by build and preflight. PASS. |
| Windows regression | `windows/command-test.log`, `windows/test.log`, `windows/manifest.json`: both modules through real Server Core 2022 OpenSSH, launch/EOF/disconnect, CRT/size/invalid inputs, file_content idempotence/ACL/link races, transport negatives/cancellation; PASS. |

`go vet ./...` and `./build.ps1` PASS at the implementation SHA. Local Windows
process tests require execution outside the filesystem sandbox: the first
sandbox run's file_content write failed, while the unrestricted complete run
passed. Normal opt-in SSH skips are distinct from the explicit fixture runs,
which require all mandatory cases and reject skips.

Reproduce with the commands in [Linux command](../../linux-command.md).
Linux fixture uses the fixed Debian/Python package lock; versions and comparison
are saved here. Product exe SHA-256 is in `linux/product-hash.json`.
Both disposable fixture runs completed cleanup successfully. Private keys,
credentials, task JSON and arbitrary command output are not included.

Limits: OS/Python mismatch uses a narrow raw-receiver shim. E2BIG uses an
instrumented raw receiver to observe errno; the product CLI variant temporarily
wraps the container's runtime launcher to constrain stack/environment, runs the
unchanged product receiver on actual CPython 3.13, then restores the launcher.
Transport send-phase
interruptions and engine fork/cancel priorities are tested in their own layers,
not mislabeled as full product CLI scenarios. Remote Docker/tunnel deployment,
other Linux distributions/architectures, Linux controller, Windows 11 SSH target,
full Windows Server target, Linux file_content and L07 are outside this record.
No claim that SSH interruption terminates remote descendants is made; fixture
container destruction performs cleanup. Support publication/CI remains #29.
