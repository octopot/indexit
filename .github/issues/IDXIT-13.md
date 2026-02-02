---
code: IDXIT-13
id: I_kwDOSVi84s8AAAABZCw_pQ
databaseId: 5975588773
number: 131
url: https://github.com/octopot/indexit/issues/131
title: "service: keep photographs of posts from other platforms"
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
createdAt: 2026-10-22T08:11:29Z
updatedAt: 2026-10-22T08:11:51Z
lastEditedAt: null
closedAt: null
---

# service: keep photographs of posts from other platforms

## Motivation

The local photograph service gives a Telegram photograph a permanent URL on this machine. The URL names where the photograph is, not an address to download it from: the service resolves it through Telegram on the first request, keeps the copy and serves it from then on without Telegram. A URL nobody opens costs nothing. A post that is gone before anyone opened its picture takes the picture with it, and that is fine: nothing was kept because nobody needed it.

Some platforms are like Telegram here: their pictures are reachable only through signed CDN addresses that expire within days, as on Instagram, Threads or Facebook, so a note that embeds such an address breaks silently. A platform whose posts embed plain, stable image URLs needs nothing: the note keeps that URL.

indexit supports such a platform from inside, as it supports Telegram: it reads the platform's posts itself, gives each picture a URL of stable coordinates, and the service knows how to resolve those coordinates through the platform when the URL is first opened. The service does not learn platforms or sources from requests, and nothing outside indexit registers pictures with it.

The URL scheme is already namespaced by platform (`/preview/telegram/v1/…`), so a platform joins it without moving the existing URLs.

Parked: nothing needs it until the first post of such a platform is kept.

## PoC

```bash
$ indexit instagram fetch post https://www.instagram.com/p/EXAMPLE/
{"_kind":"post","id":"EXAMPLE","media":[{"type":"photo","preview_url":"http://indexit.localhost:17840/preview/instagram/v1/…/….jpg"}], …}
$ curl -fsS -o /dev/null -w '%{http_code}\n' http://indexit.localhost:17840/preview/instagram/v1/…/….jpg
200
```

## Acceptance criteria

- [ ] a platform joins only when its pictures cannot be kept by their own URLs: they expire, or need the platform to resolve them;
- [ ] its pictures get URLs under `/preview/<platform>/v1/` that name the post and the picture, stable across restarts and origin-relative like the Telegram ones;
- [ ] giving a URL changes nothing on this machine; the first request resolves the picture through the platform and saves it, later requests serve the saved copy without the platform;
- [ ] a picture whose post is gone before its first request answers as a Telegram photograph that no longer exists;
- [ ] the service has no endpoint that accepts sources or registers pictures: it serves the platforms indexit is built with and nothing else;
- [ ] the dashboard and `/api/v1/stats` count these pictures by platform;
- [ ] the Telegram URLs and their behaviour are unchanged.

## Out of scope

- Videos and documents.
- Pictures or source addresses brought from outside indexit.

<!-- 2026-10-22T07:08Z https://github.com/octopot/indexit/issues/131#issuecomment-6336013769
Owner's correction of the original request, which proposed `POST /api/v1/register` taking a picture's source address from the consumer and saving it on the first request:

- indexit ships its platforms and does not extend itself from outside requests: support comes from inside, a fetcher and a resolver per platform, so the register endpoint and «a consumer brings the source address» are gone. It would also have been the service's first endpoint other than GET and HEAD, reachable by a simple cross-origin POST from any page open in the browser, with the service downloading whatever address it is given.
- Lazy saving stays, and it is what makes stable coordinates necessary: a signed address handed over at fetch time is likely to be dead by the first request, while coordinates the platform can resolve again are not. Losing a picture of a deleted post is accepted.
- Platforms with plain, stable image URLs are excluded explicitly: there the URL in the note already keeps the picture.
- `/api/v1/unsaved` is no longer counted per platform: it lists photographs a fetch registered, and a fetch should register nothing, see #129.
- Fit: the project in charge is the Telegram integration. A new platform brings its own fetcher, sign-in and output records, and is a project or milestone of its own when it is unparked.
-->
