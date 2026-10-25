# Third-party dependency licenses

This inventory covers the Go modules linked into the server commands built by
`scripts/build-server-bundle.ps1` and the dependencies of
`tools/provisioning-qr`. Server versions come from `server/go.mod`; the shipped
server set was checked against the module paths returned by
`go list -mod=readonly -deps ./cmd/...`. Provisioning QR versions come from
`tools/provisioning-qr/go.mod` and were checked against
`tools/provisioning-qr/vendor/modules.txt`.

The license and patent text files listed below were inspected in the local Go
module cache or vendored source tree. The bundle generator copies each shipped
server module's root `LICENSE`, `COPYING`, `NOTICE` and `PATENTS` files into
`licenses/<module>@<version>/` and records their SHA-256 hashes in `bundle.json`.
For the provisioning QR tool, preserve the vendored notice files alongside
distributed copies of the tool. Binary distributions must include the relevant
license text and copyright/disclaimer notices in documentation or other
materials accompanying the binaries.

## Server release bundle

| Dependency | Version | License identified | Source | Required notices and local files |
|---|---|---|---|---|
| `github.com/dustin/go-humanize` | `v1.1.0` | MIT | [Source](https://github.com/dustin/go-humanize) | Include copyright, permission terms and disclaimer from `LICENSE`. Local file present. |
| `github.com/mattn/go-isatty` | `v0.0.24` | MIT (Expat) | [Source](https://github.com/mattn/go-isatty) | Include copyright, permission terms and disclaimer from `LICENSE`. Local file present. |
| `github.com/ncruces/go-strftime` | `v1.1.0` | MIT | [Source](https://github.com/ncruces/go-strftime) | Include copyright, permission terms and disclaimer from `LICENSE`. Local file present. |
| `github.com/remyoudompheng/bigfft` | `v0.0.0-20230129092748-24d4a6f8daec` | BSD 3-Clause | [Source](https://github.com/remyoudompheng/bigfft) | Reproduce copyright, conditions and disclaimer in binary distribution materials from `LICENSE`. Local file present. |
| `golang.org/x/crypto` | `v0.58.0` | BSD 3-Clause | [Source](https://go.googlesource.com/crypto) | Reproduce copyright, conditions and disclaimer from `LICENSE`; `PATENTS` is also present and copied with the bundle. Local files present. |
| `golang.org/x/sys` | `v0.49.0` | BSD 3-Clause | [Source](https://go.googlesource.com/sys) | Reproduce copyright, conditions and disclaimer from `LICENSE`; `PATENTS` is also present and copied with the bundle. Local files present. |
| `modernc.org/libc` | `v1.79.0` | BSD 3-Clause | [Source](https://github.com/modernc-org/libc) | Reproduce copyright, conditions and disclaimer in binary distribution materials from `LICENSE`. Local file present. |
| `modernc.org/mathutil` | `v1.7.1` | BSD 3-Clause | [Source](https://github.com/modernc-org/mathutil) | Reproduce copyright, conditions and disclaimer in binary distribution materials from `LICENSE`. Local file present. |
| `modernc.org/memory` | `v1.12.1` | BSD 3-Clause | [Source](https://github.com/modernc-org/memory) | Reproduce copyright, conditions and disclaimer in binary distribution materials from `LICENSE`. Local file present. |
| `modernc.org/sqlite` | `v1.61.0` | BSD 3-Clause | [Source](https://github.com/modernc-org/sqlite) | Reproduce copyright, conditions and disclaimer in binary distribution materials from `LICENSE`. Local file present. |

## Provisioning QR tool

| Dependency | Version | License identified | Source | Required notices and local files |
|---|---|---|---|---|
| `github.com/makiuchi-d/gozxing` | `v0.1.1` | MIT; vendored license also contains the original ZXing Apache License 2.0 text | [Source](https://github.com/makiuchi-d/gozxing) | Distribute the complete vendored `LICENSE`, preserving the MIT notice and the included ZXing copyright and Apache license text. Local file present; no separate root `NOTICE` file found. |
| `github.com/skip2/go-qrcode` | `v0.0.0-20200617195104-da1b6568686e` | MIT | [Source](https://github.com/skip2/go-qrcode) | Include copyright, permission terms and disclaimer from `LICENSE`. Local file present. |
| `golang.org/x/text` | `v0.3.7` | BSD 3-Clause | [Source](https://go.googlesource.com/text) | Reproduce copyright, conditions and disclaimer from `LICENSE`; `PATENTS` is also present in vendor and should accompany the distribution. Local files present. |
| `golang.org/x/xerrors` | `v0.0.0-20200804184101-5ec99f83aff1` | BSD 3-Clause | [Source](https://go.googlesource.com/xerrors) | Reproduce copyright, conditions and disclaimer from `LICENSE`; `PATENTS` is also present in vendor and should accompany the distribution. Local files present. |

## Scope and verification limits

Modules present in `server/go.sum` but not in the resolved server command
dependency list are not included in the server table because the release
builder does not link them into its four shipped commands. The QR table includes
all modules recorded in its vendor manifest.

This is a file-based inventory, not legal advice or a legal opinion. License
identifiers and notice obligations were assessed from the pinned manifests and
the local license text; the upstream repositories and historical release tags
were not independently fetched or compared. No missing root license file was
found for the listed modules. This review does not establish whether additional
third-party components exist inside a particular Go toolchain, operating-system
runtime or future dependency release.
