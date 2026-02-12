---
code: IDX-P1
id: null
fullDatabaseId: null
number: null
url: null
owner:
  id: null
  login: "octopot"
  type: Organization
title: "Telegram integration"
shortDescription: "Read-only personal-data ingestion from Telegram into the Sparkle index — dialogs, messages, topics, contacts, multi-account."
public: true
closed: false
createdAt: null
updatedAt: null
closedAt: null
---

# Telegram integration

Bring Telegram into the `indexit` ingest pipeline as a first-class, read-only source for the Sparkle index. The `indexit telegram` subcommand streams the user's own Telegram data (dialogs, message history, forum topics, contacts, and Story viewers) as stable JSONL that a downstream Sparkle indexer can consume without re-fetching.

Design and scope live in the spec: [[Telegram fetcher, PoC implementation plan]].

Milestones:

- [[IDX-M1]] — Telegram fetcher PoC implementation: open; Phase 1 functionally complete, Phase 2 done; 12 / 19 tasks closed.

Later milestones (not yet opened) will graduate the PoC into the wider pipeline — e.g. durable peer/message store, and wiring the JSONL stream into the Sparkle indexer itself.

Tracking:

- Spec / plan: [[Telegram fetcher, PoC implementation plan]]
- Spec-compliance audits: `.github/reports/`
- Code-review iterations: `.github/reviews/`

## Backlog

> Useful, but not picked for the near work yet.
>
> state: OPEN

- [ ] [[IDX-13]] — service: keep photographs of posts from other platforms
- [ ] [[IDX-14]] — vault: index notes by filters and serve the index

## Todo

> The next agreed set of tasks.
>
> state: OPEN

- [ ] [[IDX-10]] — fetch: keep the author's formatting of message text

## In progress

> Work is going on right now.
>
> state: OPEN

- [ ] [[IDX-6]] — ci/cd: start release builds from a warm cache on the default branch
- [ ] [[IDX-15]] — test: make the dashboard and stats tests independent of the clock and the disk

## Idle

> Work has started but is deliberately paused or blocked.
>
> state: OPEN

## Done

> Delivered and checked.
>
> state: CLOSED · stateReason: COMPLETED, DUPLICATE

- [x] [[IDX-1]] — proxy: support MTProto proxies as a SOCKS5 replacement
- [x] [[IDX-2]] — auth: QR login as a working path when phone codes are not delivered
- [x] [[IDX-3]] — fetch: list forum topics of a dialog
- [x] [[IDX-4]] — fetch: download message media from a dialog or topic
- [x] [[IDX-5]] — ci/cd: restore HTTPS and Go module discovery for go.octolab.org
- [x] [[IDX-7]] — ci/cd: sign the Cask commits in the Homebrew tap
- [x] [[IDX-8]] — fetch: describe channels, groups and invite links as peer cards
- [x] [[IDX-9]] — fetch: describe users and bots as peer cards
- [x] [[IDX-11]] — service: save photographs lazily, from their URLs alone
- [x] [[IDX-12]] — fetch: give an album post whole

## Won't do

> Dropped as no longer relevant or contrary to the product.
>
> state: CLOSED · stateReason: NOT_PLANNED, DUPLICATE
