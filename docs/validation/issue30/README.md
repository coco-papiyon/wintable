# Issue #30 validation

Tested implementation: `04da351c0fd5b10aa58bfd3f6932555232a3a6db` (clean tree).
Base: `5322f2af8e31c557e5b97907a29e54611bba36f2`.
Controller: Windows amd64, Go 1.27.3. Agent: Codex (product version unknown).

`go test ./...`, `go vet ./...`, `./build.ps1` passed. Real-SSH tests require
fixture variables and skip in the ordinary unit run; their separate run is below.

## Linux real SSH

```powershell
pwsh -NoProfile -File tests/linux-ssh/run.ps1 -DockerContext desktop-linux `
  -ProductTestScript tests/linux-ssh/file-content.ps1 `
  -OutputDirectory .cache/issue30-linux-final
```

Passed without skips. `manifest.json` records clean source SHA, pinned image,
versions, status, and zero remaining fixture resources/keys. `file-content.jsonl`
records Linux command regression plus three file_content subtests:

- `raw-boundaries`: receiver 1MiB content and 2MiB JSON -1/equal/+1; malformed
  UTF-8/BOM/duplicate/unknown keys/surrogate/trailing JSON rejection.
- `API-cases`: exact source loaded over SSH as ordinary runner, test-only API
  interposition. New/equal/different/empty/Japanese/CRLF/NUL/1MiB; UID/GID/special
  mode preservation, mtime unchanged and audit-observed zero temp creation on
  equality; target/ancestor symlink, directory/FIFO/socket, missing parent,
  unsafe /tmp, ACL/xattr and permission rejection; target symlink swap at replace,
  no-replace destination race, commit/cleanup/fsync errors, ancestor rename
  rejection, foreign-temp preservation. Independent Python file reads/stat
  observe bytes/attributes/residue. Injected API failures are not physical disk
  failures or a proof against hostile root/same-UID actors.
- `product-bytes`: Windows product exe through real sshd/unchanged receiver;
  new empty → equal → Japanese/CRLF/NUL → 1MiB. Independent SHA256/mode/UID probe.

Product hash is in `file-content-product-hash.json`. The first exploratory run
used a previous exe because the build was still running and failed the product
case as unsupported; after build completion the second and final runs passed.
Only final evidence is retained here.

## Limits and remaining verification

See [design and trust boundary](../../linux-file-content.md). Only the pinned
Debian container filesystem is exercised. Physical power loss, all device types,
real foreign-UID ownership retention failure, default ACL/setgid combinations,
and file-specific commit-timed SSH disconnect are unverified. Generic Linux
command disconnect/timeout/cancel regressions pass, but do not prove file commit
phase timing. Forced receiver kill may leave a temp; fixture destruction reclaims
it. Hostile root/same-UID namespace mutation after final validation is outside the
trust contract. A full L07 completion/publication claim is withheld for these
remaining cases; implementing the backend is not a new CI publication approval.

## Windows real SSH regression

Same implementation SHA: Server Core 2022, default cmd shell. command-test.log records command, file_content, synchronized link swap, launch/EOF/disconnect tests PASS. test.log records authentication, transfer, output draining, connection reuse and timeout/cancel/disconnect PASS. windows/manifest.json records exit codes 0, container/image removal. The later evidence/sample/documentation commit does not alter receiver, executable source or tests.

