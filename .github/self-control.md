# Self-control

Open checks that outlive a working session: things to look at later, decisions
that are waiting, and follow-ups that still need an issue. Each item says when
to look, what proves it and what to do if it fails. An item is removed once it
is proven or moved into an issue; a release adds the checks it leaves open
([AGENTS.md](../AGENTS.md#7-self-control-and-cleanup)).

## Watch

### Release build cache ([IDXIT-6](issues/IDXIT-6.md), #105)

`main` holds the release build cache; `cd` lifts `--parallelism 2` only on an
exact key. Proven so far: exact hits on the v0.2.2 and v0.3.0 runs, the warmup
rebuilds every cache, `cleanup.pulls.yml` leaves no `refs/pull/*/merge`
caches, and on 2026-11-08 three `release-Linux-go1.27.3-…` caches on `main`
were read the same day, with the scheduled warmup green on 2026-11-05.

- [ ] **On the next release tag**: the `cd` run restores the exact key, skips
  the parallelism limit, builds the binaries in under 45 s and saves no cache
  from the tag (the PoC in #105). On v0.4.1 the key missed for an outside
  reason: the three warmups on `main` that day failed to download GoReleaser
  (`/releases/download/` answered 504 on GitHub's side), so the tag restored
  an older cache by prefix and built with `--parallelism 2`, still in 21 s,
  and saved no cache from the tag. Before the next tag, check that the latest
  warmup on `main` is green; rerun it if not. If it still misses, look at what
  changed the key since the last push to `main`.
- [ ] **About 2026-11-15**:
  `gh api repos/octopot/indexit/actions/cache/usage` is under 8 GB. On
  2026-11-08 it was 8.59 GB in 14 caches, after five merges that day; the
  stale ones should age out within 7 days. If not, look at what keeps them.
- [ ] **On 2026-12-01/02**: the runs of the caches cleanup and the warmup of
  day 1 are readable until the runs cleanup on day 2. The window of November
  passed unchecked.
- [ ] All of the above hold → tick the acceptance criteria in #105 and close
  it as completed. If the Slack notifications of the warmup (every `go.sum`
  merge, twice a week) are noise, notify on failure only.

### Tools

- [ ] **When `golang.org/x/crypto` tags a release after v0.58.0**: bump it in
  `tools/` with `go get golang.org/x/crypto@<tag>` there. GO-2026-5932 is fixed
  only in an untagged commit, which moves `openpgp` into the module
  `golang.org/x/crypto/openpgp`, deprecated and with no fix: GoReleaser reaches
  it through ko and cosign, so after the bump it stays reported as
  `Fixed in: N/A`. GO-2026-6225 in
  `github.com/chrismellard/docker-credential-acr-env` has no fix; `cd`
  installs the tools but doesn't scan them. Proven when `tools` on `main` is
  green or fails only on advisories with `Fixed in: N/A`.

### Site

- [ ] **On the first Nextra release after 4.6.1**: move the `nextra-theme-docs`
  override in `docs/package.json` to Zod 4.6.6 or later, or drop it, and check
  the static build. The Dependabot alert on Zod is dismissed as a tolerable
  risk until then ([docs/README.md](../docs/README.md)). Proven when the site
  builds and `docs/README.md` says so.

## Follow-ups to file

- [ ] Parked by the owner on 2026-10-20, release first: saving a single
  Telegram post as a note, with its photographs served by `indexit service`.
  The indexit skill describes the post → note workflow it would build on; the
  tool that would use it lives outside this repository. File it when the owner
  picks it up.
