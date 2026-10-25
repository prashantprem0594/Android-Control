# Operator-assisted parent password recovery

Recovery requires a verified server operator with local database access. The
operator must independently establish the parent's identity before changing
credentials. There is no unauthenticated HTTP recovery endpoint or email service.

1. Take and verify an encrypted backup using the existing administration workflow.
2. Ensure the deployed server/database has migration 14. The recovery executable
   requires an existing migrated database and never creates or migrates one.
3. Run the secure-prompt wrapper through the command logger:

   ```powershell
   & .\scripts\run-logged.ps1 -TaskId P5-001 -Category database -Command '& .\scripts\reset-parent-password.ps1 -Database "C:\PrivateDeployment\control.db"'
   ```

4. Enter the verified parent's email and new password in the credential prompt.
   The password follows the same 12-to-1024-byte policy as registration. Pass it
   privately to the parent; never include it in command arguments, logs or tickets.
5. Confirm login with the new password. Confirm old parent access/refresh tokens
   fail and enrolled active children still synchronize. Review the reset and
   revocation audit entries.

Build `server/cmd/parent-recovery` into `.cache/parent-recovery.exe` through the
logger, using the normal project Go caches, or supply an operator-controlled
executable path to the wrapper. The executable accepts only `-db` and bounded
UTF-8 JSON credentials through stdin. It prints a fixed success/failure summary;
neither credentials nor arbitrary storage errors are printed. The wrapper uses
a secure password prompt and UTF-8 stdin and creates no plaintext credential file.
Managed runtime strings exist transiently in memory; this is not a claim that
all process memory can be securely erased.

Recovery updates the password hash, revokes every unrevoked parent session,
expires unused pairing codes, revokes pending enrollments, clears login-rate
failure state and appends an immutable `parent_password_reset` event in one
transaction. Any audit/database failure rolls back the operation. Active child
enrollment, its keys, desired policies and command history are preserved. Other
parents are unaffected. A login verified before a reset rechecks the exact
password hash within its session-creation transaction, preventing stale password
verification from issuing a new session after recovery.

An already authorized parent request may have begun before revocation; password
recovery does not undo a previously committed command or policy. Review and
correct unwanted intent after recovering a compromised account. Separately
revoke a child if its key or physical custody is compromised.

Local regression tests cover access/refresh revocation, the new/old password
boundary, other-account isolation, pending/active enrollment behavior, audit
rollback, stale session issuance, private UTF-8 stdin and refusal to create a
missing database. Operational recovery and live Android reconnection remain
acceptance work.
