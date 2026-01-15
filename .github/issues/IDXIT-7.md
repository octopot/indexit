---
code: IDXIT-7
id: I_kwDOSVi84s8AAAABUksSzA
databaseId: 5675619020
number: 108
url: https://github.com/octopot/indexit/issues/108
title: "ci/cd: sign the Cask commits in the Homebrew tap"
labels:
  - "effort: medium"
  - "impact: low"
  - "scope: inventory"
  - "type: improvement"
milestone:
state: OPEN
stateReason:
createdAt: 2026-10-02T09:37:34Z
updatedAt: 2026-10-02T09:59:57Z
lastEditedAt:
closedAt:
---

# ci/cd: sign the Cask commits in the Homebrew tap

## Motivation

Every release pushes a Cask update to [octolab/homebrew-tap](https://github.com/octolab/homebrew-tap/commits/main/),
and every such commit is shown as unverified, while the commits made by hand
in the same repository are verified:

```sh
gh api 'repos/octolab/homebrew-tap/commits?per_page=5' \
  --jq '.[] | "\(.sha[0:8]) \(.commit.verification.reason) \(.commit.message)"'
# 30a1801d unsigned Brew cask update for indexit version v0.1.2
# ee1a6535 unsigned Brew cask update for indexit version v0.1.1
# dc28879a unsigned Brew cask update for maintainer version v0.1.0
# 5e55d213 unsigned Brew formula update for maintainer version v0.1.0
# 05386550 valid    chore: dump github projects
```

GoReleaser v2.18.2 writes the Cask through the GitHub contents API with
`HOMEBREW_TAP_TOKEN`, a personal access token, and passes `commit_author` as
the committer. GitHub signs an API commit only when the request carries no
custom committer, and the documentation promises it for bots, that is GitHub
Apps ([signature verification for bots](https://docs.github.com/en/authentication/managing-commit-signature-verification/about-commit-signature-verification#signature-verification-for-bots)).

GoReleaser offers two ways out:

- `commit_author.use_github_app_token: true` (v2.13+) omits the committer, so
  GitHub signs the commit as the authenticated GitHub App. The author becomes
  the app's bot account.
- `repository.git` with `commit_author.signing` (v2.11+) clones the tap over SSH
  and signs the commit with a GPG, SSH or x509 key. The author stays a person,
  but CI holds a write deploy key and a signing key valid for that person.

The same tap receives octomation/maintainer releases, so the decision applies
to both.

## Out of scope

- Signing release archives or macOS binaries (notarization).
- Rewriting the existing unsigned commits in the tap.

## Acceptance criteria

- [ ] The Cask commit of the next release is `verified: true` on GitHub.
- [ ] The tap token is scoped to `octolab/homebrew-tap` with contents write
      and no longer outlives the release job.
- [ ] `release.mjs preflight` and `doctor` check the new credentials and say
      how to create them.
- [ ] `.github/workflows/README.md` documents the secrets.

<!-- 2026-10-02T09:28Z https://github.com/octopot/indexit/issues/108#issuecomment-5949368603
Experiment 1: `use_github_app_token: true` with the personal access token. Did not work.

The flag only drops the committer from the contents API call, so it looked worth a try without a GitHub App: if GitHub filled in the committer itself, it might sign the commit as it does for web edits. v0.1.2 shipped with the flag ([84974f5](https://github.com/octopot/indexit/commit/84974f5), [run 36988933044](https://github.com/octopot/indexit/actions/runs/36988933044)) and `HOMEBREW_TAP_TOKEN` unchanged.

```text
30a1801d Brew cask update for indexit version v0.1.2
  author:    Kamil Samigullin <kamilsk@users.noreply.github.com>
  committer: Kamil Samigullin <kamilsk@users.noreply.github.com>
  verification: unsigned
ee1a6535 Brew cask update for indexit version v0.1.1   (before the flag)
  author:    Kamil Samigullin <kamil@samigullin.info>
  committer: Kamil Samigullin <kamil@samigullin.info>
  verification: unsigned
```

GitHub took both author and committer from the token owner and did not sign. A side effect: `commit_author.name` and `email` are ignored in this mode, so the author email turned into the noreply address.

Conclusion: a user token is not enough; the flag needs a GitHub App installation token. Next: a GitHub App installed on octolab/homebrew-tap, a token from `actions/create-github-app-token` in `cd.yml`, the flag stays.
-->

<!-- 2026-10-02T09:59Z https://github.com/octopot/indexit/issues/108#issuecomment-5949807079
Experiment 2: GitHub App installation token. Preparation done, awaiting the first run.

**The App.** [OctoLab Releaser](https://github.com/organizations/octolab/settings/apps/octolab-releaser) (`octolab-releaser`, app ID 5161111), owned by octolab: no webhook events, repository permissions Contents: write and Metadata: read. Its Client ID and a private key are stored as Actions secrets `HOMEBREW_TAP_APP_CLIENT_ID` and `HOMEBREW_TAP_APP_KEY`, next to the old `HOMEBREW_TAP_TOKEN`.

**The workflows** (pending commit "fix(ci/cd): mint the tap token from a GitHub App, refs #108"):
- `cd.yml`: `release.mjs preflight` checks both secrets and outputs the tap owner and name read from `.goreleaser.yml`; `actions/create-github-app-token@v3.2.0` then mints a token for that repository only with `permission-contents: write`, and goreleaser gets it as `HOMEBREW_TAP_TOKEN`. All before Go is set up, so a broken App fails the release in seconds.
- `doctor.yml`: the same preflight and minting step, so the daily run proves the App is still installed on the tap and may write to it.
- `release.mjs`: `preflight` and `doctor` know the App secrets and print how to create the App when one is missing; the PAT is no longer expected.
- `.goreleaser.yml`: `use_github_app_token: true` stays; `commit_author.name` and `email` are dropped, as in this mode the author is the App's bot account.
- `.github/workflows/README.md`: the secrets table and both flows.

Checked locally: `goreleaser check`, `cue vet`, `actionlint`, and `preflight` with and without the secrets.

**Not verified yet:**
- The installation: at the time of writing `gh api /orgs/octolab/installations` lists no `octolab-releaser`. It has to be installed on octolab/homebrew-tap only.
- The secrets: they cannot be listed without `admin:org`.
- The [doctor run 36992702266](https://github.com/octopot/indexit/actions/runs/36992702266) ran the old workflow on 1c26ba8 and only saw the PAT.

**Next:** after the commit lands on main, `gh workflow run doctor.yml` must mint the token; the next release's Cask commit must be `verified: true` with `octolab-releaser[bot]` as its author.

The PAT `HOMEBREW_TAP_TOKEN` stays: octomation/maintainer still pushes its Cask and formula with it and moves to the App separately.
-->
