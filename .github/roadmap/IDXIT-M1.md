---
code: IDXIT-M1
id:
databaseId:
number:
url:
title: "Telegram fetcher PoC implementation"
state: OPEN
openIssueCount:
closedIssueCount:
dueOn:
createdAt:
updatedAt:
closedAt:
creator: kamilsk
tags:
  - type/milestone
  - topic/telegram
---

# milestone: Telegram fetcher PoC implementation

Read-only Telegram fetcher in the indexit CLI — dialogs, messages, topics, contacts, multi-account — delivered in four phases.

Milestone of [[IDXIT-P1]]. Tracks the four-phase PoC defined in
[[Telegram fetcher, PoC implementation plan]] (see its `PoC phases` section and
§12 milestones). Tasks below map 1:1 to the §12 milestone IDs.

## Progress

**7 / 17 tasks closed.** Phase 1 is functionally complete (build green,
`go test ./...` green); the remaining Phase-1 work is robustness/polish tracked
in `.github/reports/` and `.github/reviews/`. Phases 3–4 are not started.

| Phase | Scope                       | State       | Done  |
| ----- | --------------------------- | ----------- | ----- |
| 1     | Dialogs & messages          | in progress | 6 / 9 |
| 2     | Topics & single message     | in progress | 1 / 4 |
| 3     | Contacts & Stories          | not started | 0 / 2 |
| 4     | Multiple accounts & proxies | not started | 0 / 2 |

Legend: `[x]` done · `[ ] 🔄` in progress · `[ ]` not started.

### Phase 1 — Dialogs & messages

- [ ] 🔄 1.0 Repo/toolchain refresh — module path, Go baseline, root command done; **open:** GoReleaser still `tool`, CI calls missing `cd.dist.yml`, `viper` still linked (see `.github/reports/`)
- [x] 1.1 UID grammar (`uid.Parse`, `debug uid`)
- [x] 1.2 Auth & session (login/status/logout, proxy, device identity) — minor: `INDEXIT_LOG_LEVEL` not wired
- [x] 1.3 DialogsFetcher (no limit)
- [x] 1.4 Peer cache + numeric resolution
- [x] 1.5 MessagesFetcher (`--limit`/`--from`/`--to`/`--min-id`/`--max-id`)
- [x] 1.6 URL forms + cache-bound `t.me/c` handling
- [ ] 🔄 1.7 RateGuard robustness + contract tests — **open:** retry budget / transport backoff, `PEER_FLOOD` guidance
- [ ] 🔄 1.8 Polish: stderr summary, exit codes, `--help` — **open:** single summary line + `flood_waits`, verbose tiers, golden tests, README

### Phase 2 — Topics & single message

- [x] 2.0 Forum-topic history via `messages.getReplies`
- [ ] 2.1 Topic discovery — `fetch topics --dialog=<uid>` (`channels.getForumTopics`) — [[IDXIT-3]]
- [ ] 2.2 Single message — `fetch message <msg-uid>` (URL anchor applied)
- [ ] 2.3 Media download — `fetch media --dialog=<uid> --dir <path>` (`gotd` downloader) — [[IDXIT-4]]

### Phase 3 — Contacts & Stories

- [ ] 3.0 Contacts fetcher — `fetch contacts`, `--dialogs=<uid>`
- [ ] 3.1 Story viewers — `fetch contacts --stories[=<story-uid>]` (`stories.getStoryViewsList`)

### Phase 4 — Multiple accounts & proxies

- [ ] 4.0 Multi-account — multiple session files, selectable per invocation
- [ ] 4.1 Multi-proxy — proxy pool with failover between proxies

## Known gaps (Phase 1 robustness/polish)

Functional behaviour is in place; these are tracked, not blocking:

- Spec-compliance: `.github/reports/` (e.g. `INDEXIT_LOG_LEVEL`, verbose tiers, `flood_waits` counter, GoReleaser identity, `viper`, `PEER_FLOOD` guidance).
- Code quality / bugs: `.github/reviews/` (e.g. logger mutex copy, pagination guards, RateGuard retry budget, append/atomicity, golden/mapper test coverage).
