---
uid: 9ba2ce38-02f5-42be-a82d-0ade64ed64ff
timestamp: 2026-09-18T15:46:39.061063+00:00
model: claude-fable-5-1
---
# Plan review — Claude 5.1 Fable

CLI model: `claude-fable-5-1`; effort: `high`. The review used the supplied plan and coordinator-provided facts, without independent file access.

This is an English translation of the original Russian review. The reviewer's assessments and proposals are preserved below; the coordinator's decisions follow.

## GitHub Actions audit plan review

### Significant gaps

**1. The gap between enabling workflows now and leaving remote code unchanged is not addressed.** Step 7 enables workflows before the user commits the indexit edits. Scheduled workflows will run the old default-branch code, with floating references, missing reusable targets, empty secrets, and unconfigured Pages: guaranteed failed runs. `push`/`pull_request` are safe because they execute the committed version; `schedule` and `workflow_run` are not. The plan needs to classify all eight workflows by trigger and make an explicit decision: enable scheduled workflows only after the user's commit, or accept the expected failures and document them.

**2. Empty secrets and missing Pages configuration have not been translated into configuration requirements.** This affects Codecov, Slack, GoReleaser (`GITHUB_TOKEN` is usually sufficient, but not for Homebrew/tap/announce), and Pages deployment. Decide for each integration whether to use (a) conditional execution through an environment variable (`secrets` cannot be referenced directly in a job-level `if`; use `env: HAS_TOKEN: ${{ secrets.X != '' }}`), (b) `continue-on-error`, or (c) an explicit list of required secrets/settings in the report. For Pages, decide whether to add `actions/configure-pages` with `enablement: true` (requiring permissions and the Pages API) or document that manual setup is required.

**3. “Every stable release, including patches” is neither bounded nor defined.** Actions such as `github/codeql-action` and `goreleaser/goreleaser-action` have dozens or hundreds of patch tags. Define eligibility formally: a GitHub Release that is neither a prerelease nor a draft and is semantically at or above the initial version; account for tags without a Release separately as “tag-only.” If the initial reference is a major alias (`v4`), define how to resolve it to an exact version: use the date of commit `df1e20a3` and `git rev-parse` on a clone from that time, or release history. The initial version of an alias reference is currently undefined.

**4. The count of 14 is not final.** Transitive `uses` inside composite actions and reusable workflows will expand the inventory. The plan mentions this, but assigns work before freezing the inventory. Add an “inventory frozen, transitive dependencies expanded to depth N” step before step 3, with a deduplication rule for shared dependencies such as `actions/checkout`.

**5. The indexit consumer submodule might not be fetchable.** A gitlink to `ba90772f…` is valid only if that commit exists on remote origin. If HEAD is local and unpushed, the submodule will be broken. Check `git branch -r --contains ba90772` before adding it.

**6. There is no shared evidence format for the three parallel workers.** Without a defined schema containing tag, SHA, release date, `runs.using`, input/output changes, breaking changes, migration notes, and indexit impact, results will be inconsistent and require coordinator rewrites. Define the schema before dispatching step 3.

**7. The pinning strategy is undecided.** “No floating major-only refs” allows both a tag such as `v4.2.1` and a SHA. The recommended practice is a SHA with a version comment; this also requires Dependabot/Renovate coverage for the `github-actions` ecosystem to prevent the pins from becoming stale. The plan does not mention `.github/dependabot.yml`.

**8. Actionlint does not cover local input validation against arbitrary manifests.** It checks syntax and some known actions, but not arbitrary manifests. Add an explicit tool/script that compares `with:` against `inputs:` in `action.yml` at the exact SHA, and checks that `runs.using` is neither `node16` nor `node12`, which have been removed from the runner.

**9. Cleanup dry-run** depends on the GitHub API and token. Only its syntax can be checked locally; mark it as requiring a runner in the report.

### Proposed adjustments

- Step 1: add alias-to-exact-version resolution at the import date and record the method.
- New step 1b: freeze the inventory after expanding transitive dependencies; add an “action → workflow/job/step” usage table.
- Step 3: provide workers with the evidence schema and deduplication rule. Bound the depth: patch releases without manifest changes may use a single “no contract change” line, but must list their SHAs.
- Step 5: add decisions on secrets/Pages (item 2), the pinning strategy, and dependabot.yml; check `ubuntu-latest` → 24.04 compatibility and the absence of node16 actions.
- Step 7: split enablement into two groups by trigger. Enable scheduled workflows at the coordinator's discretion while recording expected failures; then verify `state == "active"` through the API for all eight.
- Step 8: show `git status --porcelain` and `git diff --stat` alongside HEAD so the user can see which files to commit. State that the first real run should follow the user's commit through `workflow_dispatch`.

### Additional acceptance criteria

- A frozen inventory of N action dependencies (≥14), including transitive origins; every alias reference has a documented resolved initial version and method.
- Every evidence record follows a shared schema; no releases are missing between adjacent entries.
- Every external action `uses` reference is a SHA with a version comment or an exact tag under the recorded policy; `dependabot.yml` covers `github-actions`.
- No actions use `runs.using: node12|node16`; runner labels are not deprecated.
- Every secret/integration is classified as required, conditional, or disabled; empty-value behavior is checked at the YAML level.
- A workflow/trigger table records expected behavior after enablement and before the user's commit.
- All eight workflows are `state: active` according to the API; the report states that remote code is unchanged.
- The indexit consumer submodule can be fetched from origin.
- Indexit HEAD is `ba90772f…`; the working tree contains only the workflow/configuration edits listed in the report.

## Coordinator decisions

- Accepted contract validation, complete tag coverage, transitive uses, remote-state verification, and the distinction between local changes and published code. Historical indexit versions are exact, so major-alias reconstruction is unnecessary. Commit dates do not prove historical resolution of mutable tags.
- Use exact version tags as requested by the user; record SHAs in evidence. Dependabot already covers github-actions.
- Use OIDC for Codecov; send Slack notifications only with a nonempty webhook; retain GORELEASER_TOKEN for releases and the existing octolab/homebrew-tap, as clarified by the maintainer during commit review; publish indexit as a Cask using homebrew_casks, following the maintainer's subsequent migration request. Pages was configured through the API with `build_type: workflow`; no deployment was run.
- The consumer SHA was fetched from origin through a fresh clone and checkout; gitlink reproducibility was verified.
- Keep a separate Actions inventory, evidence, and per-dependency changelog. A shared coordinator verifier checks differing supplementary fields; the existing Go pilot remains unchanged.
- The user explicitly requested enabling all eight workflows. Enable them after local checks, without dispatching runs, and state that GitHub will retain the old code until the user's commit. Old scheduled workflows may fail or perform their previous operations. Enablement is not proof of successful CI.
- The first CLI attempt without a dedicated system prompt was rejected as an invalid review because its output contained imitation tool calls. Only the second response, containing a text review, is accepted.
