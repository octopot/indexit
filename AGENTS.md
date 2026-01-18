# Agent guide

## Releasing

A release is a curated note plus a signed tag. A stable release also publishes
the agent skill to the [octolab/skills](https://github.com/octolab/skills)
catalog. Pipeline mechanics, secrets and hooks are in
[.github/workflows/README.md](.github/workflows/README.md); docs conventions
are in [docs/README.md](docs/README.md#keep-the-release-accurate).

### Ground rules

- The maintainer commits, tags and pushes. Stage one self-contained change at a
  time and propose its message in a code block: a Conventional Commit subject,
  with issues as `fix #N` or `refs #N`, and a body wrapped at 72 columns that
  says what changed and why.
- Product changes first, release commit last: `chore(release): prepare vX.Y.Z`.
  A guide that explains new behavior changes with the product commit, and so
  does the agent skill in `skills/indexit/`; `go test ./skills` keeps its
  examples and command reference in step with the CLI, and
  `go generate ./skills` rewrites the reference.
- The CLI is the source of truth for what the docs, the notes and the skill
  claim.
- A published release is frozen: its note, annotation, image and catalog entry
  stay as tagged. A missed fix ships in the next patch.

### Before you start

`git fetch` and reconcile `main` with `origin/main`, then:

```sh
for w in ci cd.docs tools doctor; do gh run list -w "$w.yml" -b main -L 1; done
gh run list -R octolab/skills -w ci.yml -b main -L 1
make doctor
(cd docs && npm ci)
```

- The latest `ci` and `cd.docs` runs on `main` are green.
- `tools` is green, or fails only on govulncheck advisories in `tools/` that
  have no fixed version (`Fixed in: N/A`): `cd` installs the tools but doesn't
  scan them.
- `doctor` is green: it mints the tap and catalog tokens, which proves the App
  can publish to both. If the App, its installations, the secrets or
  `doctor.yml` changed since its last run, run it again:
  `make workflow-doctor REASON="before vX.Y.Z"` dispatches it on `main` and
  waits for the result; `REASON` is optional.
- `ci` in `octolab/skills` is green: the publish action refuses an invalid
  catalog.
- `make doctor` has no `fail`.

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
- After v0.2.0, end the **Upgrade:** paragraph with
  `If you use the agent skill, [replace it](/guide/agents/#keep-it-in-step) with the new binary’s copy.`

Then, in the same release commit:

- `changelog/_meta.js`: the new entry on top.
- `changelog/index.mdx`: a new card `vX.Y.Z · LATEST`; the previous one
  becomes `· STABLE`.
- `docs/content/index.mdx`: description, release pill, release band, scope note.
- The current version in `README.md`, `docs/README.md`, both quick starts
  (archive names included) and `docs/content/guide/*.mdx`, catalog tags such
  as `indexit--vX.Y.Z` included. In the content table of `docs/README.md`, the
  new note becomes the latest and the previous one gets its own row.
- `skills/indexit/SKILL.md`: `metadata.version` is the tag without `v`;
  `tool-version-range`, the `compatibility` line and the ranges in the skill's
  text become `>=0.Y.Z <0.(Y+1).0` before 1.0, `>=X.Y.Z <(X+1).0.0` after,
  and `=X.Y.Z-rc.N` for a prerelease.
  `make release-check` rejects another version, `go test ./skills` mismatched
  ranges.

```sh
grep -rn 'X\.Y\.W' README.md docs/README.md docs/content docs/public skills  # only old notes, cards and "since" facts may remain
go test ./skills
node .github/scripts/release.mjs render vX.Y.Z --site-url https://indexit.octolab.org/ --out "$(mktemp -d)/notes.md"
(cd docs && SITE_URL=https://indexit.octolab.org/ npx next build)
```

Check the sidebar: All releases first, then version groups with the new note.

### 2. Social preview image

`docs/public/og.png` shows the version at the bottom left. Patch only the
changed characters of the label, don't regenerate the image: cover each old
glyph with background copied from the same rows (the background is textured).
The label is monospaced: copy a new glyph from the label itself when it is
there, shifted by whole character cells; otherwise draw it at the same baseline
in Overpass Mono, about weight 450, which matches the label. Compare the label
at 4× and diff the images: only the label area may change.

### 3. Tag annotation

Follow the previous tag, `git tag -l --format='%(contents)' vX.Y.W`:

```text
vX.Y.Z — Slogan.

One or two sentences on what the release does, wrapped
at 72 columns.

- lowercase imperative bullets without trailing periods

Release notes: docs/content/changelog/vX.Y.Z.md
```

Same scope as the note. Write it to a file outside the repository:

```sh
annotation="$(mktemp -d)/annotation.txt"
```

### 4. Tag, check, push

```sh
git tag -s vX.Y.Z -F "$annotation"
git tag -v vX.Y.Z
make release-check TAG=vX.Y.Z
git push --atomic origin main vX.Y.Z
```

Run them on `main` with the release commit checked out: `make release-check`
checks the tag against the branch you are about to push, as the `pre-push`
hook does.

`git tag -v` must report a good signature from a key GitHub knows for your
email. The catalog publishes only from a tag GitHub verifies, so `cd` checks
that before anything is published.

Never push the tag separately. A tag pushed by mistake can be deleted only
before the GitHub release exists; after that, ship the next patch.

### 5. After the push

```sh
gh run watch --exit-status "$(gh run list -w cd.yml -c "$(git rev-parse 'vX.Y.Z^{commit}')" -L 1 --json databaseId -q '.[0].databaseId')"
gh release view vX.Y.Z --json name,assets -q '.name, .assets[].name'
gh api repos/octolab/homebrew-tap/commits/main -q '.commit.verification.verified, .commit.message'
gh api repos/octolab/skills/commits/indexit--vX.Y.Z -q '.commit.verification.verified, .commit.message'
gh run list -R octolab/skills -w ci.indexit.yml -L 1
```

- `cd` is green; for a prerelease, its skill job is skipped. If `gh run watch`
  says the run is not found, `cd` hasn't started yet: run the line again.
- The release has the note's title and body, four archives and
  `checksums.txt`.
- The Cask commit for vX.Y.Z is verified.
- A stable release only: the catalog tag `indexit--vX.Y.Z` points at a
  verified commit, and `ci.indexit` there is green for it.
- `brew update && brew upgrade --cask octolab/tap/indexit && indexit version`
  shows vX.Y.Z.
- `indexit skill install --agent claude-code,codex --replace`, then
  `indexit skill status --agent claude-code,codex --exact` passes. Update a
  catalog plugin that `status` lists with its agent's plugin manager first.
- The site shows the new changelog page and release band once `cd.docs`
  finishes for the release commit.

### When `cd` fails

- **Before the GitHub release exists** (the tag checks, tests or the build;
  `gh release view vX.Y.Z` finds no release): nothing is published. Delete
  the tag with `git push --delete origin vX.Y.Z && git tag -d vX.Y.Z`, fix the
  cause on `main` or in your signing setup, and go back to step 4.
- **Only the skill job failed**: the release and the Cask are out and stay.
  Fix the cause, for example the App's installation on `octolab/skills`, and
  run the failed job again: `gh run rerun <run-id> --failed`.
- **Anything else after the release exists**: it is frozen; ship the fix in
  the next patch.

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
