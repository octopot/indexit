---
code: IDXIT-78
id: I_kwDOSVi84s8AAAABOSBR_Q
number: 78
url: https://github.com/octopot/indexit/issues/78

title: "proxy: support MTProto proxies as a SOCKS5 replacement"
issueType: Feature
labels:
  - "type: feature"
  - "scope: code"
  - "impact: medium"
  - "effort: medium"
assignees: []
milestone: null
parent: null
issueFields: []

state: CLOSED
stateReason: COMPLETED
createdAt: 2026-08-26T04:36:39Z
updatedAt: 2026-08-26T04:53:34Z
closedAt: 2026-08-26T04:52:07Z
---

# proxy: support MTProto proxies as a SOCKS5 replacement

## Why

A SOCKS5 proxy to Telegram can be blocked at any time, so the tool needs a
second, independent channel; MTProto proxies are the practical one. The MTProto
transport is already wired into the client
(`dcs.MTProxy` in `internal/telegram/proxy/proxy.go`), but it is not usable with
what a user actually has in hand:

- the secret is accepted in hex only, while Telegram hands out base64url
  secrets inside proxy links;
- proxy links themselves (`tg://proxy?…`, `https://t.me/proxy?…`) are not
  understood, so every proxy has to be converted to host/port/secret by hand;
- an invalid secret is only rejected at connect time, by a raw `gotd` error.

## What changes

`INDEXIT_PROXY_*` accepts an MTProto proxy in the forms it is distributed in,
and rejects a broken one immediately, with a message that says what was wrong.

- Secrets in hex and base64 (raw and padded, URL and standard alphabets), in all
  three flavours: plain 16 bytes, `dd`-prefixed (secured), `ee`-prefixed
  (fakeTLS with a cloak host). Hex wins on ambiguity — a 32-character hex string
  is also valid base64, and reading it as base64 yields a different secret.
- `INDEXIT_PROXY_URL` accepts Telegram proxy-sharing links verbatim:
  `tg://proxy?server=…&port=…&secret=…`, `https://t.me/proxy?…` (also
  `telegram.me`, `telegram.dog`), and `tg://socks?server=…&port=…&user=…&pass=…`
  for SOCKS5. A plain `http://host:3128` stays an HTTP CONNECT proxy: only
  Telegram hosts are treated as links.
- The secret is validated at configuration time through `mtproxy.ParseSecret`,
  so a malformed one fails as a usage error before any connection is attempted.
- The proxy descriptor stays credential-free in logs and in `auth status`.

## Acceptance criteria

- [x] an MTProto secret is accepted in hex and in base64, in plain, `dd`, and
      `ee` forms;
- [x] a hex secret is never misread as base64;
- [x] `tg://proxy`, `https://t.me/proxy`, and `tg://socks` links are accepted as
      `INDEXIT_PROXY_URL`;
- [x] `http://host:port` keeps resolving to the HTTP CONNECT transport;
- [x] a malformed secret fails at configuration time with a usage exit code and
      a message naming the expected forms;
- [x] the secret never reaches logs or `auth status` output;
- [x] `.env.example` documents both secret encodings and the link forms;
- [x] unit tests cover the secret table and every accepted URL form.

## Verification

`indexit telegram auth status` with an MTProto proxy configured logs
`proxy type=mtproto host=… port=…` and connects through the proxy; the same
command with a malformed secret exits with the usage code and no connection
attempt.

## Notes

Out of scope: a proxy pool with failover between proxies (roadmap item 4.1).
