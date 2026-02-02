---
code: IDXIT-12
id: I_kwDOSVi84s8AAAABZCw7jg
databaseId: 5975587726
number: 130
url: https://github.com/octopot/indexit/issues/130
title: "fetch: give an album post whole"
issueType: Feature
labels: []
assignees:
  - kamilsk
milestone: null
parent: null
issueFields: {}
state: OPEN
stateReason: null
duplicateOf: null
createdAt: 2026-10-22T08:11:25Z
updatedAt: 2026-10-22T08:14:57Z
lastEditedAt: 2026-10-22T08:14:57Z
closedAt: null
---

# fetch: give an album post whole

## Motivation

A person copies a link to a Telegram post and asks the agent to keep it as a note. When the post is an album, Telegram keeps it as several messages, one per photo or video, and the link points to one of them. `fetch message` returns that one message and the photo URLs of the album. The post's text lives on one member, not necessarily the linked one, and the album's videos and documents are not there at all. The note comes out without its text or with media missing, and nothing tells the agent so. To get the whole post it has to guess: fetch a window of neighbouring messages and keep those with the same `grouped_id`.

A person sees one post: its text and all its media. A link to any message of it should give the post whole, in one call.

## PoC

```bash
$ indexit telegram fetch message https://t.me/example_channel/12
{"_kind":"message","id":11,"grouped_id":777,"text":"What the author wrote","media":{"type":"photo","preview_url":"…"}, …}
{"_kind":"message","id":12,"grouped_id":777,"text":"","media":{"type":"photo","preview_url":"…"}, …}
{"_kind":"message","id":13,"grouped_id":777,"text":"","media":{"type":"video","duration":14}, …}
```

## Acceptance criteria

- [ ] a link to any message of an album gives every message of the album, in message order, each as a message record with its text and media; the output is the same whichever member the link points to;
- [ ] each message is emitted once per run, also when several links point into the same album;
- [ ] a message outside an album is emitted alone, as today;
- [ ] a member the account cannot see is named in a warning and the others are still emitted, as for a missing message today;
- [ ] `media.album` is gone: the members' own records carry their photo URLs;
- [ ] the agent skill builds the note of an album post from all its records, the text from whichever member has it;
- [ ] the messages guide and the output reference describe that a link to an album gives the whole album.

## Out of scope

- Downloading the members' files: that is `fetch media`.
- Comments of the post.
- `fetch messages`: a history already holds every message of an album.

<!-- 2026-10-22T07:08Z https://github.com/octopot/indexit/issues/130#issuecomment-6336013050
Owner's notes, against the code at v0.3.1:

- The motivation no longer blames the window for photographs piling up in `/api/v1/unsaved`. The album lookup already keeps only messages with the album's `grouped_id`; what registers the neighbours is `fetch messages` itself, which writes a source record for every photograph it describes. That a URL given in an answer changes state at all is the defect, tracked in #129. Until then the last criterion is a regression guard: it holds today.
- The lookup lives in the preview writer and runs only when a preview store is set. The member list is not about previews, so it belongs to the message fetch, with previews made from its result.
- A complete album without photographs has no `media.album` today; the second criterion gives it one. Existing fields keep their meaning, but the change deserves a line in the release note.
- `url` in a member repeats what `photos` already says: one of the two can go, so a consumer does not have to choose which to trust.
- The lookup already holds the members' full messages. An alternative to `has_text` plus a second fetch is a flag of `fetch message` that emits every member of the album as a message record of its own, with text and, after #128, entities: a post whole in one call. To decide before the work starts.
-->

<!-- 2026-10-22T08:14Z https://github.com/octopot/indexit/issues/130#issuecomment-6336065573
Reworded around the value: the agent wants the post as a person sees it, text and all media, from one link. The member list with `has_text` from the first version is dropped: it told the agent where the text was but still sent it for it in a second call. A link to an album now gives every message of the album, the alternative the note above left open, and `media.album` goes because the members' records carry their photo URLs themselves. The argument about photographs piling up in `/api/v1/unsaved` no longer applies after #129.
-->
