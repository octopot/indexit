---
code: IDX-6
id: I_kwDOSVi84s8AAAABSmBHNQ
databaseId: 5542790965
number: 105
url: https://github.com/octopot/indexit/issues/105
title: "ci/cd: keep one bounded release build cache on the default branch"
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
updatedAt: 2026-11-22T18:45:47Z
lastEditedAt: 2026-11-22T18:45:47Z
closedAt: null
---

# ci/cd: keep one bounded release build cache on the default branch

## Motivation

The release job builds four targets (`darwin`/`linux` × `amd64`/`arm64`) on a single runner. A cold cross-compile of this module peaks at about 5 GB of resident memory per target — `gotd` is a single generated package holding the whole Telegram API — so four at once overran the 16 GB hosted runner and the kernel killed the job without an error ([Continuous delivery, September 22, 2026](https://github.com/octopot/indexit/actions/runs/35754753213)). Two at once fit, and take about three minutes.

A build cache saved on `main` takes that time back: v0.4.1 restored one and finished the job in 2m21s, with 21 s of building binaries, against nine to ten minutes for a cold job ([Continuous delivery, November 8, 2026](https://github.com/octopot/indexit/actions/runs/42469808137)). It did so on a cache of an earlier `go.sum`: the build is warm whenever `gotd` and what it imports are unchanged, whatever the key says. An exact key cannot be promised on a tag anyway — a Go patch release, a failed warmup or the push that carries the tag changes it — and it is worth seconds.

The cost is the cache quota. Every push to `main` that changes the key saves another cache of about 2 GB and nothing removes the one it replaces. On November 22, 2026, after four such pushes in seven minutes, four release caches held 8.1 GB of the 10 GB quota, one of them usable, and GitHub evicted the caches of `ci` to make room. Each cache also carries the objects of the ones before it, and grew from 1.6 GB to 2.7 GB in four days.

## Verification PoC

```sh
gh run view <cd run id> --log | grep -E 'Cache (restored|hit|not found)|parallelism|took:'
gh cache list -R octopot/indexit --ref refs/heads/main --key release-
gh cache list -R octopot/indexit -L 100 --json ref -q '.[].ref' | sort | uniq -c
gh api repos/octopot/indexit/actions/cache/usage
```

Read the `cd` run in the month of its tag: the runs cleanup deletes it on day 2 of the next one.

## Acceptance criteria

- [x] a release tag restores a release build cache saved on `main`, and saves none;
- [x] when that cache was built with the same Go version and `gotd`, building binaries takes under 45 s;
- [ ] with any other cache, or none, the release still succeeds: it builds at most two targets at once, in under 5 min;
- [ ] an hour after the last push to `main`, `main` holds one release build cache, the one for its head, under 2.5 GB;
- [ ] that cache is still there after two weeks without a push;
- [ ] pushes to `main` in quick succession end with one finished warmup build, for the last of them;
- [ ] a day after dependency updates are merged, with no pull request open, cache usage is under 7 GB.

## Out of scope

- An exact cache key on every tag: it depends on Go releases, on GitHub's availability and on the order of pushes.
- Building more than two targets at once on a warm cache: it saves under 20 s.
- Caches of open pull requests and of `ci`, beyond the usage limit above.
- The download of GoReleaser from GitHub Releases, which failed v0.4.1 four times with 504: a separate issue.
- Splitting the build across runners (GoReleaser Pro `partial`, or a matrix of `--single-target` builds with a prebuilt shim): revisit only for a target a hosted Linux runner cannot build.
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

<!-- 2026-11-22T18:37Z https://github.com/octopot/indexit/issues/105#issuecomment-7083753548
**2026-11-22: the description is reworded around what GitHub's cache data shows.**

- Release speed is no longer the problem. v0.4.1 restored a cache from `main` by prefix and built the binaries in 21 s at `--parallelism 2`, and no cold release has run out of memory since 2026-09-22.
- An exact key on a tag cannot be promised. It changes with a Go patch release, a failed warmup, or the push that carries the tag, and lifting the parallelism limit on a hit saves under 20 s. The decision rule of the 2026-09-24 comment (exact key → close as not planned; otherwise reopen the split) no longer applies: the split and the exact key are out of scope.
- The open problem is the cache budget. Every push to `main` that changes the key saves another release cache of about 2 GB and nothing removes the one it replaces. On 2026-11-22, four of them held 8.1 GB of the 10 GB quota, one usable, and usage went over the quota.

The criteria now ask for one release cache on `main`, under 2.5 GB, a release that still builds on any cache, and usage under 7 GB after dependency merges. The issue closes as completed when they hold.
-->
