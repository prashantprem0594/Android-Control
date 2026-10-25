# Build log policy

Every shell command used to build, test, provision, migrate, package, sign, or deploy App Control must be recorded with its output.

## Required fields

Each text-log entry contains:

- Start and end time in India Standard Time.
- Command category.
- Working directory.
- Exact sanitized command.
- Complete sanitized standard output and standard error.
- Process exit code.
- Duration in milliseconds.
- SHA-256 digest of the sanitized entry body.
- Task or tracker ID used to group related commands.
- Exact model-token usage when supplied by the runtime.
- A clearly labeled logged-text token estimate when exact model usage is unavailable.

Logs are stored in `logs/build/YYYY-MM-DD.txt` and are append-only during normal development.

## Command wrapper

Use:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\run-logged.ps1 `
  -TaskId P1-002 `
  -Category test `
  -WorkingDirectory .\server `
  -Command "go test ./..."
```

Valid categories are:

- `build`
- `test`
- `tooling`
- `database`
- `android`
- `device`
- `release`
- `verification`
- `other`

## Token accounting

The current Codex runtime does not expose exact input/output token usage for an individual implementation task. The logger must therefore record:

```text
model_input_tokens: unavailable
model_output_tokens: unavailable
logged_text_estimated_tokens: <number>
token_estimation_method: ceil(sanitized command and output characters / 4)
```

`logged_text_estimated_tokens` measures only the command and its captured output. It is not presented as ChatGPT/Codex billing usage and does not include system instructions, conversation context, reasoning, or tool-call protocol overhead.

If exact runtime usage becomes available later, it will be stored separately as `model_input_tokens` and `model_output_tokens`; estimates will never be relabeled as exact counts.

Summarize estimates by task ID with:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\summarize-task-tokens.ps1
```

For actions that do not run through a shell, such as a structured file patch, call `write-build-log.ps1` immediately afterward and record the action and result.

## Secret handling

Logs must never contain:

- Passwords.
- Access or refresh tokens.
- Authorization bearer values.
- Private keys.
- Firebase service-account JSON.
- APK signing-key passwords.
- Database encryption keys.

The logger redacts common secret patterns, but prevention is the primary control: commands should refer to secret-file paths or environment-variable names and must never print their values. A command that intentionally displays a secret must not be run.

FCM service-account files, signing keys, and `.env` files are excluded from version control. Their contents are never valid test evidence.

## Evidence use

Tracker evidence should reference the dated log and, when useful, the entry digest. A phase cannot pass if its required build or device verification was performed without a corresponding log entry.


Regression coverage: scripts/test-logged-command-output.ps1 verifies console output, persisted output, recorded exit status and entry SHA-256 for success, a terminating exception and a native failure. Run it through run-logged.ps1 with task P6-004.

Windows PowerShell native stderr is captured without treating its first line as a terminating shell error. The native exit code determines success; real PowerShell errors still stop later statements. Regression coverage includes stderr with both zero and nonzero exits, complete persisted output/digests and fail-fast behavior.

Log files are UTF-8 without a BOM. Verification must explicitly decode UTF-8; Windows PowerShell default ANSI decoding produces false SHA-256 mismatches for non-ASCII text. The Unicode regression covers this case. The previously recorded 23 October exception was an encoding false positive: all original hashes match with correct decoding. Its original record is retained under retiredExceptions, with no active exception and no changed log bytes.

`verify-build-log.ps1 -Date YYYY-MM-DD` validates routine daily log integrity.
Use `-RequireRedactionProbe` when checking a log that includes the explicit
redaction probe; ordinary logs need not contain that probe's marker. Unredacted
probe text is rejected in both modes.
