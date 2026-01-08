# EP-10381: Adopt Kubernetes Approach for Changelog Management

* Issue: [#10381](https://github.com/kgateway-dev/kgateway/issues/10381)

## Background

This proposal adopts Kubernetes' approach to changelog management by embedding changelog information directly within pull request descriptions. Historically, changelog entries have been maintained as separate files organized by release-specific directories, a practice that has resulted in complex directory structures and navigational challenges. In contrast, the new method requires contributors to include user-facing release notes, indicate the type of change (for example, using the `/kind fix` command), and optionally reference related GitHub issues directly in their pull requests.

This change not only streamlines the process but also enables automation to parse pull request descriptions and aggregate changelog information into comprehensive release notes during the release process. In addition, automation will trigger the assignment of appropriate labels based on the `/kind` command, thereby simplifying categorization and future management.

Although this new approach introduces certain trade-offs—such as the loss of git blame history for individual changelog files—these are balanced by significant improvements in developer experience and long-term maintainability.

## Motivation

As we prepare to donate the Gloo project to the CNCF, there is an opportunity to simplify our changelog process and align with best practices seen in other CNCF projects and within the Kubernetes ecosystem. The current process, which requires developers to manually create and place changelog files in release-specific directories (for example, `changelog/v1.18.3/<changelog-file>.yaml`), imposes unnecessary complexity and presents a steep learning curve for new contributors.

Over time, the `changelog/` directory has expanded to include approximately 200 subdirectories in some repositories in the previous GH organization, making it increasingly difficult to navigate and identify the correct location for new entries. Although a Makefile target (`make generate-changelog`) was introduced to mitigate some of these issues, its limited adoption has led to inconsistent changelog management across repositories.

Furthermore, the existing practice of embedding special markers—such as `skipCI: true`—within changelog files couples changelog data with CI configurations, leading to inconsistencies and a less sustainable long-term solution. The motivation behind this proposal is to eliminate these complexities by decoupling the changelog process from CI configurations and integrating changelog data directly into pull requests, thereby enhancing the contributor experience and ensuring more consistent maintenance.

### Goals

- Improve the developer experience by embedding the changelog process into the PR template.
- Ensure the new process is documented in the CONTRIBUTING.md file so new contributors can easily understand the requirements.
- Eliminate the need to manually update changelog file locations when a new release is cut.
- Enable automation to parse and aggregate changelog entries from PRs into release notes.
- Decouple changelog files from CI configurations to ensure consistency across repositories.
- Allow release managers or maintainers to edit PR descriptions post-merge if necessary.
- Simplify long-term maintenance by removing the `changelog/` directory.

### Non-Goals

- Enforce the new process across all upstream repositories or downstream solo-io GH repositories immediately. This transition can be made gradually, with new repositories adopting it first.
- The proposal will not change the content or structure of the actual changelog entries used in the release notes; it only modifies how entries are provided, tracked, and aggregated.
- This proposal does not address CI/CD or release automation processes unrelated to changelog management. Commentary on a strawman proposal for CI/CD automation is included in the proposal, but it is not the primary focus.
- Validate the PR template and PR description after the PR is merged.

## Implementation Details

Adopt the approach taken by the [Kubernetes](https://github.com/kubernetes/kubernetes/blob/master/.github/PULL_REQUEST_TEMPLATE.md) ecosystem. This requires an overhaul of the current changelog process, including the following steps:

- Update the PR template to include a new section for the user-facing changelog description.
- Document new requirements in the CONTRIBUTING.md file to guide contributors on how to fill out the changelog section.
- Add automation to react to `/kind <fix, new_feature, breaking_change, etc.>` command within the PR template to categorize the type of change.
- Add a separate field for linking the GitHub issue associated with the change.
- Implement automation (e.g., GitHub Actions) to parse the PR template and aggregate changelog information into a single release changelog file when a new release is cut.
- Introduce validation to ensure the required changelog fields are populated before merging the PR.
- Decouple CI-specific fields (e.g., `skipCI: true`) from changelogs to maintain separation of concerns and improve consistency across repositories.

In some edge cases, providing multiple kind labels to a PR may be appropriate. For example, a PR may contain changes to the helm chart, the API, and the docs. This is supported by the new PR template, but multiple release notes will not be supported. Instead, the release note will be the same for each kind.

### PR Template Update

Introduce a new section in the PR template to allow users to configure the user-facing changelog description and the type of change:

```markdown
<!--
Thanks for opening a PR! Please delete any sections that don't apply.
-->

# Description

<!--
A concise explanation of the change. You may include:
- **Motivation:** why this change is needed
- **What changed:** key implementation details
- **Related issues:** e.g., `Fixes #123`
-->

# Change Type

<!--
Select one or more of the following by including the corresponding slash-command:
```
/kind breaking_change
/kind bug_fix
/kind design
/kind cleanup
/kind deprecation
/kind documentation
/kind flake
/kind new_feature
```
-->

# Changelog

<!--
Provide the exact line to appear in release notes for the chosen changelog type.

If no, just write "NONE" in the release-note block below.
If yes, a release note is required:
-->

```release-note

```

# Additional Notes

<!--
Any extra context or edge cases for reviewers.
-->
```

### Automation - PR Description

When a PR's description is updated -- either during the review process or after the PR is merged -- the PR description will be scanned for a changelog section.

Implementing this as a custom GHA action is the most flexible and allows us to re-use this logic across multiple repositories:

> Note: This will require maintainers to have write access to the repository to add the new labels before automation has permissions to do so.

1. Parse the PR description for required sections
2. Validate the changelog format and content
3. Apply appropriate labels based on the validation results
4. Provide feedback to PR authors if any issues are found

Having this automation own the responsibility of validating the PR description and applying labels based on the validation results allows us to simplify the PR template and reduce the cognitive load for PR authors.

### Automation - Release Changelog Generation

Additional automation will be introduced to list any _merged_ PRs with a `release-notes-needed` label since the last release and aggregate the changelog information into a release changelog file when a new release is cut. The new https://github.com/kgateway-dev/kgateway/blob/main/.github/workflows/release.yaml builds on top of [goreleaser](https://goreleaser.com/) to delegate the responsibility of building and publishing build-related artifacts within the overall release process.

Goreleaser is flexible enough to support custom changelog formats, which will allow us to pipe the custom changelog information into the release notes that it manages. This will be done via providing the `GORELEASER_ARGS="--clean --release-notes CHANGELOG.md"` argument to the release workflow for workflow_dispatch or new tag events. See the [goreleaser changelog documentation](https://goreleaser.com/customization/changelog/) for more details.

There's a couple of options for how this automation will be implemented:

- A composite action that allows another workflow to call this action periodically
- Extend the release GHA workflow and add a new job to it
- Develop a custom GHA action to handle the changelog aggregation

The latter option is the most flexible and allows us to re-use this logic across multiple repositories.

### Example - Retrofitting a Changelog File

Let's step through an example of retrofitting an example changelog file to the new PR template.

```yaml
changelog:
  - type: FIX
    issueLink: https://github.com/solo-io/gloo-mesh-enterprise/issues/18468
    description: Fixes an admission-time validation bug that prevented the LoadBalancerPolicy's `spec.config.consistentHash.httpCookie.ttl` field from being set to a zero value such as "0s".
    resolvesIssue: false
```

This would be converted to the following format in the PR description:

<details>

```markdown
# Description

Fixes an admission-time validation bug that prevented the LoadBalancerPolicy's `spec.config.consistentHash.httpCookie.ttl` field from being set to a zero value such as "0s".

Related to #18468. Requires backport to 1.18.x.

# Change Type

/kind fix

# Changelog

```release-note
Fixes an admission-time validation bug that prevented the LoadBalancerPolicy's `spec.config.consistentHash.httpCookie.ttl` field from being set to a zero value such as "0s".
```

# Additional Notes

This change requires backporting to the 1.18.x release branch.
```

</details>

### Side Quest: Overhauling Special CI Markers in Changelog Entries

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

### Test Plan

TBD.

## Open Questions

- Do we want to check-in static changelogs or do this magically/implicitly in automation?

## Answered Questions

- Q: Should we use slash commands (e.g., `/kind fix`) or labels (e.g., `/label type:fix`) to classify the type of change? Both options imply additional automation to ensure the correct label is applied to the PR. Which approach is more effective and easier to automate? A: I think we'd want to use slash commands and have automation apply a label. This is because contributors may not have permissions to apply labels, and it would be easy for automation to search for all issues that have a special label when generating release notes.
- Q: How should we handle LTS release branches that follow the old changelog process while newer branches adopt the new approach? Is there a graceful way to transition between the two? No LTS release branches are currently supported.
- Q: Can we remove the `changelog/` directory altogether moving forward? What impact will this have on historical changelog information? Does the long-term maintenance benefit outweigh the potential loss of access to older changelog files? A: Yes, we can remove the `changelog/` directory altogether moving forward. The solo-io/gloo fork can serve as an archive for historical changelog information.
- A: Do we need to support NON_USER_FACING anymore? I think in most cases, release notes NONE is sufficient. Nope, `NONE` is sufficient for our use cases.

## Alternatives

- Adopt [envoy's approach](https://github.com/envoyproxy/envoy/blob/main/changelogs/current.yaml), or maintain a CHANGELOG.md file in the repository root that aggregates all changelog entries for a release. This approach has it's own challenges with managing the file (e.g. merge conflicts) and ensuring it's up-to-date.
- Adopt [controller-runtime'](https://github.com/kubernetes-sigs/controller-runtime/tree/main/.github/PULL_REQUEST_TEMPLATE)s approach that uses emojis within a PR title to help classify the change.
- Continue with the current process and any address dexex issues or inconsistencies as they arise.
