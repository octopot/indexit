# indexit documentation

An English, use-case-led site for the v0.1.0 feature set, built on the existing Nextra 4 / Next.js stack.

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
TARGET=static BASE_PATH=/indexit SITE_URL=https://octopot.github.io/indexit/ ./Taskfile docs build
```

Static output is written to `docs/dist/`. Omit `BASE_PATH` for hosting at the domain root. The [Pages workflow](../.github/workflows/cd.docs.yml) supplies the path automatically. Local development remains available while the static build runs.

## Edit the content

| Location | Purpose |
| --- | --- |
| `content/index.mdx` | Product introduction and paths into the guides |
| `components/workflow-demo.jsx` | Interactive fx and fzf illustrations using fictional sample data |
| `components/demo-data.mjs` | Sample records and simple matching helpers for the illustrations |
| `content/guide/` | Quick start, practical workflows, setup, and troubleshooting |
| `content/changelog/index.mdx` | Release overview |
| `content/changelog/v0.1.0.md` | Stable v0.1.0 release notes |
| `content/**/_meta.js` | Navigation order and labels |
| `app/globals.css` | Shared colors, responsive layouts, and landing page styles |
| `app/layout.jsx` | Site identity and Nextra theme |
| `app/[[...mdxPath]]/page.jsx` | Page-specific titles and social metadata |
| `public/` | Favicon and social preview |

The public origin in metadata comes from `SITE_URL` (see `site.mjs`); the Pages workflow sets it from the Pages configuration, so a domain change needs no edits. Internal links use Next.js/Nextra and keep the configured base path.

## Keep the release accurate

The CLI implementation is the source of truth. Specs in `.github/notes/` include future work and older behavior; closed issues provide context, not a substitute for checking the current code.

Public documentation describes v0.1.0 as the released stable version. Lead installation instructions with Homebrew and release binaries; keep source builds as an optional contributor workflow. Lead usage examples with direct pipes into fx, jq, or fzf; avoid intermediate JSONL files. Use readable public usernames and Telegram links in commands, keeping numeric UID grammar in the reference. Archive names and supported platforms must match `.goreleaser.yml`. Do not describe planned features as shipped.

Maintain these details near the relevant examples:

- The output flag reference explains that `-o` appends across runs; primary examples work directly with stdout.
- History/media commands ignore a message anchor as a cursor.
- Use an explicit topic UID when selecting a forum topic.
- Media failures can be recorded in the manifest even on exit code 0.
- Optional JSON fields may be absent; IDs can require 64-bit precision.

No live-account export is needed to build the docs. Check examples against command help, the UID parser, and existing Go tests; keep real account data out of examples.

`package.json` pins Zod to `4.1.12` for Nextra and its theme to avoid [a validation regression](https://github.com/shuding/nextra/issues/5036). Revisit those overrides when upgrading Nextra. See [Nextra’s docs theme guide](https://nextra.site/docs/docs-theme/start) for the underlying structure.

The interactive illustrations run entirely in the browser on fictional data. They demonstrate exploration and selection, not an embedded copy of fx or fzf. The fx illustration uses literal text search and simple subsequence path matching; the native tools provide the full search syntax. No CLI commands or account connections are executed by these components.
