# Agent guide

## Local services

A local service listens on loopback only, on `127.0.0.1` and `::1`, and
answers at `http://<name>.localhost:<port>/`; `brew services` runs it as the
user, without root. Each one has its own port, recorded here and in the
AGENTS.md of the other repository:

| Port | Service |
| --- | --- |
| 17840 | indexit |
| 17841 | maintainer |

A new service takes the next free port and adds it to both tables.

## Releasing

A release is a curated note plus a signed tag. A stable release also publishes
the agent skill to the [octolab/skills](https://github.com/octolab/skills)
catalog. A prerelease (`vX.Y.Z-rcN`) is a release too: it has its own note and
tag and is frozen once published. GoReleaser publishes it as a prerelease that
never becomes *Latest* and leaves the tap as it is, and the catalog keeps
stable releases only. Pipeline mechanics, secrets and hooks are in
[.github/workflows/README.md](.github/workflows/README.md); docs conventions
are in [docs/README.md](docs/README.md#keep-the-release-accurate).

### Ground rules

- The agent brings the shells, the maintainer fires: the agent prepares each
  step in full and hands over one command; the maintainer reviews it, commits,
  tags and pushes. Nothing else waits for him.
- A change is a series of commits planned up front, each bound to its issue
  (`, issue #N` while it is in progress, `, closes #N` on the commit that
  finishes it). The agent stages one self-contained commit at a time, writes
  its message to a file outside the repository and gives
  `git contrib < <file>`: `type(scope): summary`, then, when it helps, a body
  wrapped at 72 columns. An intermediate state the working tree no longer has
  is staged from blobs (`git hash-object -w` and `git update-index
  --cacheinfo`), leaving the tree as it is. Tests and linters run on the end
  state, not on each commit.
- What can be verified mechanically the agent does without asking: the checks
  before the release, the release commit, the note, the image and the tag
  annotation, the checks after the push with the upgrade of this Mac, the
  issue mirror, the announcement row, `.agents/self-control.md` and the
  cleanup. It asks only for decisions: scope, wording the owner cares about,
  a criterion to change.
- Product changes first, release commit last: `chore(release): prepare vX.Y.Z`.
  A guide that explains new behavior changes with the product commit, and so
  does the agent skill in `skills/indexit/`; `go test ./skills` keeps its
  examples and command reference in step with the CLI, and
  `go generate ./skills` rewrites the reference.
- The CLI is the source of truth for what the docs, the notes and the skill
  claim.
- A published release is frozen: its tag, annotation, image and catalog entry
  stay as tagged. Its note may still get pictures or wording that keep it true
  to the tag. A missed fix ships in the next patch, or in the next candidate
  before the final tag.
- A release that changes stored data or URLs ships with the way to move to
  it, and the agent moves this Mac with it: a backup, the data directory, the
  owner's notes, a live check, then the backup goes once the owner confirms.

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
- `doctor` is green: it mints the tap and catalog tokens from the OctoLab
  Releaser App, which proves the App can publish to both. If the App, its
  installations, the `OCTOLAB_RELEASER_*` secrets or `doctor.yml` changed
  since its last run, run it again:
  `make workflow-doctor REASON="before vX.Y.Z"` dispatches it on `main` and
  waits for the result; `REASON` is optional.
- `ci` in `octolab/skills` is green: the publish action refuses an invalid
  catalog.
- `make doctor` has no `fail`.

On the candidate commit:

```sh
make verify
make go-dist-check
make release-config-check
```

`make release-config-check` accepts exactly the GoReleaser `brews`
deprecation and nothing else: `brews` ships the CLI and the macOS service as
one Formula. Existing Cask users need the one-time migration documented in
the local image guide. Published changelogs retain their original Cask
commands.

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

**Upgrade:** on macOS, run `brew update` followed by `brew upgrade --formula octolab/tap/indexit`. For a manual installation, use a [vX.Y.Z release archive](https://github.com/octopot/indexit/releases/tag/vX.Y.Z). Run `indexit version` to check the installed version.
```

A release that changes the service also says, in a bullet or the **Upgrade:**
paragraph, to run `brew services restart octolab/tap/indexit`.

- English; a short slogan ending with a period ("One message, one entry.").
- Only what users get: behavior and Telegram compatibility, no docs, tools or
  CI/CD. Explain jargon in user terms; no comparisons to release candidates.
- Links are site-relative with a trailing slash. Markdown and HTML, no MDX.
- After v0.2.0, end the **Upgrade:** paragraph with
  `If you use the agent skill, [replace it](/guide/agents/#keep-it-in-step) with the new binary’s copy.`

A prerelease note, `vX.Y.Z-rcN.md`, is short and self-contained: what to try,
what to report, and links only to pages that are already on the site, not to
the final note. Add it to `changelog/_meta.js` with `display: 'hidden'` and
give it no card. Of the list below, a prerelease changes only
`skills/indexit/SKILL.md`: the current version elsewhere stays the stable one.

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
  and the exact tag without `v` for a prerelease, such as `=0.3.0-rc1`.
  `make release-check` rejects another version, `go test ./skills` mismatched
  ranges.

```sh
grep -rn 'X\.Y\.W' README.md docs/README.md docs/content docs/public skills  # only old notes, cards and "since" facts may remain
go test ./skills
node .github/scripts/release.mjs render vX.Y.Z --site-url https://indexit.octolab.org/ --out "$(mktemp -d)/notes.md"
(cd docs && TARGET=static SITE_URL=https://indexit.octolab.org/ npx next build)
```

Check the sidebar: All releases first, then version groups with the new note;
no prerelease note in it.

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

Same scope as the note. Write it to `.agents/release/vX.Y.Z/annotation.txt`:
`.agents/` is not tracked, so it never reaches a commit.

### 4. Tag, check, push

For a prerelease, record the stable channels first:

```sh
gh release view --json tagName -q .tagName
gh api repos/octolab/homebrew-tap/contents/Formula/indexit.rb -q .sha
```

Then the maintainer runs one line, which the agent hands over with the
release commit:

```sh
git tag -s vX.Y.Z -F .agents/release/vX.Y.Z/annotation.txt && git tag -v vX.Y.Z && make release-check TAG=vX.Y.Z && git push --atomic origin main vX.Y.Z
```

Run them on `main` with the release commit checked out: `make release-check`
checks the tag against the branch you are about to push, as the `pre-push`
hook does, together with tidy modules and the GoReleaser config.

`git tag -v` must report a good signature from a key GitHub knows for your
email. The catalog publishes only from a tag GitHub verifies, so `cd` checks
that before anything is published.

If the push lock is set (`remote.origin.pushurl` is `no_push`), lift it for
this push only and set it again afterwards.

Never push the tag separately. A tag pushed by mistake can be deleted only
before the GitHub release exists; after that, ship the next patch.

### 5. After the push

The agent runs this section by itself once the push is done and reports what
it saw.

```sh
gh run watch --exit-status "$(gh run list -w cd.yml -c "$(git rev-parse 'vX.Y.Z^{commit}')" -L 1 --json databaseId -q '.[0].databaseId')"
gh release view vX.Y.Z --json name,isPrerelease,assets -q '.name, .isPrerelease, .assets[].name'
gh api repos/octolab/homebrew-tap/commits/main -q '.commit.verification.verified, .commit.message'
gh api repos/octolab/skills/commits/indexit--vX.Y.Z -q '.commit.verification.verified, .commit.message'
gh run list -R octolab/skills -w ci.install.yml -b indexit--vX.Y.Z -L 1
```

- `cd` is green; for a prerelease, its skill job is skipped. If `gh run watch`
  says the run is not found, `cd` hasn't started yet: run the line again.
- The release has the note's title and body, four archives and
  `checksums.txt`.
- A stable release: the Formula commit for vX.Y.Z in the tap is verified,
  signed as the App.
- A prerelease: `isPrerelease` is `true`,
  `gh release view --json tagName -q .tagName` still names the previous stable
  release, and the Formula has the recorded `sha`.
- On the first Formula release, retire `Casks/indexit.rb` in `octolab/homebrew-tap`
  as a separate reviewed maintainer change, after the Formula is available.
  GoReleaser creates the Formula but does not delete the old Cask.
- A stable release only: the catalog tag `indexit--vX.Y.Z` points at a
  verified commit, and `ci (install)` there is green for that tag.
- `brew update && brew upgrade --formula octolab/tap/indexit && indexit version`
  shows vX.Y.Z.
- A stable release only: the catalog plugins are updated, then
  `indexit skill status --agent claude-code,codex --exact` passes:

  ```sh
  claude plugin marketplace update octolab && claude plugin update indexit@octolab
  codex plugin marketplace upgrade octolab && codex plugin add indexit@octolab
  ```

  The skill comes from the catalog, not from `indexit skill install`; `ci`
  checks that install on every change. A copy that `status` reports next to
  the plugin is a leftover: remove it, the agent would load both.
- The site shows the new changelog page and release band once `cd.docs`
  finishes for the release commit.
- On this Mac: `brew services restart octolab/tap/indexit` (or `start` when it
  was stopped), `/` answers, the photographs of the owner's notes are served
  with no `telegram: connecting` in `$(brew --prefix)/var/log/indexit.log`,
  and the change of the release works with the installed binary.
- Criteria ticked by the release commit go to GitHub
  (`maintainer.py sync --only <N> --yes`); the server fields it brings back
  are the next `chore(project): sync the issue mirror` commit.
- What the owner has to see himself, such as a note in Obsidian or the
  dashboard in a browser, goes to `.agents/self-control.md`.

### When `cd` fails

- **Before the GitHub release exists** (the tag checks, tests or the build;
  `gh release view vX.Y.Z` finds no release): nothing is published. Delete
  the tag with `git push --delete origin vX.Y.Z && git tag -d vX.Y.Z`, fix the
  cause on `main` or in your signing setup, and go back to step 4.
- **A prerelease reached a stable channel** (*Latest* or the tap changed):
  restore the previous stable release as *Latest* and the Formula, then
  file an issue before the next candidate.
- **Only the skill job failed**: the release and the Formula are out and stay.
  Fix the cause, for example the App's installation on `octolab/skills`, and
  run the failed job again: `gh run rerun <run-id> --failed`.
- **Anything else after the release exists**: it is frozen; ship the fix in
  the next patch.

### 6. Announcement

Add a row for "Latest contributions" in the
[kamilsk/kamilsk](https://github.com/kamilsk/kamilsk) profile README, in the
checkout `~/Development/public/kamilsk/kamilsk`, stage it with the message
`chore: add new contribution log entry` and hand over its `git contrib`:

```md
| 2026-10-02   | 🗃️ [indexit][]              | [v0.1.1][oi011], [v0.1.2][oi012]  | Telegram exports without duplicates       |

[oi012]:  https://github.com/octopot/indexit/releases/tag/v0.1.2
```

Newest first, one row per release day; a prerelease gets no row. The
reference label is `o` + project
initials + version digits. Notes is a short phrase, no longer than the
existing ones, saying what the user gets.

### 7. Self-control and cleanup

`.agents/self-control.md` holds the checks that outlive a session: each item
says when to look, what proves it and what to do if it fails. The agent adds
the checks a release leaves open, closes an item with what proved it and
removes it once proven, and keeps the follow-ups that still need an issue.

The drafts of a release live in `.agents/release/vX.Y.Z/` (plan, note,
annotation, migration). After the checks of step 5 the agent removes them,
its scratch services, builds and test folders, and any backup the owner has
confirmed.
