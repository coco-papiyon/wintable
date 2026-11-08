# Issue 27 verification record

Executed on 2026-11-08 by Codex using PowerShell, Git, Docker CLI and Go.
Clean source under test: `1f9c7b2577aa58a3683dfa45931efd040679cf6e`.
Base: `b069641e0437fe38008979a449064b6065837853`.
This directory is an allowlisted copy of the final suite's evidence. It contains
no private keys or task inputs. The follow-up evidence commit changes no executable
fixture source; hashes in each manifest identify the exact bytes used.

| Verification | Result |
| --- | --- |
| `pwsh -NoProfile -File tests/linux-ssh/verify.ps1 -OutputDirectory .cache/linux-ssh/final-suite` | PASS, all eight scenarios |
| Clean rebuild (normal scenario, `--no-cache`) | PASS, complete package/Python/sshd inventory equals lock |
| Independent earlier clean rebuild | PASS, same dependency inventory (pre-final cancellation extensions) |
| Real Windows → loopback → Debian sshd L02 | PASS: five auth/trust rejections; hash/EOF/reuse/flood; two deadlines; explicit cancel/disconnect |
| Faults: partial-create/startup/test/ready/common-deadline | Expected exit 1, expected phase, cleanup and zero residuals |
| Simulated cleanup error after physical removal | Expected exit 1 and cleanup_ok=false; zero physical residuals |
| Wrong Python lock | Expected exit 1, runtime comparison rejects, cleanup succeeds |
| `go test ./...` | PASS (normal opt-in fixture skip expected) |
| `go vet ./...` | PASS |
| `./build.ps1` | PASS, distribution wrapper checks and Windows exe build |
| Final `go test ./internal/transport`, `go vet ./internal/transport` | PASS |
| Dedicated remote Docker + authenticated SSH tunnel | Not run: no dedicated remote host provisioned |
| Windows-container real SSH regression | Not run in this change; existing fixture and production transport are unchanged |
| Product Linux receiver/CLI L03–L06 | Not run; #28 scope |

Full-repository checks preceded the final additions to this test file. The final
transport checks and suite compiled/executed the completed test and runner.
Docker 29.9.2, Windows amd64 controller, Go 1.27.3 and target inventory/kernel
are recorded in manifests and `normal/versions.json`. All manifests' source
status is clean. `normal/test.jsonl` contains case-level expected/actual logs and
exit/capture/hash observations, `normal/starts` records exact probe starts.

All eight runs inspected their owned containers, networks, image tags and
credential directories after removal. An additional label-wide Docker query
after the suite found no `wintable.linux-ssh.run` containers/networks/images.
The isolated discovery image was also removed. Docker Desktop was switched back
to its original Windows-container mode after Linux verification.

The ready timeout injects an already-expired clock; the common timeout kills a
sleeping child with a one-second remaining budget. These verify the real timeout
branches and cleanup without waiting 60/600 seconds. Cleanup error injection
verifies failure reporting after physical resources are removed; it is not a
claim that every daemon/ACL failure can be recovered automatically.

This is #27 foundation acceptance for the local Linux-container mode. It does
not constitute Linux product support publication or #28/#29 acceptance.
