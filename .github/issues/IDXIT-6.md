---
code: IDXIT-6
id: I_kwDOSVi84s8AAAABSmBHNQ
databaseId: 5542790965
number: 105
url: https://github.com/octopot/indexit/issues/105
title: "ci/cd: start release builds from a warm cache on the default branch"
issueType: null
labels:
  - "type: improvement"
  - "scope: inventory"
  - "impact: medium"
  - "effort: medium"
assignees:
  - kamilsk
milestone: null
parent: null
issueFields: {}
state: OPEN
stateReason: null
duplicateOf: null
createdAt: 2026-09-22T17:12:29Z
updatedAt: 2026-10-18T07:42:08Z
lastEditedAt: 2026-10-18T07:42:08Z
closedAt: null
---

# ci/cd: start release builds from a warm cache on the default branch

## Motivation

The release job builds four targets (`darwin`/`linux` × `amd64`/`arm64`) on a single runner. A cold cross-compile of this module peaks at about 5 GB of resident memory — `gotd` is a single generated package holding the whole Telegram API — so four at once overran the 16 GB hosted runner and the kernel killed the job without an error ([Continuous delivery, September 22, 2026](https://github.com/octopot/indexit/actions/runs/35754753213)). A warm build cache needs about 0.6 GB per target and builds in seconds, so the job was meant to start warm and fall back to `--parallelism 2` only when cold.

It never starts warm. Every release since — v0.1.1, v0.1.2, v0.2.0, v0.2.1 — logged `Cache not found` even for the bare `release-Linux-` prefix, ran with `--parallelism 2`, and spent 3m29s (v0.2.1) building binaries in a job of about ten minutes. A tag restores only caches of its own ref and of the default branch, so what one release saves is out of reach of the next; nothing keeps a release cache on `main` between the monthly cleanup and a tag, and GitHub evicts a cache that nobody reads for seven days. The repository sits at 9.9 GB of its 10 GB cache quota, most of it caches of closed Dependabot pull requests.

Splitting the build across runners was the original plan here. Four green releases at `--parallelism 2` show the single runner is not the limit: the ceiling costs about three minutes, a warm cache takes them back without three extra jobs and a prebuilt shim, and GoReleaser's own split is a Pro feature.

## Verification PoC

```sh
gh run view <cd run id> --log | grep -E 'Cache (restored|not found)|BUILD_FLAGS|took:'
gh cache list -R octopot/indexit --ref refs/heads/main --key release-
gh api repos/octopot/indexit/actions/cache/usage
```

## Acceptance criteria

- [ ] `main` holds a release build cache after any push that changes `go.sum`, `tools/go.sum` or `.goreleaser.yml`, and it survives a quiet week;
- [ ] no cache is saved from a tag;
- [ ] on the next tag the restore hits the exact key, the parallelism limit is skipped, building binaries takes under 45 s, and there is no OOM;
- [ ] a miss, or a fallback to another Go version or dependency set, still builds with `--parallelism 2`;
- [ ] a closed pull request leaves no caches behind, and usage stays under 8 GB after a release;
- [ ] the runs of the caches cleanup and warmup on day 1 stay readable until the runs cleanup a day later.

## Out of scope

- Splitting the build across runners (GoReleaser Pro `partial`, or a matrix of `--single-target` builds with a prebuilt shim): revisit only for a target a hosted Linux runner cannot build.
- Reverting `--parallelism 2` as the cold fallback.
- The same ceiling in `make go-dist-check`, which is generated from `octomation/makefiles`.

<!-- 2026-09-24T15:56Z https://github.com/octopot/indexit/issues/105#issuecomment-5817549098
Status 2026-09-24: the split is parked; a warm build cache comes first.

Why: the release job almost always cross-compiled cold. setup-go restores only on an exact key (it hashed `go.sum` + `tools/go.sum`), warmup never produced that key, and the monthly cleanup drops everything. Locally, one target cold takes 21s and 4.3 GB RSS; warm with only `-X` ldflags changed, 1.6s and 0.6 GB. With a warm cache, four parallel targets fit the runner easily.

Pending commit:
- `cd.yml` restores `~/.cache/go-build` and `~/go/pkg/mod` with fallback keys and saves them after a successful run; `--parallelism 2` applies only when no cache matched at all.
- `warmup.caches.yml` builds all release targets with the same key.

Splitting across runners works with OSS GoReleaser (`--single-target` builds plus a prebuilt `tool:` shim in the release job), but it saves about a minute on top of the cache, at the cost of three jobs and a shim. Worth revisiting only for a target that a hosted Linux runner cannot build.

How to decide, on the next tag (baseline v0.1.0, run 35891180728: job 9m14s, "building binaries" 2m43s):
1. "Restore the release build cache" logs a matched key, and "Limit build parallelism on a cold cache" is skipped.
2. "building binaries" in the GoReleaser step takes under 30s with default parallelism, and no OOM.
3. After a cleanup, the warmup "Caching release builds" job succeeds and the next release still hits.

If all three hold, close this issue as not planned. If 2 fails with a matched key, reopen the split.
-->
