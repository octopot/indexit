---
code: IDXIT-4
id: I_kwDOSVi84s8AAAABOSddOw
databaseId: 5253848379
number: 81
url: https://github.com/octopot/indexit/issues/81
title: "fetch: download message media from a dialog or topic"
labels:
  - "type: feature"
  - "scope: code"
  - "impact: high"
  - "effort: medium"
milestone:
state: OPEN
stateReason:
createdAt: 2026-08-26T06:01:20Z
updatedAt: 2026-08-26T06:01:30Z
lastEditedAt:
closedAt:
issueType: Feature
assignees: []
parent: null
issueFields: []
---

# fetch: download message media from a dialog or topic

## Why

`fetch messages` describes media but cannot hand over the bytes, and by
construction never will: `mapper.Media`
(`internal/telegram/mapper/mapper.go:228`) keeps `type`, `mime`, `size` and
`duration` and drops the file location — `id`, `access_hash`, `file_reference`,
DC — while `model.MediaDescriptor` has no fields to hold them. Even if it did,
`file_reference` is short-lived, so the two-phase shape "emit JSONL now,
download later" is not sound: the bytes have to be fetched in the same pass that
walked the history.

The practical case is a photo archive kept as one forum topic per trip. Today
the only way to get those frames out is to save them by hand from an official
client, one album at a time.

## What changes

`indexit telegram fetch media --dialog=<uid> --dir <path>` walks the same
history as `fetch messages` (including `messages.getReplies` for a topic), and
downloads every media message into a file through the `gotd` downloader, while
`-o` keeps emitting JSONL — one manifest record per file.

- `--dir` is the destination directory. `-o/--output` keeps its existing
  meaning (manifest to a file or stdout), so no flag changes type between
  commands.
- A manifest record carries `message_id`, `topic_id`, `date`, `grouped_id` (the
  album a frame belongs to), `type`, `mime`, `size`, the file `path` and whether
  the file was downloaded or skipped as already present.
- File names are deterministic: `<message-id>.<ext>`, extension taken from
  `DocumentAttributeFilename` or the MIME type (`.jpg` for photos). A second run
  therefore neither duplicates nor renames, and lexicographic order matches
  chronological order.
- Photos are downloaded at the largest available size; `--media=photo,video,…`
  narrows what is fetched.
- Already-downloaded files are skipped by default; `--overwrite` re-fetches.
- The windows of `fetch messages` apply unchanged: `--from`, `--to`, `--min-id`,
  `--max-id`, `--limit`, `--page-size`; flood waits stay with `RateGuard`.
- Downloading needs the `upload.*` RPCs, which the narrow `API` interface does
  not carry. A `MediaAPI` (`API` plus `downloader.Client`) is introduced; the
  concrete `*tg.Client` already satisfies it, so the hand-written fakes in the
  existing tests keep compiling.

## Acceptance criteria

- [x] `fetch media --dialog=<t.me/c link with a topic anchor> --dir <dir>`
      writes every photo of that topic into `<dir>` and one manifest record per
      file;
- [x] the same command against a non-forum dialog downloads that dialog's media;
- [x] a repeat run downloads nothing and reports every file as skipped;
- [x] `--overwrite` re-downloads them;
- [x] `--media=photo` excludes documents and video, and vice versa;
- [x] `--limit`, `--from`/`--to` and `--min-id`/`--max-id` bound the run exactly
      as they do for `fetch messages`;
- [x] a photo arrives at its largest size, not a thumbnail — verified against
      the size Telegram reports;
- [x] `grouped_id` is present for messages that belong to an album, so frame
      order inside an album is recoverable;
- [x] a failed download of one message does not abort the run: it is reported
      and the walk continues;
- [x] unit coverage for location building (photo size selection, document
      location, file naming) and for the skip/overwrite decision, using the
      hand-written fake style of `internal/telegram/fetch_test.go`.

## Verification

Run against a real forum topic holding 43 photos and compare the file count,
the byte sizes and the visible frames with the official client; run it twice to
confirm the second pass is a no-op.

## Выполнение

Реализовано в `internal/telegram/media.go` (`FetchMedia`, построение
`InputPhotoFileLocation`/`InputDocumentFileLocation`, выбор наибольшего размера
фото, имена файлов), `model.MediaRecord` и команде `fetch media`. Обход истории
вынесен из `FetchMessages` в общий `walkMessages`: `fetch messages` и
`fetch media` расходятся в том, что делают с сообщением, а не в том, как
листают историю. Загрузка идёт во временный `<файл>.part` и переименовывается на
место, поэтому прерванный прогон не оставляет файла, который следующий прогон
счёл бы скачанным.

Живая проверка: топик «Ваньково, июнь 2025» — 43 фото, 5,8 МБ, повторный прогон
1,16 с без единой загрузки; выгрузка девятнадцати альбомов подряд — 547 файлов,
75 МБ, ни одного отказа. Отдельно подтвердилось, что ссылка
`t.me/c/<peer>/<topic>` без третьего сегмента означает СООБЩЕНИЕ, а не топик:
такой вызов прошёл по всему диалогу (680 файлов). Поведение верное — это
семантика ссылок Telegram, — но цена ошибки в мегабайтах, поэтому адресация
топика через `channel:<id>:<topic>` названа в `--help` команды.
