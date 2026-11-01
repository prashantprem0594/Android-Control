# App build history

Controlled parent and child builds now automatically preserve their verified APK snapshots under `releases/app-history/`. This local folder is ignored by Git; keep a separate backup if it must survive loss of the workspace.

The layout is `releases/app-history/{parent,child}/{versionCode-versionName}/{UTCtimestamp-fullSHA256}/`. Each entry contains its APK and `archive.json`, an allowlisted public inventory of package, variant, version, minimum SDK, byte count and SHA-256. Public build context includes source scope, fixture flag, HTTPS server origin, development-CA use and `productionReady=false`, so development/fixture artifacts are identifiable. Firebase settings and private configuration are excluded. Files are marked read-only. The archiver never overwrites a history entry: identical version/content is verified and reused, while changed bytes get a new entry even when the version number stays the same. A changed/incomplete existing entry causes a visible failure and is preserved for review.

The initial six archived entries predate the context fields. They retain their original immutable `archive.json` and receive an exclusive, read-only `build-context.json` sidecar on verified reuse. Subsequent reuse validates that context too; no existing inventory or APK is overwritten.

`scripts/build-wakeup-test-apks.ps1` invokes the archiver only after its APK verification and `wakeup-build.json` publication. This also covers builds launched through `build-laptop-test-apks.ps1`. Direct Gradle builds do not create a verified snapshot or automatically enter this history.

To archive a verified snapshot independently, run:

```powershell
& .\scripts\archive-app-builds.ps1 -SnapshotDirectory data/laptop-apks-ui-final-20261030 -VersionName 0.1.0
```

New reports include the version name; `-VersionName` is required for older reports that lack it. When both are provided they must match. Input hashes, bounded metadata, fixed parent/child filenames and package identities are validated before copying. Traversal/alias/reparse-point paths are refused; an exclusive archive lock prevents overlapping writers.

On 30 October 2026 the three available configured snapshots (`data/laptop-apks-001`, `data/laptop-apks-ui-20261030`, and `data/laptop-apks-ui-final-20261030`) were archived separately for both apps as version 1 / 0.1.0. The focused archive tests cover exact-repeat reuse, changed bytes at the same version, a new version, differing historical/future minimum SDKs, immutable legacy context sidecars, hash/metadata/version/context mismatches, unsafe paths, junctions and damaged history. Evidence is in `APP-HISTORY-TESTS`, `APP-HISTORY-EXISTING` and `APP-HISTORY-VERIFY` in `logs/build/2026-10-30.txt`.

This preserves available build artifacts. It does not reconstruct earlier APKs that no longer exist, prove installation, or turn development debug APKs into production releases.
