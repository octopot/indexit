---
code: IDXIT-11
id: I_kwDOSVi84s8AAAABZCw32A
databaseId: 5975586776
number: 129
url: https://github.com/octopot/indexit/issues/129
title: "service: save photographs lazily, from their URLs alone"
issueType: Feature
labels: []
assignees:
  - kamilsk
milestone: null
parent: null
issueFields: {}
state: CLOSED
stateReason: COMPLETED
duplicateOf: null
createdAt: 2026-10-22T08:11:21Z
updatedAt: 2026-10-22T09:37:39Z
lastEditedAt: 2026-10-22T09:37:39Z
closedAt: 2026-10-22T09:55:17Z
---

# service: save photographs lazily, from their URLs alone

## Motivation

A preview URL is a name: it says which photograph of which post a note shows, not an address to download it from. The service turns it into a picture when someone opens it: it resolves the coordinates through Telegram on the first request, keeps the copy and serves it from then on without Telegram. Opening is the consumer's act, so giving a URL should change nothing, and a URL nobody opens should leave no trace, as if it had never been given. A post that is gone before anyone opened its picture takes the picture with it, and that is fine: nothing was kept because nobody needed it.

Today it does not work that way. For every photograph `fetch message` and `fetch messages` describe, they write a source record into the data directory, because the service needs the peer's access hash to download the photograph and the URL does not carry it. A fetch therefore changes state: every photo of a `fetch messages` run counts as registered, and the dashboard and `/api/v1/unsaved` list it as needing a look and as lost with the post, although no note embeds it. A consumer that reads a channel to search or filter it fills the list with photographs it never meant to keep.

The saved pictures are worth sharing between computers: kept in a cloud folder such as Google Drive, they should display by the same URLs on another computer that runs the service, without asking Telegram. Today the URL names the Telegram account, so it cannot. A URL should hold only what any Telegram client sees in the post itself: the channel, the post and the photograph.

The service is for posts a person can copy a message link to: posts of channels and supergroups, private ones included (`t.me/c/…`). Private chats and basic groups have no such link, and the service does not cover them.

## PoC

```bash
$ touch before && indexit telegram fetch message https://t.me/c/1987654321/4217 | jq -r .media.preview_url
http://indexit.localhost:17840/preview/telegram/v1/c1987654321/4217/p5839201746381920304.jpg
$ find ~/.local/share/indexit -newer before -type f | wc -l     # the fetch wrote nothing
       0
$ curl -fsS -o /dev/null -w '%{http_code}\n' http://indexit.localhost:17840/preview/telegram/v1/c1987654321/4217/p5839201746381920304.jpg
200
$ ls "$HOME/Google Drive/indexit/images/c1987654321/4217/"
p5839201746381920304.jpg
# another computer, same Drive folder, the service running without a Telegram session:
$ curl -fsS -o /dev/null -w '%{http_code}\n' http://indexit.localhost:17840/preview/telegram/v1/c1987654321/4217/p5839201746381920304.jpg
200
```

## Acceptance criteria

URLs and fetch:

- [x] `fetch message` and `fetch messages` write nothing to the data directory for the photographs they describe: a preview URL is a function of the message alone;
- [x] a photograph of a channel or supergroup post, public or private, gets `/preview/telegram/v1/c<channel_id>/<message_id>/p<photo_id>.jpg`; nothing in it identifies the account, and the photo ID keeps its sign;
- [x] a photograph of a private chat or a basic group gets neither `preview_url` nor `preview_error`: it is not a case of the service, not a failure; `fetch media` still downloads it;
- [x] the picture's path on disk is the URL's path under the image directory, and nothing else is kept per picture.

Serving:

- [x] a picture on disk is served without any request to Telegram and without a Telegram session; an existing file that cannot be read answers 503, never a new download;
- [x] on a miss the service finds the channel by itself, in this order: a channel it already found, the CLI's peer cache read without writing it, the account's dialogs with the archive, the channel's cached username resolved and checked against the ID; otherwise it answers 404 that tells to fetch the post on this computer. Concurrent misses of one channel share one lookup;
- [x] the service re-reads the post, checks the photo ID and answers 410 when the post or the photograph is gone or replaced;
- [x] any signed-in account that can see the channel can fill a miss: the service no longer requires the account that gave the URL;
- [x] a min channel never overwrites a full entry of the peer cache, so a cached access hash can always read posts.

Storage:

- [x] the image directory can be set apart from the rest of the data, for example to a Google Drive folder (`images` under `[service]`); locks, statistics and thumbnails stay on the local disk;
- [x] two computers that share the image directory show the same pictures by the same URLs; a miss is filled by whichever service is asked, if its account can see the channel;
- [x] a picture is written to a temporary file in its own directory, named for the host and process, checked as a complete JPEG, then renamed; a saved picture is never overwritten, and a service removes only its own temporary files;
- [x] a conflict copy left by the cloud folder (`p… (1).jpg`) is neither served nor counted as an error.

Dashboard, compatibility and docs:

- [x] the dashboard counts the pictures on disk; «what needs a look» for photographs nobody opened, `/api/v1/unsaved` and the warm-up command are gone;
- [ ] the URLs with the account segment, the `sources/` records and the code that reads them are gone; the release note says that notes made before have to be made again;
- [x] the local images guide, the output reference and the agent skill describe the new URLs, the image directory and that previews cover only posts with a message link, and no longer call a fetched photograph registered;
- [x] tests prove that a fetch writes nothing, that a hit never calls Telegram, each step of the channel lookup, and 410 for a replaced photograph.

## Out of scope

- Private chats and basic groups: they have no message link, and the service will never cover them.
- Compatibility with URLs given before: indexit is at v0 with a single user.
- Serving beyond this computer, such as inside a Tailscale network.
- Photographs of web page previews and images sent as documents.
- Saving photographs ahead of the first request.

<!-- 2026-10-22T07:08Z https://github.com/octopot/indexit/issues/129#issuecomment-6336011588
Owner's notes, against the code at v0.3.1:

- The state is the `sources/` record of `preview.Store.Save`, written from the preview writer of `fetch`. The service reads it on the first request for the access hash (`source metadata unavailable; fetch the message again` without it). Basic groups and the account itself need no hash; channels and users do.
- Decided: no access hash in the URL. The URL keeps its coordinates; on the first request the service gets whatever it lacks, the access hash included, by itself, and may read the post to do so.
- v0.3.1 ships «what needs a look», `/api/v1/unsaved` and the warm-up command in the local images guide, all built on registrations. With nothing registered they have nothing to list, and they go in the release that ships this. The v0.3.1 note stays as published.
- #130 and #131 rely on this: a platform URL should cost nothing until opened, and an album lookup should not need a guard against registering neighbours.
-->

<!-- 2026-10-22T07:59Z https://github.com/octopot/indexit/issues/129#issuecomment-6336012336
Design agreed with a review by Fable 5.1 and gpt-6-astra (high), checked against the code at v0.3.1 and gotd v0.162:

- Channel ID, message ID within a channel and photo ID are the same for every account, so the URL can be account-free. In private chats and basic groups message IDs are per account; they have no message link either, and the owner keeps them out of the service for good.
- A channel's access hash cannot be obtained from its ID alone (`CHANNEL_INVALID`), hence the lookup chain. The username is not put into the URL: it changes, and private channels have none. The peer cache is not a registration: every fetch writes it anyway, and the service only reads it.
- The photo ID stays in the URL: after an edit that replaces the photograph, the old URL answers 410 instead of showing another picture.
- Two defects to fix on the way: `mapper.CacheChannel` stores min channels without the guard users have, and `DownloadPreview` rejects a session of another account.
- Writing to a cloud folder: gpt-6-astra proposed a local temporary file, a copy and a SHA-256 sidecar; the simpler recipe with a temporary file next to the target and a JPEG check was chosen. Keeping the URLs given before working, proposed by both as a new URL version beside the old one, was dropped by the owner: v0, a single user.
-->

<!-- 2026-10-22T13:05Z https://github.com/octopot/indexit/issues/129#issuecomment-6337604735
Implemented and staged with #130, reviewed by Fable 5.1 and gpt-6-astra (xhigh) in three rounds. Open until the release commit: the v0.4.0 note says that notes made before have to be made again (draft ready). Two choices on the way: a download takes one of 64 shared lock files of this computer, so nothing is kept per picture; where a filesystem has no hard links, publishing falls back to a rename after a look, which only another computer saving the same photograph, with the same bytes, can race.
-->
