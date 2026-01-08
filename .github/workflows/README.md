# GitHub Actions

All external actions use concrete release tags. The version history, migration
notes and verification results are in [the audit report](../reports/20260918-actions-audit.md).
Dependabot checks the `github-actions` ecosystem daily; review upstream changes
and the action contract before accepting an update.

| Workflow | Trigger | Purpose |
| --- | --- | --- |
| [ci.yml](ci.yml) | PR, main push, version tag, monthly, manual | Lint, Go tests, coverage artifact and Codecov |
| [cd.yml](cd.yml) | Version tag, manual | Test and publish tag releases; build a snapshot on manual branch runs |
| [cd.docs.yml](cd.docs.yml) | Documentation PR/main push, monthly, manual, reusable | Build documentation; deploy only from main outside PRs |
| [tools.yml](tools.yml) | Tools PR/main push, monthly, manual | Install tools and check generation |
| [cleanup.caches.yml](cleanup.caches.yml) | Monthly, manual, reusable | Delete caches with the built-in GitHub CLI |
| [warmup.caches.yml](warmup.caches.yml) | Cache cleanup completion, manual | Warm Go, docs and tools caches |
| [cleanup.runs.yml](cleanup.runs.yml) | Monthly, manual, reusable | Delete completed runs without age/count retention; manual dry-run supported |
| [cleanup.stale.yml](cleanup.stale.yml) | Daily, manual, reusable | Mark/close stale issues and PRs |

GitHub-hosted Ubuntu runners supply the runtime required by the current actions.
Workflow permissions default to `contents: read`; publishing and maintenance jobs
request their additional permissions explicitly.

- Releases publish to this repository and update `Casks/indexit.rb` in
  `octolab/homebrew-tap`. `GORELEASER_TOKEN` must allow release publishing here
  and content writes to the tap. After the first Cask release, install with
  `brew install --cask octolab/tap/indexit`.
  macOS signing/notarization is not configured yet; Gatekeeper may block the binary.
- Codecov uses GitHub OIDC. The repository must be activated in Codecov; no
  `CODECOV_TOKEN` is required by the workflow.
- Pages must use **GitHub Actions** as its build source. The `github-pages`
  environment must permit main-branch deployments.
- To clear run history, run **Workflow runs cleanup** with `pattern: All` (the
  default) and `dry_run` unchecked. Scheduled and reusable runs also target all
  workflows, including Dependabot. Active runs remain, including cleanup itself.
  The upstream action also deletes orphaned runs whose workflows no longer exist,
  regardless of their status or the selected workflow.
- `SLACK_WEBHOOK` is optional: an empty value skips sending notifications.
