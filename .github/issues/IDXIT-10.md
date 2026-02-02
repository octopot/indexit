---
code: IDXIT-10
id: I_kwDOSVi84s8AAAABZCwzyg
databaseId: 5975585738
number: 128
url: https://github.com/octopot/indexit/issues/128
title: "fetch: keep the author's formatting of message text"
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
createdAt: 2026-10-22T08:11:17Z
updatedAt: 2026-10-22T08:11:42Z
lastEditedAt: null
closedAt: null
---

# fetch: keep the author's formatting of message text

## Motivation

A message record carries `text` as plain text: the formatting the author gave it — bold and italic, a link hidden behind words (`text_url`), inline code and a `pre` block with its language, a quote and an expandable quote, a spoiler, a custom emoji — is dropped on export, and the guide says so (`Text is plain; formatting entities are not exported`). A consumer that turns a post into Markdown or HTML cannot get it back: a prompt the author put into a code block reads as ordinary paragraphs, a link hidden behind a word is lost entirely, and a quote is indistinguishable from the text around it.

Telegram keeps this as a separate list of entities beside the text — the type, an offset and a length, plus a URL, a language or a document id where the type has one. The export is the only place that can keep it: the consumer has no session and no second chance once the post is edited or gone. The text stays plain as it is today; the entities come beside it, losslessly, so each consumer renders them in its own format.

## PoC

```bash
$ indexit telegram fetch message https://t.me/example_channel/11
{"_kind":"message","id":11,"text":"Save this prompt:\nTurn a photo into a poster…","entities":[{"type":"bold","offset":0,"length":4},{"type":"pre","offset":18,"length":30,"language":"text"},{"type":"text_url","offset":5,"length":4,"url":"https://example.com/"}], …}
```

## Acceptance criteria

- [ ] `fetch message` and `fetch messages` emit `entities` beside `text` whenever Telegram returns any, in Telegram's order; a message without formatting has no `entities` field;
- [ ] every entity type of the Telegram layer indexit is built with is kept under its own name, derived from its constructor (`messageEntityTextUrl` → `text_url`), and `messageEntityUnknown` as `unknown`; none is silently dropped;
- [ ] entities carry their parameters under Telegram's field names in snake_case: `url` of `text_url`, `language` of `pre`, `document_id` of a custom emoji, `collapsed` of an expandable quote; the user of a mention is a `user_uid` (`user:<id>`), like `linked_chat_uid`;
- [ ] offsets and lengths are documented in the unit Telegram uses (UTF-16 code units), with a sample of a text whose emoji make them differ from code points;
- [ ] `text` is unchanged byte for byte, so existing consumers see no difference;
- [ ] the media caption of a photo, an album or a document gets its entities the same way as a text message;
- [ ] the guide's output reference and the agent skill's output reference describe the field and drop the «formatting entities are not exported» line.

## Out of scope

- Rendering entities to Markdown or HTML: a consumer's choice of format.
- Downloading custom emoji pictures.
- Entities of service messages and of poll texts.

<!-- 2026-10-22T07:08Z https://github.com/octopot/indexit/issues/128#issuecomment-6336010803
Owner's notes on the criteria, against the code at v0.3.1:

- Unknown future types: gotd decodes a response against the layer it was generated for, and an entity constructor it does not know fails the whole response (`unable to decode MessageEntityClass: unexpected id`), so indexit never sees such an entity. The criterion now promises every type of the built-in layer instead; a test that enumerates the implementations of `MessageEntityClass` keeps any of them from being dropped, and naming by constructor picks up new types with a gotd upgrade. gotd v0.162 already knows more than the motivation lists: `formatted_date`, `diff_insert`, `diff_replace`, `diff_delete`, `box`.
- Mention of a user: the export names peers by UID elsewhere (`linked_chat_uid`, `personal_channel_uid`), so the entity does too.
- Captions: in Telegram a media caption is the message text with the same entities, so that criterion holds by construction; it stays as a test case for a photo and for the album member that carries the caption.
- `document_id` of a custom emoji is a 64-bit ID like `grouped_id`: the output reference's note on exact integers covers it.
- The skill in `skills/indexit/` is the first consumer that turns a post into a Markdown note. Rendering stays out of the CLI; whether the note recipe starts to use entities is a separate decision for the same release.
-->
