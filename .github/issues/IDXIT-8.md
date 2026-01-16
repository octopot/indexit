---
code: IDXIT-8
id: I_kwDOSVi84s8AAAABU7ZCKg
databaseId: 5699420714
number: 109
url: https://github.com/octopot/indexit/issues/109
title: "fetch: describe channels, groups and invite links as peer cards"
labels:
  - "type: feature"
  - "scope: code"
  - "impact: medium"
  - "effort: medium"
milestone: "[[IDXIT-M1]]"
state: CLOSED
stateReason: COMPLETED
createdAt: 2026-10-04T12:47:22Z
updatedAt: 2026-10-04T13:29:53Z
lastEditedAt: 2026-10-04T12:55:14Z
closedAt: 2026-10-04T13:29:53Z
issueType: Feature
assignees: []
parent: null
issueFields: []
---

# fetch: describe channels, groups and invite links as peer cards

## Why

indexit can walk a dialog, but it cannot say what the dialog is. `fetch dialogs` names each peer — ID, type, title, username — and only for dialogs the account already has. What a channel is about, how many people follow it, which group hosts its comments, which other usernames it answers to: none of that is reachable. An invite link cannot be looked at at all.

These are exactly the facts a consumer needs to file a channel rather than its messages. The concrete case is a notes vault that keeps one note per Telegram channel: the note wants the channel's own description and audience, and the channel's discussion chat belongs on the same note — so channels and chats given together must be paired by what Telegram says links them, not by guessing from titles. Telegram's web previews are no way around it: they are blocked on some networks, show only public channels, and never name the linked group.

## What changes

A fetch command that takes one or more references and answers each with a JSONL card of the peer it names.

- A reference is any form indexit already accepts, plus a channel's web preview link and an invite link.
- A card carries the peer's stable identity and type (channel, supergroup, basic group, user, bot), its title and usernames, its description, member count, the linked channel or discussion group, and the flags Telegram reports.
- Every reference gets exactly one card, in the order given; a reference that cannot be described yields a card with a machine-readable reason instead of stopping the run.
- Looking never changes the account: no chat is joined, an invite is only inspected.

## Out of scope

- Joining chats or accepting invites.
- Message history, media, participants lists.
- Profile photos and other files.

## Acceptance criteria

- [ ] a public channel by username yields its description, member count and linked discussion group;
- [ ] a discussion group yields the channel it belongs to;
- [ ] an invite link to a chat the account is in yields the full card; to any other chat, what Telegram previews and a reason why there is no identity;
- [ ] an expired or revoked invite, an unknown username, a numeric ID unknown to the session, and a user or bot each yield a distinct reason;
- [ ] one card per reference, in order, whatever fails in between;
- [ ] the exit status tells a run where nothing could be described from a partial one;
- [ ] the guide documents the card fields and the reasons.

## Verification

Describe a few real channels — one with a discussion group, one without — their groups and an invite link, and compare every field with what the official client shows; then confirm that the account's chat list is unchanged.
