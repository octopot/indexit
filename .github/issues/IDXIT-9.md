---
code: IDXIT-9
id: I_kwDOSVi84s8AAAABXrRFwA
databaseId: 5883839936
number: 122
url: https://github.com/octopot/indexit/issues/122
title: "fetch: describe users and bots as peer cards"
issueType: Feature
labels:
  - "type: feature"
  - "scope: code"
  - "impact: medium"
  - "effort: medium"
assignees:
  - kamilsk
milestone: "[[IDXIT-M1]]"
parent: null
issueFields: {}
state: OPEN
stateReason: null
duplicateOf: null
createdAt: 2026-10-16T12:05:08Z
updatedAt: 2026-10-16T12:05:08Z
lastEditedAt: null
closedAt: null
---

# fetch: describe users and bots as peer cards

## Motivation

`fetch peer` describes channels and groups in full since [[IDXIT-8]], but a user or a bot gets only its identity and the error `not_channel`: no bio, no personal channel, nothing a consumer can file. The case behind it is the same notes vault: a person's Telegram account is a note of its own, next to their contact card and their channel, and that note wants the account's bio and the channel the account itself points to. Today the bio is copied by hand from the `t.me/<username>` web preview, which is blocked on some networks and never names the personal channel; the channel is guessed from the bio text.

Telegram answers this with one call, `users.getFullUser`: the bio, the personal channel (`personal_channel_id`), and for a bot its description and commands.

## PoC

```bash
$ indexit telegram fetch peer @example_person @example_bot
{"_kind":"peer","ref":"@example_person","uid":"user:<id>","peer_type":"user","title":"…","username":"example_person","about":"…","personal_channel_uid":"channel:<id>","personal_channel":{"uid":"channel:<id>","title":"…","username":"…"}, …}
{"_kind":"peer","ref":"@example_bot","uid":"user:<id>","peer_type":"bot","title":"…","username":"example_bot","about":"…","bot":{"description":"…","commands":[…]}, …}
```

## Acceptance criteria

- [ ] a user by username yields its bio, other active usernames and the flags Telegram reports (verified, premium, scam, fake);
- [ ] a user with a personal channel yields that channel's uid, title and username, shaped like `linked_chat` of a channel card;
- [ ] a bot yields `peer_type: "bot"` and its description and commands, whether it is given as `@username` or as a cached `user:<id>`;
- [ ] a user hidden by privacy settings yields what Telegram gives and no error for the missing parts;
- [ ] the phone number is never emitted, even when Telegram returns it for a contact;
- [ ] `not_channel` is retired or kept only for a documented reason, and the guide's card fields and reasons are updated;
- [ ] still one card per ref, in order, and the account is not changed.

## Out of scope

- Profile photos, stories, gifts and other files.
- Common chats with the user, their contacts or presence.
- Messages of the personal channel: that is `fetch messages`.
