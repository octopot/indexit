---
code: IDXIT-79
id: I_kwDOSVi84s8AAAABOSEa6Q
number: 79
url: https://github.com/octopot/indexit/issues/79

title: "auth: QR login as a working path when phone codes are not delivered"
issueType: Feature
labels:
  - "type: feature"
  - "scope: code"
  - "impact: high"
  - "effort: medium"
assignees: []
milestone: null
parent: null
issueFields: []

state: CLOSED
stateReason: COMPLETED
createdAt: 2026-08-26T04:46:42Z
updatedAt: 2026-08-26T04:47:43Z
closedAt: 2026-08-26T04:46:52Z
---

# auth: QR login as a working path when phone codes are not delivered

## Why

Phone-code login stopped being a usable path. `auth.sendCode` returns
`sentCodeTypeApp` **successfully**, but Telegram delivers the code nowhere — not
to the service chat, not to the account's Login email — and a resend fails with
`SEND_CODE_UNAVAILABLE`. Verbose logging showed a correct request: right DC,
clean handshake, no `rpc_error`. Official clients on the same account keep
receiving codes, so the channel is alive and the suppression is server-side, tied
to the third-party `api_id` and/or the proxy IP — nothing the tool can fix from
its side.

Without a second, independent way in, an expired session leaves the tool locked
out with no recourse.

## What changes

`indexit telegram auth login --qr` authorizes through Telegram's QR login flow,
a different trust path: an already-authorized device approves the login, so code
delivery is out of the picture entirely.

- A login token is rendered as a scannable QR in the terminal, with the
  Settings → Devices → Link Desktop Device instruction and the token's expiry;
  the token is re-exported and redrawn automatically when it expires.
- The QR is drawn with Unicode half-blocks on a forced white-on-black field, so
  polarity stays scannable regardless of the terminal theme.
- The QR client is built with updates enabled — the confirmation arrives as
  `updateLoginToken`.
- Accepting the QR on an account with a cloud password returns
  `SESSION_PASSWORD_NEEDED`; the 2FA password is then prompted for and completes
  the authorization.
- The code prompt offers QR as an escape hatch: when Telegram has no other
  delivery channel left, the prompt says so and accepts `qr` to switch flows
  instead of waiting for a code that will not arrive.
- Resend is offered only when Telegram actually names a next delivery channel,
  instead of implying a retry that does nothing.

## Acceptance criteria

- [x] `auth login --qr` authorizes a session without a phone code;
- [x] the QR is scannable from a phone in both light and dark terminals;
- [x] an expired token is re-issued and redrawn without restarting the command;
- [x] an account with 2FA completes through the password prompt after QR
      acceptance;
- [x] the code prompt states when no delivery channel is left and offers `qr`;
- [x] resend is only offered when a next channel exists;
- [x] the rendered QR is covered by a test that reconstructs the module grid.

## Verification

A live session was authorized through the QR flow with 2FA on 2026-06-21, after
the phone-code path had failed repeatedly on the same account. `TestRenderQR`
reconstructs the module grid from the half-block output and compares it against
the source bitmap, guarding polarity and the two-rows-per-line packing.

## Notes

Filed retrospectively: the work was done on 2026-06-21 and shipped in `7c56d0f`
(`internal/telegram/qr.go`, `internal/command/telegram/qr.go`, and the QR
fallback in `internal/command/telegram/auth.go`). The issue records the context
and the acceptance criteria the implementation already meets.

Do not re-debug code delivery (fresh `api_id`, email, service chat, phone format)
— it was ruled out; QR is the working path.
