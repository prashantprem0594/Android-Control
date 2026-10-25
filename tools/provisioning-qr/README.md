# Provisioning QR utility

This standalone Go module renders the public managed-provisioning JSON as a
PNG QR code, then decodes the generated image with an independent QR decoder
and requires the decoded bytes to exactly match the source payload.

Build using only the checked-in vendor tree:

```powershell
$env:GOPROXY = 'off'
go build -mod=vendor ./...
```

Run it with an already verified payload from
`scripts/new-managed-provisioning-payload.ps1`:

```powershell
.\provisioning-qr.exe -input payload.json -output provisioning.png
```

The input is public setup metadata. The utility cannot reset, enroll, or
provision a device and does not use ADB. It does not alter the server module.
Dependencies are pinned in `go.mod`, vendored here, and their upstream license
files are included in `vendor`.
