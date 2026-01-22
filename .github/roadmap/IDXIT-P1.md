---
code: IDXIT-P1
id: null
fullDatabaseId: null
number: null
url: null
owner:
  id: null
  login: "lifeosm"
  type: Organization
title: "Telegram integration"
shortDescription: "Read-only personal-data ingestion from Telegram into the Sparkle index — dialogs, messages, topics, contacts, multi-account."
public: true
closed: false
createdAt: null
updatedAt: null
closedAt: null
creator: kamilsk
tags:
  - type/project
  - topic/telegram
---

# Telegram integration

Bring Telegram into the `indexit` ingest pipeline as a first-class, read-only source for the Sparkle index. The `indexit telegram` subcommand streams the user's own Telegram data (dialogs, message history, forum topics, contacts, and Story viewers) as stable JSONL that a downstream Sparkle indexer can consume without re-fetching.

Design and scope live in the spec: [[Telegram fetcher, PoC implementation plan]].

Milestones:

- [[IDXIT-M1]] — Telegram fetcher PoC implementation: open; Phase 1 functionally complete, Phase 2 has one task left; 11 / 19 tasks closed.

Later milestones (not yet opened) will graduate the PoC into the wider pipeline — e.g. durable peer/message store, and wiring the JSONL stream into the Sparkle indexer itself.

Tracking:

- Spec / plan: [[Telegram fetcher, PoC implementation plan]]
- Spec-compliance audits: `.github/reports/`
- Code-review iterations: `.github/reviews/`
