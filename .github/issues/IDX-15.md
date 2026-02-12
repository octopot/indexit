---
code: IDX-15
id: I_kwDOSVi84s8AAAABhiR1yQ
databaseId: 6545503689
number: 148
url: https://github.com/octopot/indexit/issues/148
title: "test: make the dashboard and stats tests independent of the clock and the disk"
issueType: null
labels:
  - "effort: medium"
  - "impact: medium"
  - "scope: code"
  - "type: bug"
assignees:
  - kamilsk
milestone: null
parent: null
issueFields: {}
state: OPEN
stateReason: null
duplicateOf: null
createdAt: 2026-11-22T17:33:28Z
updatedAt: 2026-11-22T17:33:28Z
lastEditedAt: null
closedAt: null
---

# test: make the dashboard and stats tests independent of the clock and the disk

## Motivation

`TestAPI` failed on `main` for 38a47e9 ([Continuous integration, November 22, 2026](https://github.com/octopot/indexit/actions/runs/45042249669)). It checks that the access hash of the fixture peer, `555`, never reaches `/api/v1/stats`, by looking for the three digits anywhere in the response. The response also carries the time of the scan with nanoseconds, the modification times of the fixture files, which the fixture takes from the wall clock, and the free space of the real disk. Their digits held `555` (`"scanned_at": "…19.445552935Z"`): the test failed about once in a hundred runs, and every run while the disk's free space held the digits.

The test is not wrong about what it protects. The dashboard reads the clock and the disk itself, and nothing in a test can fix them, so the response is never the same twice and an assertion on it can only be loose. The statistics reader already takes its clock as a parameter, but the dashboard cannot give it one, and the tests of `internal/stats` leave it on the wall clock: their October 2026 logs fall out of the twelve months the reader keeps, and the tests start failing in November 2027. `TestStatsErrorIsShown` computes the current month twice and fails when a month ends between the two.

## Verification PoC

```sh
go test -count=2000 -run 'TestAPI$' ./internal/dashboard/
go test -count=1 ./internal/stats/   # with the clock moved past October 2027
```

## Acceptance criteria

- [x] the dashboard reads the time and the free disk space through its own sources, which a test can replace; the service behaves as before;
- [x] the dashboard tests run on a fixed time and a fixed free space, and the API response of the fixture is the same in every run;
- [x] the access hash cannot leak unnoticed: a response with another access hash in the peer cache is identical, and the fixture's hash is a value no other field can contain by chance;
- [x] the statistics tests do not depend on today's date, and no test computes the current month twice;
- [x] `go test -count=2000 -run 'TestAPI$' ./internal/dashboard/` passes.

## Out of scope

- Tests that assume they do not run as root (`chmod 0`, read-only directories), depend on `HOME`, or pick network addresses and ports: another class of environment dependency.
- Separating what the dashboard collects from how it renders it.
