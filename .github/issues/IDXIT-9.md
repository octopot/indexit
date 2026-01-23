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
state: CLOSED
stateReason: COMPLETED
duplicateOf: null
createdAt: 2026-10-16T12:05:08Z
updatedAt: 2026-10-18T13:38:26Z
lastEditedAt: 2026-10-18T13:37:56Z
closedAt: 2026-10-18T13:38:26Z
---

# fetch: describe users and bots as peer cards

## Motivation

`fetch peer` describes channels and groups in full since [[IDXIT-8]], but a user or a bot gets only its identity and the error `not_channel`: no bio, no personal channel, nothing a consumer can file. The case behind it is the same notes vault: a person's Telegram account is a note of its own, next to their contact card and their channel, and that note wants the account's bio and the channel the account itself points to. Today the bio is copied by hand from the `t.me/<username>` web preview, which is blocked on some networks and never names the personal channel; the channel is guessed from the bio text.

Telegram answers this with one call, `users.getFullUser`: the bio, the personal channel (`personal_channel_id`), the birthday and business card, and for a bot its description and commands. The card carries everything the profile shows, the phone number included when Telegram returns it.

A user card is only as good as the address it is fetched by, and that address is account-bound: the peer cache keeps an access hash per user and channel, and Telegram issues it to one account only. `auth logout` removes the session but keeps the cache, so after logging in as another account — or pointing `--session` at another account's file — every cached `user:<id>` and `channel:<id>` goes out with a foreign hash and fails. Accounts will multiply once several sessions are supported, so the cache has to know whose it is.

## PoC

```bash
$ indexit telegram fetch peer @example_person @example_bot
{"_kind":"peer","ref":"@example_person","uid":"user:<id>","peer_type":"user","title":"…","username":"example_person","about":"…","personal_channel_uid":"channel:<id>","personal_channel":{"uid":"channel:<id>","title":"…","username":"…"}, …}
{"_kind":"peer","ref":"@example_bot","uid":"user:<id>","peer_type":"bot","title":"…","username":"example_bot","about":"…","bot":{"description":"…","commands":[…]}, …}
```

## Acceptance criteria

- [x] a user by username yields its bio, first and last name, other active usernames and the flags Telegram reports (verified, premium, scam, fake, deleted, support);
- [x] a user with a personal channel yields that channel's uid, title and username, shaped like `linked_chat` of a channel card;
- [x] a bot yields `peer_type: "bot"` and its description and commands, whether it is given as `@username` or as a cached `user:<id>`;
- [x] a user hidden by privacy settings yields what Telegram gives and no error for the missing parts;
- [x] whatever else the profile shows is on the card when Telegram returns it: the phone number, the birthday (with or without the year), the business location, hours and intro, the Stars rating and the price of a paid message;
- [x] a failed full request still yields the identity, with the error;
- [x] `not_channel` is retired: a user or a bot is a full card, not an error, and the guide's card fields and reasons are updated;
- [x] still one card per ref, in order, and the account is not changed;
- [x] a peer cache written for one account is never used for another, whether the switch came through logout and login, `--session` or `--peer-cache`: the other account starts with an empty cache and a warning that says how to refill it;
- [x] a cache that cannot be shown to belong to the current account, including one written before this change, is not trusted;
- [x] the same account keeps its cache across a re-login, and the check costs no request while the session is unchanged;
- [x] when the account cannot be determined, the command still runs without the cache and does not overwrite it;
- [x] `auth status` shows which account the peer cache belongs to.

## Out of scope

- Profile photos, stories, gifts and other files; profile decoration (emoji status, colours, wallpaper).
- Common chats with the user, their contacts or presence.
- What the profile means only to this account: blocked, notification settings, the private note, read dates.
- Messages of the personal channel: that is `fetch messages`.

<!-- 2026-10-18T13:38Z https://github.com/octopot/indexit/issues/122#issuecomment-6250577943
Shipped in [v0.2.2](https://github.com/octopot/indexit/releases/tag/v0.2.2) and verified on a live account: a user card with bio, names and personal channel; @BotFather and the cached user:93372553 both as peer_type bot with description and commands; the ownerless cache reset with a warning on the first fetch; verified=session afterwards; auth status shows the cache owner.
-->
