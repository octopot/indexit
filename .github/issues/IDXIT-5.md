---
code: IDXIT-5
id: I_kwDOSVi84s8AAAABR_1hCw
databaseId: 5502755083
number: 102
url: https://github.com/octopot/indexit/issues/102
title: "ci/cd: restore HTTPS and Go module discovery for go.octolab.org"
labels:
  - "type: bug"
  - "severity: major"
  - "scope: deps"
  - "impact: high"
milestone:
state: CLOSED
stateReason: COMPLETED
createdAt: 2026-09-18T17:43:51Z
updatedAt: 2026-09-24T16:39:33Z
lastEditedAt: 2026-09-24T15:56:26Z
closedAt: 2026-09-24T16:39:33Z
---

# ci/cd: restore HTTPS and Go module discovery for go.octolab.org

## Motivation

Fresh CI runners must be able to resolve and download public `go.octolab.org`
modules directly over verified HTTPS. Dependency installation must not depend
on a warm cache or on a module proxy retaining previously published versions.

Failure reference: [Continuous integration, September 18, 2026](https://github.com/octopot/indexit/actions/runs/35373271231).

```text
go.octolab.org/toolkit/cli@v0.6.3 requires
go.octolab.org@v0.12.0: unrecognized import path "go.octolab.org":
tls: failed to verify certificate: x509: certificate is valid for
*.github.com, *.github.io, *.githubusercontent.com, github.com,
github.io, githubusercontent.com, not go.octolab.org
```

## Acceptance criteria

- [x] HTTPS serves a trusted certificate valid for `go.octolab.org`.
- [x] Requests with `?go-get=1` at `/`, `/toolkit/cli`, and `/toolkit/config`
      return valid `go-import` metadata pointing to the intended repositories.
- [x] Direct downloads of the versions below succeed with an empty module cache
      and normal TLS and checksum verification enabled. `toolkit/config@v0.0.4`
      will not be restored: its tag is lost upstream, so the criterion is met
      by moving to `v0.0.5`, which the project already requires.
- [ ] CI dependency setup and the Go dependency jobs in cache warmup pass on
      both configured Go versions without the certificate/discovery error.
- [ ] Record the verification commands and results in an issue comment.

## Verification PoC

Check HTTPS and inspect the discovery metadata:

```sh
curl --fail --show-error --location 'https://go.octolab.org/?go-get=1'
curl --fail --show-error --location 'https://go.octolab.org/toolkit/cli?go-get=1'
curl --fail --show-error --location 'https://go.octolab.org/toolkit/config?go-get=1'
```

Download into a fresh temporary cache with proxy access disabled:

```sh
GOMODCACHE="$(mktemp -d)" GOPROXY=direct GOINSECURE= GOSUMDB=sum.golang.org \
  go mod download -json \
  go.octolab.org@v0.12.0 \
  go.octolab.org@v0.12.2 \
  go.octolab.org/toolkit/cli@v0.6.3 \
  go.octolab.org/toolkit/config@v0.0.5
```

## Out of scope

- Changing the project's proxy policy or bypassing TLS verification.
- Repairing the independent `github.com/caarlos0/go-shellwords@v1.0.12`
  download failure in tools installation.
- Upgrading Go tools or application dependencies, or requiring unrelated
  workflow stages to pass before accepting domain recovery.

<!-- 2026-09-24T15:39Z https://github.com/octopot/indexit/issues/102#issuecomment-5817271097
Status 2026-09-24.

Cause: GitHub Pages served its default `*.github.io` certificate for `go.octolab.org`; the custom domain and HTTPS were restored outside this repository (Pages settings re-checked). Current certificate: Let's Encrypt, `CN=go.octolab.org`, valid until 2026-12-18.

Verified locally:
- `curl ... ?go-get=1` for `/`, `/toolkit/cli`, `/toolkit/config`: HTTP 200 (the two subpaths after a 301 to a trailing slash), `go-import` points to `github.com/octolab/pkg`, `github.com/octolab/cli`, `github.com/octolab/config`.
- `GOMODCACHE=$(mktemp -d) GOPROXY=direct GOSUMDB=sum.golang.org go mod download`: `go.octolab.org@v0.12.0`, `@v0.12.2`, `toolkit/cli@v0.6.3` succeed; a full download of every module in `go.mod` succeeds too.
- `toolkit/config@v0.0.4` is dropped from the criteria: its tag is lost in `octolab/config` and will not come back; the project uses `v0.0.5`.

Prevention (pending commit): `node .github/scripts/release.mjs vanity` checks every `go.octolab.org` module in `go.mod` and `tools/go.mod` over verified HTTPS, and the daily `doctor` runs it.

Left: land that change, then re-run Continuous integration and the cache warmup on both Go versions.
-->
