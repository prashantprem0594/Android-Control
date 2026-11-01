# Server binary bundles

<<<<<<< HEAD
Latest Linux preparation, 29 October 2026: `.cache/server-hosting-ece47e8caa4349f085e9e5b689470c01` independently verifies four Linux-amd64 tools, 142 source entries, ten module records and eleven license notices (`SERVER-HOSTING-BUNDLE`, logs/build/2026-10-29.txt). It includes the current parent wake-up sender hardening. See [Linux hosting templates](SERVER_HOSTING.md) for the separate service/proxy/backup assets. Cross-compilation and manifest verification do not establish native Linux runtime or live deployment.

The current 26 October local bundle is
`.cache/server-checkpoint-20261026-7963355f`: 8 Windows/Linux x64 binaries,
125 source inventory rows, 12 module records and 13 copied license notices.
The builder and independent verifier pass; see `logs/build/2026-10-26.txt`.
This is a build/metadata checkpoint, not deployment or Linux runtime evidence.

=======
>>>>>>> origin/main
`scripts/build-server-bundle.ps1` builds the server and matching administration,
parent recovery and release-metadata tools from one source snapshot. It exports
files only. It does not start a server, migrate a database, register a service or
copy deployment credentials.

From the workspace root, with dependencies already cached:

```powershell
& .\scripts\run-logged.ps1 -TaskId P7-001 -Category build -Command '& .\scripts\build-server-bundle.ps1 -OutputDirectory .cache/server-release-001'
```

Use a new output path for every build. Default targets are `windows-amd64` and `linux-amd64`;
`-Targets linux-arm64` selects ARM Linux. Existing output directories are refused.

The builder uses offline, read-only module resolution, verifies cached module
contents, disables CGO and workspace overrides, and passes `-trimpath` and
`-buildvcs=false`. It records actual per-target command dependencies, binary Go
build information, source hashes and binary sizes/SHA-256 hashes. Replaced modules
are refused. Concurrent source edits cause failure before `bundle.json` is written.
Incomplete directories must not be deployed. No cleanup or deletion is automatic.

Each bundle also contains the root `LICENSE`, `COPYING`, `NOTICE` and `PATENTS`
files found for every shipped Go module under `licenses/<module>@<version>/`.
Missing module license text fails bundle generation; the manifest records hashes
for the copied notices. See [third-party license review](THIRD_PARTY_LICENSES.md)
for the pinned server and provisioning QR dependency inventory.

<<<<<<< HEAD
Verify a completed bundle without executing its binaries or deploying it:

```powershell
& .\scripts\run-logged.ps1 -TaskId P7-VERIFY -Category verification -Command '& .\scripts\verify-server-bundle.ps1 -BundleDirectory .cache/server-release-001'
```

The verifier checks artifact sizes and SHA-256 hashes, the source-inventory and
module-list hashes, source-inventory entry structure, and every license-notice
hash. Source files themselves are not included in the bundle, so their recorded
per-file hashes cannot be recomputed by this verifier. It rejects missing files,
duplicate entries, invalid or traversal paths, paths outside the bundle and
reparse points in bundle paths. It verifies manifest metadata only; it does not
run bundled programs or assert their runtime safety.
Root and ancestor junctions are rejected too. Windows trailing-dot/space aliases
and reserved device names are refused, and metadata paths cannot be reused as
artifact or notice entries. The expanded fixture suite covers these cases;
cleanup checks its resolved workspace boundary and removes junction nodes before
recursive removal. The reviewed verifier also passes the generated local
`.cache/license-review-bundle` on 25 October; see `logs/build/2026-10-25.txt`.
Run its positive and tampered-bundle fixtures through the logging wrapper:

```powershell
& .\scripts\run-logged.ps1 -TaskId P7-VERIFY-TEST -Category test -Command '& .\scripts\test-server-bundle-verification.ps1'
```

=======
>>>>>>> origin/main
Only `cmd`, `internal`, `go.mod` and `go.sum` enter the source inventory; embedded
migrations are included. No source files, databases, keys or environment files
are copied. `bundle.json` is a completion record, not a signature or proof of
runtime acceptance. Keep its hash through a trusted distribution channel; an
attacker able to replace both binaries and the manifest can replace their hashes.

Compile success does not establish target OS compatibility, repeatable binary
hashes, TLS configuration or live migrations. Before deployment, use
the target machine's identity and private configuration, take a verified backup,
and perform the deployment/restore acceptance procedure. Cross-compiled Linux
binaries have not thereby been executed on Linux. The remote-call feature remains
disabled by default and requires the documented server and child opt-ins.

The four tools are built together to avoid using an old administration binary
against a schema introduced by a newer server. Keep this bundle intact when
selecting tools for [database recovery](DATABASE_RECOVERY.md) and
[backup scheduling](BACKUP_SCHEDULING.md). Existing scheduler bundles pin exact
tool hashes; export a reviewed replacement when changing the administration tool.
