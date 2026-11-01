# Telecom feasibility prototype

<<<<<<< HEAD
## Already-current output feedback - 25 October 2026

The synthetic `endpoint-feedback-v1` check reproduced misleading unconfirmed
feedback when Speaker was already current. Modern endpoint requests now accept
that exact already-current choice immediately when no other request is pending;
pending selections still use Telecom. Child full lint and debug/test/release
assembly pass. Runtime shortcut and picker checks observe Speaker and clear
feedback, with fixture/original-dialer cleanup. This AVD has no earpiece, and no
physical sound, wired/Bluetooth or Android 13 route result is claimed. See
`scripts/test-android-telecom-audio.ps1` and `logs/build/2026-10-25.txt`.

## Child-process failure recovery - 25 October 2026

When the child has no live call entry, its call screen offers **Open Android
call controls**. An explicit local tap checks phone permission and delegates to
Telecom's system call screen. If Android still has a call, the screen says that
Android is handling it. No command or automatic answer/end action is added.

`scripts/test-android-telecom-recovery.ps1` passes `process-recovery-v1` on the
isolated unenrolled Android 37 Device Owner AVD. It arms one synthetic SIP call,
keeps the instrumentation alive, SIGKILLs its exact child PID and observes a
live preloaded in-call service binding. Old local/remote IDs cannot affect that
call. The new button opens the actual preloaded UI; its End control disconnects
the fixture. After complete call teardown, explicit normal role requalification
lets the next call reach the child with a fresh ID and local rejection works.
Cleanup unregisters the synthetic account and restores the original dialer.

This is `preloaded-ui-and-explicit-next-call-requalification`, not automatic
child rebind or mid-call handoff. Neither of those was observed in the earlier
attempts. Users can finish the ongoing call with Android controls; requalifying
the child as the phone app is a separate local recovery step after that call.
Actual SIM, sound, locked-screen, dual-SIM and Redmi/HyperOS behavior remain open.
Logs retain failed harness assumptions and successful reruns in
`logs/build/2026-10-25.txt`. Current child release lint/assembly and the ordinary
`ui-controls-v1` synthetic regression pass after this change.

=======
>>>>>>> origin/main
The child now contains a default-phone-role prototype. It is not a completed
replacement dialer or a verified remote-call product. The backend can queue
short-lived answer, reject and end commands for an exact child call ID, but
physical execution and enrolled-device behavior are not verified. No real calls
were placed during development checks.

## Implemented components

- **Set up phone role (feasibility)** requests Android's dialer role explicitly.
  Phone, phone-state and notification permissions are requested separately.
- `DialActivity` handles ACTION_DIAL and provides a keypad. It never calls
  automatically from an incoming intent. Calls require a user press and use
  TelecomManager.placeCall. A system-phone button opens the preloaded dialer.
- `ManagedInCallService` uses the system binding with BIND_INCALL_SERVICE and
  exported=true. It maintains process-local live call IDs and callbacks.
- Incoming/ongoing CallStyle notifications open a call screen that can appear
  over the lock screen. Android 14+ full-screen alert access has an explicit
  setup button; permission-denied behavior falls back to notifications.
- The call screen shows the selected call and state, answer audio/reject/end,
  mute, hold/resume, DTMF and calling-account selection when supported. Multiple
  calls have distinct controls. Rotation preserves only the opaque selected ID;
  a stale ID does not silently target another call.
- Android 13 speaker routing uses the legacy audio API. Android 14+ chooses
  from the available call endpoints and requests an endpoint change. Labels
  update from observed callbacks, not simply from a request returning.

The implementation follows the public
[InCallService/default-dialer contract](https://developer.android.com/reference/android/telecom/InCallService).
Telecom retains ringing responsibility in this prototype. Outgoing calls use
the platform path, including emergency calls; the app avoids placing its own
call screen over confirmed or unknown emergency classification. These choices
still require the carrier/device test procedure below; no emergency guarantee
is inferred from compilation.

## Action validation and data handling

`CallActionGate` validates an exact live identity, a deadline no more than 30
seconds away, and an appropriate state. Answer/reject require ringing; remote
end requires active/held. Expired, removed, mismatched or disconnected targets
are rejected. Internal remote feasibility actions also reject emergency or
unclassified protected calls. Local user controls retain the ability to end
their own call and cancel dialing/account selection.

The result `REQUESTED` means only that a Telecom operation was submitted. It is
not proof of answer/disconnect or speaker output. The authenticated command
worker dispatches the exact action on the main thread and waits for the
requested call-state observation before reporting success. Telecom request
submission alone is reported as failure if no result is observed before expiry.
The parent foreground **Foreground calls** screen reads a bounded authorized
snapshot of opaque call IDs and states; snapshots are published during child
sync and expire for queueing after 30 seconds. The parent UI offers
answer/reject for ringing calls and end for active/held calls. This workflow
does not add remote actions to incoming-call notifications.

Caller display information stays in process memory and on screen/notifications.
The prototype does not persist numbers, add app call history, log call content,
record audio or stream media. Process/service recreation creates new call IDs,
so old requests cannot retarget newly rebound calls. Standard Android system
call records are outside this app's storage. Contacts/history, video UI,
conference management and production multi-SIM behavior remain incomplete.

## Verification

`scripts/test-call-actions.ps1` runs dependency-free JVM checks of live identity,
stale/missing/disconnected targets, invalid states, exact expiry, deadline
bounds, remote emergency rejection, retained local user control, provider
failure and request/outcome separation. Both APKs build with the installed
SDK. Android 13 routing necessarily retains APIs deprecated on Android 14.

Physical acceptance remains open because no device call-result evidence has
been collected.
On the designated test Redmi, record model, OS/HyperOS build, SIM/carrier,
role/permission state and sanitized results through the logging wrapper:

1. Select the prototype's phone role and permissions. Place normal incoming
   and outgoing SIM calls and verify ringtone, notification and visible state.
2. Test answer/reject/end, hold, DTMF, account selection and multiple calls,
   including a call ending while a control is displayed and activity rotation.
3. Repeat incoming calls with the screen locked and with notification/full-screen
   permissions denied/restored; verify that required UI remains available.
4. Verify actual speaker/earpiece output on Android 13 and Android 14+, then
   wired/Bluetooth routing, rather than relying only on endpoint labels.
5. Kill/rebind the dialer process during controlled normal calls and verify the
   platform's fallback/recovery and that old IDs/actions cannot affect a new call.
6. Exercise the approved carrier/device non-emergency test procedure for the
   system emergency path. Do not place an unnecessary emergency call.
7. Combine call tests with device lock, package blocks, internet pause, network
   changes and reboot before approving management/call coexistence.

Restore the preloaded phone app through Android's Default apps settings after
the experiment. Production phone UI and hardware acceptance remain open. See
the [remote-call API](REMOTE_CALL_API.md) for the queued command contract.

## Android role and empty dial-pad evidence (24 October 2026)

scripts/test-android-dialer-role.ps1 passed on the isolated unenrolled Android 37 AVD. Android accepted the child as ROLE_DIALER through normal qualification, and Telecom reported it as the default. Both ACTION_DIAL with no data and with an empty tel scheme launched the child DialActivity. UI hierarchy checks found the phone header, keypad, Call and Delete controls. No number was supplied and no Call button was pressed. The script never bypasses qualification, registers phone accounts or places calls. Its finally block restored the original role holder; a separate logged read confirmed that the role and Telecom default agree afterward. Physical incoming SIM, locked-screen call UI, audio routing, emergency and crash-recovery acceptance remain open.

Emergency classification correction (24 October 2026): missing handles previously evaluated as non-emergency through a nullable comparison. EmergencyCallGuard now protects missing/blank/non-telephone handles, missing classifiers, platform emergency flags and classification failures from internal remote actions. JVM tests prove zero remote action for unknown classification and retain local user control. APK/physical emergency-path evidence is separate; no emergency call was placed.

Synthetic Telecom core evidence (24 October 2026): an isolated debug-only SIP ConnectionService fixture with no network/audio permissions supplied controlled incoming calls to Android Telecom. The production child InCallService observed ringing, active and removed calls; local answer/end/reject and fresh call IDs passed. Internal remote SIP actions, expired/deadline-invalid/missing targets and stale IDs were rejected. The fixed managed account was unregistered and the original default dialer/Telecom role restored. No SIM/outgoing/emergency call or media stream was used. Expanded screen-button verification subsequently passed with mandatory ui-controls-v1 proof for Answer audio, Hold call, Resume, Mute/Unmute, End and next-call Reject, including the expected Telecom callbacks and screen states. Earlier failed compilation and stale-APK core results are preserved and excluded from this UI proof. Real SIM, actual sound, locked-screen, emergency and Redmi coverage remain open.

## Local audio-output selection - 24 October 2026

Parent-configured contact shortcuts and Call Parent are described in
[CALL_CONTACTS.md](CALL_CONTACTS.md). They fill the child dial pad for explicit
local review and calling; they do not enforce an outgoing-call allowlist.

The active-call screen now has an Audio output picker for currently available earpiece, speaker, wired and Bluetooth outputs. Android 14+ selects exact Telecom CallEndpoint identities; Android 13 uses supported route masks and specific currently supported Bluetooth devices. Bluetooth access is requested only after the user chooses it on the legacy path; no scanning/location permission is requested. Hardware support is optional. Bluetooth names are displayed locally only, never persisted or sent through the call API.

Requests recheck the exact active call, current service and available choice. Device removal/replacement makes a stale selection fail. Request feedback is distinct from observed audio status; only Telecom endpoint/audio callbacks update the observed route, and specific legacy Bluetooth confirmation checks the active device. Late errors/timeouts cannot overwrite feedback for a newer request. Feedback is scoped to its call and cleared on hold/removal/service destruction. Both Speaker and Phone audio shortcuts remain available.

The child debug APK compiles with these changes, including the final optional-Bluetooth manifest declaration. No new audio/call/device test was initiated under the feature-building priority. Actual earpiece, speaker, wired/Bluetooth sound, routing delays and Redmi/API-version acceptance remain open. Platform reference: https://developer.android.com/reference/android/telecom/InCallService .
