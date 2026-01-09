> # 🗃️ indexit
> 
> It's all indexed.

**Your Telegram. Your data.**

Export conversations as JSONL, fetch individual messages by link, and save photos and files from chats or forum topics. One Go CLI, using your Telegram account.

## Install

On macOS with [Homebrew](https://brew.sh/):

```sh
brew install --cask octolab/tap/indexit
indexit version
```

For Linux or a manual installation, download a [v0.1.0 release archive](https://github.com/octopot/indexit/releases/tag/v0.1.0) for your platform. Ready-to-run binaries are available for macOS and Linux on `amd64` and `arm64`; Go is not required. See the [installation steps](docs/content/guide/index.mdx#1-install-indexit).

## Quick start

```sh
brew install fx
indexit telegram auth login --qr
indexit -q telegram fetch messages --dialog @example_channel --limit 200 | fx
```

Set up your API credentials first and replace the example channel with your own. In fx, expand a message, press `/` to search its contents, or `@` to find a field. No intermediate file is needed.

- [Quick start](docs/content/guide/index.mdx) — install, sign in, and explore your first messages.
- [Explore & find](docs/content/guide/explore.mdx) — interactive fx and fzf workflows, with browser demos.
- [Messages](docs/content/guide/messages.mdx) — history, date windows, and exact links.
- [Topics & media](docs/content/guide/media.mdx) — bring a trip archive home.
- [v0.1.0 release notes](docs/content/changelog/v0.1.0.md) — the first stable release.

v0.1.0 focuses on Telegram ingestion. Built-in search, contacts, Stories, and automatic synchronization are future work.

## Development

<details>
<summary>Build from source (optional)</summary>

To work on indexit itself, clone the repository and build it with Go 1.27 or newer:

```sh
git clone https://github.com/octopot/indexit.git
cd indexit
go build -o ./bin/indexit .
./bin/indexit version
```

</details>

## Documentation site

```sh
./Taskfile docs npm ci
./Taskfile docs dev
```

See [docs/README.md](docs/README.md) for the site structure, production build, and static export.

---

<p align="center">
  <sub>indexit 🗃️</sub><br>
  <sub>MIT · Built by <a href="https://github.com/octolab">OctoLab.</a></sub>
</p>
