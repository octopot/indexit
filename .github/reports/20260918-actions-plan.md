---
uid: 4f104bd7-ad60-4741-9c59-6d5dab579b5b
timestamp: 2026-09-18T12:58:56.545422+00:00
model: gpt-6-astra
---
# GitHub Actions audit plan

This records the pre-execution plan. The [final report](20260918-actions-audit.md) includes subsequent scope clarifications and maintainer commits from paired review.

Starting state: `ba90772f124c6d666882c13494e0528d9bb9b129`; the indexit and changelog working trees are clean.
Workflows first appear in `df1e20a3f0d119edfaf913365982b5f5c2025f57` (the go-tool import). Research covers indexit's own history, including later additions and local composite actions.
All eight YAML workflows on GitHub are `disabled_manually`.

## Sequence

1. Freeze the original action references, their change history, HEAD, and a shared upstream cutoff. Check every stable release/tag, including patches, from the initial version to the latest available version; do not count moving major aliases as separate releases.
2. Discuss this plan with Claude 5.1 Fable at high effort; preserve the feedback and coordinator decisions.
3. Divide the 14 dependencies among at most three gpt-5.6-luna max agents. For each dependency, preserve its source as a research submodule in octolaba/changelog, exact tag SHAs, release metadata, manifests/action contracts, adjacent comparisons, migration requirements, and impact on indexit. Add indexit as a consumer submodule. A version list or a single aggregate major-version diff is not a substitute for analysis.
4. The coordinator checks sequence completeness and evidence, then integrates the material using changelog's conventions: inventory, evidence, results, and state. Keep the Actions extension separate from the frozen Go pilot, without inventing approval decisions.
5. Adapt all workflows to the latest exact stable versions. Check transitive uses, runtimes/runners, inputs/outputs, permissions, reusable workflows, artifacts, Pages, Codecov, Slack, caches, the linter, and GoReleaser. Fix related configuration where necessary. Keep PR validation separate from publishing; check cleanup dry-run behavior.
6. Run actionlint, check references and local paths, and compare inputs with upstream manifests. Perform available local linter, build, documentation, and GoReleaser checks without publishing or sending notifications. Record exact results and limitations.
7. Write a concise report and a summary table showing initial → pre-audit → final versions, migration notes, and research links. After validation, enable the eight workflows through the GitHub API without dispatching runs, then verify their remote state.
8. Commit research only in octolaba/changelog on pilot/implementation. Do not commit or push in indexit; confirm that HEAD is unchanged and show the diff. Enabling workflows on GitHub does not deliver local edits: remote code remains unchanged until the user commits the changes separately.

## Acceptance criteria

- Every discovered external action and every intermediate stable release has evidence and a sequential changelog, including dependencies of composite actions.
- Every researched repository is a submodule with an exact gitlink; verification records file integrity and coverage.
- No floating major-only action references remain; inputs and consumer adaptations are checked against the latest manifests.
- All YAML workflows are valid, missing reusable targets are resolved, and remote workflows are active.
- The summary distinguishes local verification, upstream claims, and behavior that requires a real GitHub runner.
- Indexit HEAD remains `ba90772f124c6d666882c13494e0528d9bb9b129`; research commits are allowed only in changelog.

## Scope clarification

The user initially asked to focus on YAML, Actions, and their changelogs. In subsequent Plannotator feedback, the user clarified that the restriction concerned Go tests and explicitly requested restoring and validating the docs migration. Acceptance therefore covers YAML, configuration schemas, action contracts, release evidence, and the documentation build. Go application tests and release builds are not repeated.
