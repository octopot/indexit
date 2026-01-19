# indexit documentation

An English, use-case-led site for the v0.2.1 feature set, built on the existing Nextra 4 / Next.js stack.

## Run locally

Use Node 24 (the CI version). From the repository root:

```sh
./Taskfile docs npm ci
./Taskfile docs dev
```

Open the local URL printed by Next.js, normally <http://localhost:3000>.

Or, from `docs/`, run `npm ci` and `npm run dev`.

## Build

For a production server:

```sh
./Taskfile docs build
./Taskfile docs start
```

For GitHub Pages:

```sh
TARGET=static SITE_URL=https://indexit.octolab.org/ ./Taskfile docs build
```

Static output is written to `docs/dist/`. `SITE_URL` is required; add `BASE_PATH=/indexit` only when hosting under a path, as on the default `octopot.github.io/indexit/`. The [Pages workflow](../.github/workflows/cd.docs.yml) supplies both automatically. Local development remains available while the static build runs.

## Edit the content

| Location | Purpose |
| --- | --- |
| `content/index.mdx` | Product introduction and paths into the guides |
| `components/workflow-demo.jsx` | Interactive fx and fzf illustrations using fictional sample data |
| `components/demo-data.mjs` | Sample records and simple matching helpers for the illustrations |
| `content/quick-start.mdx` | Agent setup entry point and manual quick start |
| `public/quick-start.md` | Plain Markdown instructions for agents; keep commands in sync with the quick start |
| `components/agent-setup.jsx` | Copyable prompt linking to the agent instructions at the configured site URL |
| `content/guide/` | Practical workflows, setup, and troubleshooting |
| `content/changelog/index.mdx` | Release overview |
| `content/changelog/v0.2.1.md` | Latest release notes |
| `content/changelog/v0.2.0.md` | Channel cards, invite previews, and agent skill release notes |
| `content/changelog/v0.1.2.md` | Message and media deduplication release notes |
| `content/changelog/v0.1.1.md` | Dialog deduplication release notes |
| `content/changelog/v0.1.0.md` | First stable release notes |
| `content/**/_meta.js` | Navigation order and labels |
| `navigation.mjs` | Release groups in the sidebar; keeps published release URLs unchanged |
| `app/globals.css` | Shared colors, responsive layouts, and landing page styles |
| `app/layout.jsx` | Site identity and Nextra theme |
| `app/[[...mdxPath]]/page.jsx` | Page-specific titles and social metadata |
| `public/` | Favicon and social preview |

The public origin in metadata comes from `SITE_URL` (see `site.mjs`); the Pages workflow sets it from the Pages configuration. Internal links use Next.js/Nextra and keep the configured base path.

Quick start lives at `/quick-start/`. The sidebar groups the quick start with the guide pages through `_meta.js`.

Changelog is a section of the same sidebar, with All releases first. `navigation.mjs` groups release pages into expandable `v0.1.x`, `v0.2.x`, and later branches, newest first. Add each release note and its label to `content/changelog/_meta.js` as before; the version group is created automatically. Keep the notes at `content/changelog/{tag}.md` so published links and the release workflow stay valid. Top-level navbar links use separate `type: 'page'` aliases; content folders use `display: 'children'` to keep the full sidebar available across guides and releases.

## Change the domain

The site is served from the custom domain `indexit.octolab.org`; `octopot.github.io/indexit/` redirects to it, keeping the path. The build bakes the domain into asset paths and metadata, and changing it in Settings → Pages triggers no rebuild. To change it:

1. Update `pages.cname` in [`.github/settings.json`](../.github/settings.json) (remove it for the default domain) and push.
2. Change Settings → Pages → Custom domain to match.
3. Rebuild: `gh workflow run cd.docs.yml -f reason="domain change"`.

The docs build stops while the two disagree, the deployment is smoke-tested, and the daily [doctor](../.github/workflows/doctor.yml) reports a stale site.

## Keep the release accurate

The CLI implementation is the source of truth. Specs in `.github/notes/` include future work and older behavior; closed issues provide context, not a substitute for checking the current code.

The documentation is prepared for v0.2.1. Publish the matching release archives before directing users to its download links. Lead installation instructions with Homebrew and release binaries; keep source builds as an optional contributor workflow. Lead usage examples with direct pipes into fx, jq, or fzf; avoid intermediate JSONL files. Use readable public usernames and Telegram links in commands, keeping numeric UID grammar in the reference. Archive names and supported platforms must match `.goreleaser.yml`. Do not describe planned features as shipped.

Release notes describe changes to indexit's behavior and Telegram compatibility. Keep documentation, development tools, CI/CD, and repository inventory changes out of the product changelog. Preserve historical release notes when updating the current installation instructions.

Maintain these details near the relevant examples:

- The output flag reference explains that `-o` appends across runs; primary examples work directly with stdout.
- History/media commands ignore a message anchor as a cursor.
- Use an explicit topic UID when selecting a forum topic.
- Media failures can be recorded in the manifest even on exit code 0.
- Optional JSON fields may be absent; IDs can require 64-bit precision.

No live-account export is needed to build the docs. Check examples against command help, the UID parser, and existing Go tests; keep real account data out of examples.

`package.json` pins Zod to `4.1.12` under `nextra-theme-docs` to avoid [a validation regression](https://github.com/shuding/nextra/issues/5036). In the current dependency tree, this also covers the theme's Nextra peer dependency. Keep the exact Zod version: a caret range allows incompatible releases. When upgrading Nextra and its theme, check whether the site builds without this override. See [Nextra’s docs theme guide](https://nextra.site/docs/docs-theme/start) for the underlying structure.

For `speech-rule-engine@4.1.4`, an override replaces its pinned xmldom dependency with `@xmldom/xmldom@^0.9.12` to include the security fixes while allowing later `0.9.x` patches. The lockfile records the resolved version. This override stops applying when speech-rule-engine changes version; run `npm audit` after that update and remove or revise the rule as needed.

The interactive illustrations run entirely in the browser on fictional data. They demonstrate exploration and selection, not an embedded copy of fx or fzf. The fx illustration uses literal text search and simple subsequence path matching; the native tools provide the full search syntax. No CLI commands or account connections are executed by these components.
