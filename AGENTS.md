# Agent guide

## Releasing

A release is a curated note plus a signed tag. Pipeline mechanics, secrets and
hooks are in [.github/workflows/README.md](.github/workflows/README.md); docs
conventions are in [docs/README.md](docs/README.md#keep-the-release-accurate).

### Ground rules

- The maintainer commits, tags and pushes. Stage one self-contained change at a
  time and propose a one-line Conventional Commit subject in a code block, with
  issues as `fix #N` or `refs #N`.
- Product changes first, release commit last: `chore(release): prepare vX.Y.Z`.
  A guide that explains new behavior changes with the product commit.
- The CLI is the source of truth for what the docs and notes claim.
- A published release is frozen: its note, annotation and image stay as tagged.
  A missed fix ships in the next patch.

### Before you start

`git fetch` and reconcile `main` with `origin/main`; `ci`, `tools` and
`cd.docs` are green; `make doctor` has no `fail`; `npm ci` in `docs/`.

### 1. Changelog

Write `docs/content/changelog/vX.Y.Z.md`; it becomes the GitHub release:

```md
---
title: vX.Y.Z — Slogan.
description: "One sentence on what changed for the user."
---

# vX.Y.Z — Slogan.

Lead paragraph: what the release does, in terms of commands and results.

## A section per user-facing change

- Short, concrete bullets.

[Explore messages →](/guide/messages/)

**Upgrade:** on macOS, run `brew update` followed by `brew upgrade --cask octolab/tap/indexit`. For a manual installation, use a [vX.Y.Z release archive](https://github.com/octopot/indexit/releases/tag/vX.Y.Z). Run `indexit version` to check the installed version.
```

- English; a short slogan ending with a period ("One message, one entry.").
- Only what users get: behavior and Telegram compatibility, no docs, tools or
  CI/CD. Explain jargon in user terms; no comparisons to release candidates.
- Links are site-relative with a trailing slash.

Then, in the same release commit:

- `changelog/_meta.js`: the new entry on top.
- `changelog/index.mdx`: a new card `vX.Y.Z · LATEST`; the previous one
  becomes `· STABLE`.
- `docs/content/index.mdx`: description, release pill, release band, scope note.
- The current version in `README.md`, `docs/README.md`, both quick starts
  (archive names included) and `docs/content/guide/*.mdx`.

```sh
grep -rn 'X\.Y\.W' README.md docs/README.md docs/content docs/public  # only old notes and cards may remain
node .github/scripts/release.mjs render vX.Y.Z --site-url https://indexit.octolab.org/ --out "$(mktemp -d)/notes.md"
(cd docs && SITE_URL=https://indexit.octolab.org/ npx next build)
```

Check the sidebar: All releases first, then version groups with the new note.

### 2. Social preview image

`docs/public/og.png` shows the version at the bottom left. Patch only the
changed characters of the label, don't regenerate the image: cover each old
glyph with background copied from the same rows (the background is textured),
draw the new one at the same baseline in Overpass Mono, about weight 450, which
matches the label. Compare the label at 4× and diff the images: only the label
area may change.

### 3. Tag annotation

Follow the previous tag, `git tag -l --format='%(contents)' vX.Y.W`:

```text
vX.Y.Z — Slogan.

One or two sentences on what the release does, wrapped
at 72 columns.

- lowercase imperative bullets without trailing periods

Release notes: docs/content/changelog/vX.Y.Z.md
```

Same scope as the note. Write it to a file outside the repository.

### 4. Tag, check, push

```sh
git tag -s vX.Y.Z -F annotation.txt
make release-check TAG=vX.Y.Z
git push --atomic origin main vX.Y.Z
```

Never push the tag separately. A tag pushed by mistake can be deleted only
before the GitHub release exists; after that, ship the next patch.

### 5. After the push

Watch `cd` (`gh run watch`), then check:

- the GitHub release: note title and body, four archives, `checksums.txt`;
- the Cask commit in `octolab/homebrew-tap` is verified;
- `brew upgrade --cask octolab/tap/indexit && indexit version`;
- the site shows the new changelog page and release band.

### 6. Announcement

Propose a row for "Latest contributions" in the
[kamilsk/kamilsk](https://github.com/kamilsk/kamilsk) profile README:

```md
| 2026-10-02   | 🗃️ [indexit][]              | [v0.1.1][oi011], [v0.1.2][oi012]  | Telegram exports without duplicates       |

[oi012]:  https://github.com/octopot/indexit/releases/tag/v0.1.2
```

Newest first, one row per release day. The reference label is `o` + project
initials + version digits. Notes is a short phrase, no longer than the
existing ones, saying what the user gets.
