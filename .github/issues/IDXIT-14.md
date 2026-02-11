---
code: IDXIT-14
id: I_kwDOSVi84s8AAAABg_Lx6w
databaseId: 6508704235
number: 147
url: https://github.com/octopot/indexit/issues/147
title: "vault: index notes by filters and serve the index"
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
createdAt: 2026-11-20T09:30:44Z
updatedAt: 2026-11-20T09:59:39Z
lastEditedAt: 2026-11-20T09:59:39Z
closedAt: null
---

# vault: index notes by filters and serve the index

## Motivation

indexit indexes external sources and gives their consumers a convenient way
to get data out of them. Telegram was the first source; a notes vault is the
next one, and more will follow.

Several local tools need to know what is already in my notes: pulse hides
the GitHub trends I have in the vault. Each tool reads the vault on its own,
from a path in its own configuration, and fails silently when that path is an
old clone: on 2026-11-20 pulse read a clone from October and kept showing 21
repositories I had just restored.

indexit should find the vaults of this Mac, index every note in them, and let
a consumer ask for the subset it needs, such as "every note that names a
GitHub repository", instead of reading the files.

## PoC

```bash
$ indexit vault list
Mindset    ~/Reflection/Mindset            known to Obsidian
...
$ curl -s 'http://indexit.localhost:17840/api/v1/vaults/Mindset/notes?key=github-repo'
{"vault":"Mindset","built_at":"…","notes":2712,"entries":{"github.com/owner/name":[{"note":"4. Resources/…/GitHub - 123.md"}]}}
```

## Behavior

- Discovery: indexit proposes the vaults Obsidian knows on this Mac, from
  Obsidian's own list, with their paths. It does not scan the disk, and it
  indexes only the vaults the configuration names.
- A vault is indexed whole: every Markdown note, with its path, its tags and
  its frontmatter. The body of a note is not read.
- A consumer asks for a subset by conditions: a path prefix or pattern, tags,
  frontmatter fields, and keys extracted by a named extractor, such as
  `github-repo` for the repositories a note names in its `url` or in an
  `owner/name` alias. Frequent questions can be given a name in the
  configuration.
- Keys are normalized: a GitHub repository is one key whatever its case,
  `www.`, `.git` or trailing slash; an owner without a repository is not a
  repository key.
- The index reflects the notes on disk at the time it is asked: a note added
  or restored is in the next answer, without a manual reindex. Unchanged
  notes are not read again.
- Answers are read only, on loopback, with a validator so that an unchanged
  subset answers "not modified".
- The dashboard names each vault with its path, its notes and the time it was
  indexed.

## Limits

- Only the frontmatter, the tags and the path of a note are indexed.
- Only named extractors, no free-form patterns, in this task.
- Read only: indexit never writes to a vault.
- Only GET and HEAD, as the rest of the service: no lookup by POST.

## Acceptance criteria

- [ ] Test: Obsidian's list with three vaults gives three proposals; a clone
  Obsidian does not know is not proposed.
- [ ] Test: every note of a fixture vault is indexed, in any folder.
- [ ] Each condition (path, tags, frontmatter, key) is tested on fixtures,
  alone and combined.
- [ ] Test: the variants of a GitHub URL and of `owner/name` give one key.
- [ ] Test: a note added after the first answer is in the next one; with no
  change the answer is "not modified".
- [ ] Test: a write method gets 405 and a request from another site 403.
- [ ] The URLs of the photograph service do not change.
- [ ] The guide in `docs/content/` describes vault discovery, the conditions
  and the API, with an example.

## Out of scope

- Full-text search over note bodies.
- Access tokens and an access log: they come later, for every consumer.
- Clients of the index: pulse integrates on its own side.
