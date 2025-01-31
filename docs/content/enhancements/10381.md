# Summary

This proposal aims to adopt Kubernetes' approach to changelog management by embedding changelog information directly within PR descriptions. Contributors will specify user-facing release notes, the type of change (e.g., /kind fix), and optionally link to related GitHub issues. Automation will be introduced to parse PR descriptions and aggregate this information into release notes during the release process. Additionally, the `/kind ...` command in PR descriptions will trigger automation to add appropriate labels to the PR for proper categorization bucketing.

The proposal also aims to address the current coupling of CI configurations with changelog entries by decoupling these concerns. While decoupling CI configurations is outside the direct scope of this proposal, a strawman plan will be outlined to ensure a smooth transition.

This new approach introduces trade-offs, such as losing git blame history for individual changelog files. However, these are offset by significant improvements to developer experience and long-term maintainability. By streamlining workflows and reducing reliance on external documentation or directory structures, this approach will improve contributor experience and simplify maintenance.

## Motivation

As we prepare to donate the Gloo project to the CNCF, we have an opportunity to simplify and align with best practices seen in other CNCF projects and projects within the Kubernetes ecosystem. The current changelog process requires developers to manually create and place changelog files in release-specific directories, e,g, `changelog/v1.18.3/<changelog-file>.yaml`. This approach imposes unnecessary complexity on contributors, particularly those that are new to the project.

Over time, the `changelog/` directory has grown to approximately 200 subdirectories in some solo-io repositories. This sprawl makes it difficult to navigate and determine the correct location for new entries. While a Makefile target (`make generate-changelog`) was introduced to help alleviate some of these issues, it hasn't been widely adopted across the solo-io GH organization. As a result, changelog management lacks consistency across repositories.

Additionally, some repositories allow contributors to annotate changelogs with special markers (e.g., `skipCI: true`). While this mechanism provides flexibility, it also couples changelog entries with CI configuration, and results in inconsistencies in which markers are supported across the fleet of product repositories. This coupling is not a sustainable long-term solution and we should aim to decouple these concerns as we transition to a new changelog process.

## Goals

- Improve the developer experience by embedding the changelog process into the PR template.
- Ensure the new process is documented in the CONTRIBUTING.md file so new contributors can easily understand the requirements.
- Eliminate the need to manually update changelog file locations when a new release is cut.
- Enable automation to parse and aggregate changelog entries from PRs into release notes.
- Decouple changelog files from CI configurations to ensure consistency across repositories.
- Allow release managers or maintainers to edit PR descriptions post-merge if necessary.
- Simplify long-term maintenance by potentially removing the `changelog/` directory.

## Non-Goals

- Enforce the new process across all upstream repositories or downstream solo-io GH repositories immediately. This transition can be made gradually, with new repositories adopting it first.
- The proposal will not change the content or structure of the actual changelog entries used in the release notes; it only modifies how entries are provided, tracked, and aggregated.
- This proposal does not address CI/CD or release automation processes unrelated to changelog management. Commentary on a strawman proposal for CI/CD automation is included in the proposal, but it is not the primary focus.

## Proposal

Adopt the approach taken by the [Kubernetes](https://github.com/kubernetes/kubernetes/blob/master/.github/PULL_REQUEST_TEMPLATE.md) ecosystem. This requires an overhaul of the current changelog process, including the following steps:

- Update the PR template to include a new section for the user-facing changelog description.
- Document new requirements in the CONTRIBUTING.md file to guide contributors on how to fill out the changelog section.
- Add automation to react to `/kind <fix, new_feature, breaking_change, etc.>` command within the PR template to categorize the type of change.
- Add a separate field for linking the GitHub issue associated with the change.
- Implement automation (e.g., GitHub Actions) to parse the PR template and aggregate changelog information into a single release changelog file when a new release is cut.
- Introduce validation to ensure the required changelog fields are populated before merging the PR.
- Decouple CI-specific fields (e.g., `skipCI: true`) from changelogs to maintain separation of concerns and improve consistency across repositories.

### PR Template Update

Introduce a new section in the PR template to allow users to configure the user-facing changelog description and the type of change:

```markdown
# Description

...

## Checklist

...

## Which issue(s) this PR fixes:
<!--
*Automatically closes linked issue when PR is merged.
Usage: `Fixes #<issue number>`, or `Fixes (paste link of issue)`.
_If PR is about `failing-tests or flakes`, please post the related issues/tests in a comment and do not use `Fixes`_*
-->
Fixes #1234

## What type of PR is this?
<!--
Label the type of change. Supported kinds include:
/kind fix
/kind new_feature
/kind breaking_change
/kind helm
/kind dependency_bump
/kind deprecation
-->

/kind fix

## Does this PR introduce a user-facing change?
<!--
If no, just write "NONE" in the release-note block below.
If yes, a release note is required:
Enter your extended release note in the block below.
-->
```

TODO: We need automation that adds a comment when an invalid command is used in the PR description. This will help guide contributors on how to properly format the PR description. Without it, we risk having PRs merged without the necessary changelog information or requiring maintainers to manually enforce the rules.

For example, the current process using a real changelog file:

```yaml
changelog:
  - type: FIX
    issueLink: https://github.com/solo-io/gloo-mesh-enterprise/issues/18468
    description: Fixes an admission-time validation bug that prevented the LoadBalancerPolicy's `spec.config.consistentHash.httpCookie.ttl` field from being set to a zero value such as "0s".
    resolvesIssue: false
```

And that would be converted to the following format:

```markdown
## Which issue(s) this PR fixes:
<!--
*Automatically closes linked issue when PR is merged.
Usage: `Fixes #<issue number>`, or `Fixes (paste link of issue)`.
_If PR is about `failing-tests or flakes`, please post the related issues/tests in a comment and do not use `Fixes`_*
-->
Related to #18468. Requires backport to 1.18.x.

## What type of PR is this?
<!--
Label the type of change. Supported kinds include:
/kind fix
/kind new_feature
/kind breaking_change
/kind helm
/kind dependency_bump
/kind deprecation
-->

/kind fix

## Does this PR introduce a user-facing change?
<!--
If no, just write "NONE" in the release-note block below.
If yes, a release note is required:
Enter your extended release note in the block below.
-->
Fixes an admission-time validation bug that prevented the LoadBalancerPolicy's `spec.config.consistentHash.httpCookie.ttl` field from being set to a zero value such as "0s".
```

### Release Notes Automation

This section outlines how automation will generate release notes based on the changelog section in PR descriptions. New automation will be introduced to the release GHA workflow.

Requirements:

- PR template includes a changelog section with the required fields.
- Extend the GHA release workflow to handle release notes. Now, it would be responsible for each PR merged since the last release and aggregate changelog information into github release notes. Needs to be categorized by type of change (e.g., fix, new_feature, breaking_change).
- Update the release checklist to audit the release notes and ensure they are accurate and complete?

Open questions:

- Do we need to support on-demand workflow (e.g. `workflow_dispatch`) and/or a way to manually run the release notes generation workflow when needed?
- What does krel do for k/k? What does it do when a PR for a release branch is edited post-merge?

<!-- TODO: daneyeon mentioned <https://docs.github.com/en/repositories/releasing-projects-on-github/automatically-generated-release-notes> is a possibility. -->
<!-- TODO: nina mentioned <https://github.com/kubernetes/release/blob/master/docs/krel/README.md> for the tooling that k/k relies on to handle changelogs. -->

### Overhauling Special CI Markers in Changelog Entries

<!-- TODO: Move this to it's own proposal and link back here? -->

Currently, some repositories include custom fields in their changelog files to control CI behavior. For example, the `skipCI: true` field is used in the GME repository to prevent CI from running for a specific PR. This coupling is not sustainable in the long term and should be decoupled from the changelog process.

That said, exposing knobs that control CI behavior may have some value in certain scenarios. Take the following changelog entry as an example:

```yaml
changelog:
  - type: NON_USER_FACING
    description: >-
      Update README.md to include new installation instructions.

      skipCI-kube-tests:true
      skipCI-in-memory-e2e-tests:true
      skipCI-storybook-tests:true
```

In this case, modifying a markdown file or other non-user-facing content that does not require CI to run should be able to skip CI checks. This allows us to control costs and provide better time-to-merge characteristics for trivial changes. Further sub-sections will explore potential alternatives to address this concern. Alternatively, we could consider removing support for this behavior altogether.

#### Alternative 1: Remove Manual CI Overrides

Remove the ability for developers to modify the CI pipeline directly using slash commands, e.g. `/kick-ci` or providing special markers in changelog entries. CI behavior would then become fixed and determined solely by the code and changes being committed, without any manual intervention or overrides.

**Pros:**

- Simplifies CI/CD pipeline implementation by removing any ad-hoc developer inputs.
- Encourages investment in optimizing the pipeline itself to reduce runtime instead of exposing knobs to skip steps.
- Ensures consistency and reliability by running the same pipeline for all changes.
- Prevents potential misuse or accidental skipping of critical test suites.

**Cons:**

- Removes flexibility for developers who may want to re-run or skip certain tests in specific scenarios.
- Could increase CI costs and run times for trivial changes (e.g., README updates).
- May frustrate developers if pipelines are slow or include unnecessary steps for certain changes.
- Investments in reducing CI runtime is not always trivial and may require significant effort or have process implications.

Overall, this alternative is the most straightforward and ensures a consistent CI/CD pipeline for all changes. However, it may not be suitable for all projects or scenarios, especially those with complex or lengthy pipelines. We can always revisit this behavior over time if it becomes a significant issue, or CI pipelines become the main dev bottleneck (vs. code review).

#### Alternative 2: Migrate special marker annotations (to GHAs?)

Transition special markers like `skipCI-kube-tests:true` from changelog annotations into GHA configurations or workflows. In this model, contributors could add labels to PRs (e.g., `ci-skip-e2e-tests`) to modify pipeline behavior.

<!-- TODO: Just discovered <https://docs.github.com/en/actions/managing-workflow-runs-and-deployments/managing-workflow-runs/skipping-workflow-runs>. That approach basically models our changelog approach embedding special markers in the commit message? -->

**Pros:**

- Improves separation of concerns by moving CI configuration out of changelog entries.
- Leverages GitHub's native tagging and workflow capabilities, making metadata more centralized and accessible.
- Enables automated enforcement of rules, such as requiring specific tags for certain types of changes.

**Cons:**

- Increases reliance on GitHub-specific features, potentially reducing portability.
- Contributors may not have the necessary permissions to add labels or tags to PRs and maintainers may need to intervene.
- Still exposes pipeline modification to developers, which could lead to misuse or inconsistent application.
- Requires investments in further automation to instrument and enforce CI behavior based on tags. We don't want to accidentally skip critical tests and regress main branch stability.

Additionally, we need to clearly document and restrict the scenarios where skipping CI steps is appropriate in the root CONTRIBUTING.md file to guide contributors on how to tag PRs correctly.

#### Alternative 3: Automate CI Behavior based on PR Changes

Automatically adjust the CI pipeline based on the files modified in a pull request. For example, a README update might only trigger linting and formatting checks, while code changes trigger full test suites.

<!-- TODO: IIRC, there's an issue with this model or some edge case. Ex: skipping a required GHA job based on path or branch filtering has some weird behavior, which means you need to have conditions that determine whether the job needs to be run and/or set the GH context yourself? -->

**Pros:**

- Removes the need for developers to manually modify CI behavior.
- Fully automates pipeline adjustments, reducing cognitive overhead for contributors.
- Ensures consistency by using predefined rules for pipeline adjustments.

**Cons:**

- Likely requires sophisticated change detection logic, which introduces complexity in our CI pipeline and have long-term maintenance implications.
- May not account for edge cases where trivial changes have downstream impacts.
- Developers lose control over CI behavior, which might be frustrating in certain scenarios.

This alternative is the most hands-off approach for developers, as they don't need to worry about CI configurations at all. However, it requires significant investment in automation and change detection logic to ensure the pipeline is adjusted correctly for all changes.

## Open Questions

- **LTS Release Branches**: How should we handle LTS release branches that follow the old changelog process while newer branches adopt the new approach? Is there a graceful way to transition between the two?
- **Removal of `changelog/` Directory**: Can we remove the `changelog/` directory altogether moving forward? What impact will this have on historical changelog information? Does the long-term maintenance benefit outweigh the potential loss of access to older changelog files?
- **Developer Experience Regression**: With this new process, there may be a slight regression in developer experience. Previously, developers could use `git log` and `git show <commit-hash> -- changelog` to view commit history. Now, they would need to navigate to GitHub to see the full changelog in the pull request. The vscode lens extension could help with this, but it's not as convenient as `git log`. Is this trade-off acceptable, considering the PR number is embedded in the commit title and we have already adopted squash/merge commit settings?
- **CI/CD Integration and Backwards Compatibility**: The current process includes custom fields like `skipCI: true` in the changelog YAML files, which configure CI pipeline behavior. How do we ensure backwards compatibility when decoupling these fields from the changelog, while maintaining the same functionality? How can we continue to control CI behavior without embedding configurations in the changelog itself?
- **Changelog Validation**: Currently, validation is automated. Moving changelog descriptions to PRs could mean relying on PR authors and reviewers to enforce standards manually. Should this be an intermediate step, or can we automate validation post-merge? Should PR changelog descriptions be editable after approval, and if so, how do we maintain proper audit trails?
- Do we need to support NON_USER_FACING anymore? I think in most cases, release notes NONE is sufficient.
- Are release managers responsible for auditing PR descriptions going forward, or is that a responsibility for PR reviewers?
- Do we need a static changelog/CHANGELOG-1.18.x.md file that stores all the changelog entries for a release? This would be consistent with how k/k does it, but it's not clear if it's necessary for us.

## Answered Questions

- Q: Should we use slash commands (e.g., `/kind fix`) or labels (e.g., `/label type:fix`) to classify the type of change? Both options imply additional automation to ensure the correct label is applied to the PR. Which approach is more effective and easier to automate? A: I think we'd want to use slash commands and have automation apply a label. This is because contributors may not have permissions to apply labels, and it would be easy for automation to search for all issues that have a special label when generating release notes.

## Alternatives

- Adopt [envoy's approach](https://github.com/envoyproxy/envoy/blob/main/changelogs/current.yaml), or maintain a CHANGELOG.md file in the repository root that aggregates all changelog entries for a release. This approach has it's own challenges with managing the file (e.g. merge conflicts) and ensuring it's up-to-date.
- Adopt [controller-runtime'](https://github.com/kubernetes-sigs/controller-runtime/tree/main/.github/PULL_REQUEST_TEMPLATE)s approach that uses emojis within a PR title to help classify the change.
- Continue with the current process and any address dexex issues or inconsistencies as they arise.
