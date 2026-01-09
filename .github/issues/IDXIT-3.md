---
code: IDXIT-3
id: I_kwDOSVi84s8AAAABOSdbLw
databaseId: 5253847855
number: 80
url: https://github.com/octopot/indexit/issues/80
title: "fetch: list forum topics of a dialog"
labels:
  - "type: feature"
  - "scope: code"
  - "impact: medium"
  - "effort: medium"
milestone:
state: CLOSED
stateReason: COMPLETED
createdAt: 2026-08-26T06:01:14Z
updatedAt: 2026-08-26T06:01:28Z
lastEditedAt:
closedAt: 2026-09-18T11:53:43Z
issueType: Feature
assignees: []
parent: null
issueFields: []
---

# fetch: list forum topics of a dialog

## Why

Forum topics are reachable only if their ID is already known. `ResolvePeer`
picks a `TopicID` out of a pasted `t.me/c/<peer>/<topic>/<msg>` link
(`internal/telegram/resolve.go`), and `FetchMessages` then walks the topic
through `messages.getReplies` (`internal/telegram/fetch.go`). Nothing lists the
topics themselves: `fetch dialogs` emits the forum supergroup as a single
`DialogRecord` with `is_forum: true` and stops there.

So a forum that keeps one topic per subject — the concrete case is a photo
archive with an album per trip — cannot be walked programmatically. Every topic
ID has to be copied out of a client by hand, which does not scale past a few and
cannot be scripted at all.

This is milestone task 2.1 of the PoC plan.

## What changes

`indexit telegram fetch topics --dialog=<uid>` emits one JSONL record per forum
topic, through `channels.getForumTopics`.

- The record carries the stable identity (`topic_id`), the title, the creation
  date, the `top_message` ID, counters Telegram already returns (messages,
  unread), and the closed/hidden/pinned flags.
- `--dialog` accepts the same UID forms as the other fetchers, including
  `t.me/c/` links; a non-forum peer is a usage error that says so, not an empty
  result.
- Paging, `--limit`, `--page-size`, `-o` and the rate guard behave as in
  `fetch dialogs`.
- The emitted `topic_id` is directly usable as the `--dialog` topic anchor of
  `fetch messages` and `fetch media`.

## Acceptance criteria

- [x] `fetch topics --dialog=<forum uid>` lists every topic of the forum,
      paging past the first response;
- [x] each record carries `topic_id`, `title`, `date`, `top_message` and the
      closed/hidden/pinned flags;
- [x] a non-forum peer fails with a message naming the peer and the reason;
- [x] `--limit` and `--page-size` are honoured;
- [x] titles keep their original Unicode (emoji, Cyrillic) unescaped in JSONL;
- [x] mapper coverage for the topic record, in the style of the existing mapper
      tests.

## Verification

Run against a real forum supergroup and compare the emitted titles and IDs with
what the official client shows; feed one emitted `topic_id` into
`fetch messages` and confirm the same history comes back as with a pasted link.

## Выполнение

Реализовано в `internal/telegram/topics.go` (`FetchTopics`, обход
`messages.getForumTopics`), `mapper.Topic`, `model.TopicRecord` и команде
`internal/command/telegram/fetch.go`. Метод добавлен в интерфейс `API`, поэтому
ручные фейки тестов дополнены им же.

Живая проверка: форум «3. Areas / Кот & Утя» (`channel:3879648969`) — 31 топик
одним прогоном, кириллица и эмодзи в JSONL не экранированы, курсор двигается по
последнему топику страницы. Юнит: пагинация, остановка по `count`, `--limit`,
пропуск удалённых топиков, отказ на не-форуме, маппер записи.
