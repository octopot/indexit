# Self-control

Open checks that outlive a working session: things to look at later, decisions
that are waiting, and follow-ups that still need an issue. Each item says when
to look, what proves it and what to do if it fails. An item is removed once it
is proven or moved into an issue; a release adds the checks it leaves open
([AGENTS.md](../AGENTS.md#7-self-control-and-cleanup)).

## Watch

### Release build cache ([IDX-6](issues/IDX-6.md), #105)

The issue now asks for one bounded release cache on `main`, not an exact key
on a tag (2026-11-22). Proven so far: v0.4.1 restored a cache from `main` and
built the binaries in 21 s at `--parallelism 2`, and no cold release ran out of
memory since 2026-09-22. On 2026-11-22 cache usage went over the 10 GB quota:
four release caches of about 2 GB each on `main` after four pushes in seven
minutes, one of them usable. v0.4.2 restored the exact key from `main`, skipped
the parallelism limit, built the binaries in about 2 s and saved no cache
(`cd` run 45056412164); usage stood at 10.1 GB in 8 caches right after it,
four of them release caches on `main`.

- [ ] **An hour after a push to `main` and a day after dependency merges**:
  `gh cache list --ref refs/heads/main --key release-` shows one cache, and
  `gh api repos/octopot/indexit/actions/cache/usage` is under 7 GB with no pull
  request open. Until #105 bounds the cache, a miss here is expected; record the
  numbers in the issue.
- [ ] All the criteria of #105 hold → tick them and close it as completed. If
  the Slack notifications of the warmup (every `go.sum` merge, twice a week)
  are noise, notify on failure only: five warmup runs on 2026-11-22 make the
  case.
- [ ] **File an issue**: `cd` downloads GoReleaser from GitHub Releases on every
  run, and v0.4.1 needed five attempts over 2h25m because the download
  answered 504, although the tools module pins the same v2.18.2.

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
