# Set up indexit with your user

Help the user install indexit v0.2.0, connect their Telegram account, and explore one conversation they choose. indexit is a macOS/Linux CLI that exports Telegram data as JSONL. It uses a user account, not a bot token. Go and GitHub credentials are not required for installation from a release.

## 1. Check the machine and install

Check `uname -s`, `uname -m`, and whether `indexit` is already available. If it is, run `indexit version` before deciding whether installation is needed. This guide describes v0.2.0; consult the installed command's `--help` if the version differs.

On macOS with Homebrew:

```sh
brew install --cask octolab/tap/indexit
indexit version
```

Otherwise, use the official release: https://github.com/octopot/indexit/releases/tag/v0.2.0

Map `Darwin` to `darwin`, `Linux` to `linux`, `x86_64` to `amd64`, and `arm64` or `aarch64` to `arm64`. The four supported archives are:

- `indexit_0.2.0_darwin-arm64.tar.gz`
- `indexit_0.2.0_darwin-amd64.tar.gz`
- `indexit_0.2.0_linux-amd64.tar.gz`
- `indexit_0.2.0_linux-arm64.tar.gz`

Download the matching archive and `checksums.txt` from `https://github.com/octopot/indexit/releases/download/v0.2.0/`. Verify the archive's SHA-256 against its entry in `checksums.txt` using `shasum -a 256` on macOS or `sha256sum` on Linux. Stop if they differ. Extract in a temporary directory, then copy the binary into a user-owned directory on PATH. For Linux x86-64, after downloading and verifying:

```sh
tar -xzf indexit_0.2.0_linux-amd64.tar.gz
mkdir -p "$HOME/.local/bin"
install -m 755 indexit "$HOME/.local/bin/indexit"
export PATH="$HOME/.local/bin:$PATH"
indexit version
```

`install` copies the binary and sets its permissions. `-m 755` makes it executable. This destination does not need `sudo`. Explain how to keep the PATH entry in the user's shell configuration if it is missing. If the OS or architecture is unsupported, report that instead of choosing an incompatible archive.

## 2. Set up credentials locally

Use an agreed working directory for subsequent commands: indexit loads `.env` from the current directory by default. Preserve any existing configuration.

Ask the user to create an application at https://my.telegram.org/apps and enter its `api_id` and `api_hash` in the local `.env` file using their editor:

```dotenv
TELEGRAM_API_ID=123456
TELEGRAM_API_HASH=replace_with_your_api_hash
```

These are placeholders, not usable credentials. The user should enter the real values locally, not paste them into chat. Keep `.env` private and out of version control. Do not print credentials or session file contents. Telegram's setup guide is https://core.telegram.org/api/obtaining_api_id.

## 3. Let the user complete sign-in

Check for an existing authorized session with:

```sh
indexit telegram auth status
```

If sign-in is needed, have the user run this in a visible interactive terminal from the configured directory:

```sh
indexit telegram auth login --qr
```

On the phone: Telegram → Settings → Devices → Link Desktop Device. The user scans the QR and enters a 2FA password in the terminal if prompted. If your environment cannot show an interactive terminal, give the user the command and wait for them to complete it. Confirm with `indexit telegram auth status` afterward.

The session is reused on later runs. Its default location is `~/.config/indexit/telegram/session.json`; the peer cache is `~/.cache/indexit/telegram/peers.json`. XDG_CONFIG_HOME and XDG_CACHE_HOME override their respective base directories. Existing session files must have mode `0600`.

## 4. Explore one conversation

Ask which conversation the user wants to explore. For a public channel or group, use its actual `@username` or `https://t.me/username` link. Do not run placeholder examples unchanged.

For an interactive view, install fx if needed (on macOS: `brew install fx`; other platforms: https://fx.wtf/install), then have the user run:

```sh
indexit -q telegram fetch messages --dialog @example_channel --limit 200 | fx
```

Substitute the chosen conversation. In fx, the arrow keys expand records, `/` searches contents, and `@` finds a JSON path. For private conversations, let the user select a returned `uid` from:

```sh
indexit -q telegram fetch dialogs | fx
```

Listing dialogs also fills the local peer cache needed for private/numeric addresses. Interactive viewers need the user's terminal; do not launch fx in a headless runner. If the user wants you to process JSONL directly, use a small explicit limit and the conversation they chose.

indexit ships an agent skill for its own version. Offer to install it for the agent the user works with, and run it only with their consent:

```sh
indexit skill install --agent claude-code
```

Use `--agent codex` for Codex. The skill loads in a new agent session.

Report the installed version, whether sign-in succeeded, and the command to open the chosen conversation. If a step is blocked, say which step needs attention. Do not claim setup or an export succeeded without checking.

## Further documentation

Resolve these paths against the documentation site that served this file (including any hosting prefix):

- `quick-start/`: the same onboarding journey with manual steps.
- `guide/authentication/`: phone-code login, account files, and logout.
- `guide/proxy/`: connection setup if Telegram is unreachable.
- `guide/explore/`: fx and fzf workflows.
- `guide/peers/`: channel descriptions, linked discussion groups, and invite previews.
- `guide/reference/`: flags, address formats, configuration, and exit codes.
- `guide/agents/`: the agent skill for Claude Code and Codex.

Search in fx or fzf only covers the fetched records. v0.2.0 exports data; built-in indexing and search are future work.
