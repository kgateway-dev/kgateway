<!--
**Note:** When your Enhancement Proposal (EP) is complete, all of these comment blocks should be removed.

This template is inspired by the Kubernetes Enhancement Proposal (KEP) template: https://github.com/kubernetes/enhancements/blob/master/keps/sig-architecture/0000-kep-process/README.md

To get started with this template:

- [ ] **Create an issue in kgateway-dev/kgateway**
- [ ] **Make a copy of this template.**
  `EP-[ID]: [Feature/Enhancement Name]`, where `ID` is the issue number (with no
  leading-zero padding) assigned to your enhancement above.
- [ ] **Fill out this file as best you can.**
  At minimum, you should fill in the "Summary" and "Motivation" sections.
- [ ] **Create a PR for this EP.**
  Assign it to maintainers with relevant context.
- [ ] **Merge early and iterate.**
  Avoid getting hung up on specific details and instead aim to get the goals of
  the EP clarified and merged quickly. The best way to do this is to just
  start with the high-level sections and fill out details incrementally in
  subsequent PRs.

Just because a EP is merged does not mean it is complete or approved. Any EP
marked as `provisional` is a working document and subject to change. You can
denote sections that are under active debate as follows:

```
<<[UNRESOLVED optional short context or usernames ]>>
Stuff that is being argued.
<<[/UNRESOLVED]>>
```

When editing EPS, aim for tightly-scoped, single-topic PRs to keep discussions
focused. If you disagree with what is already in a document, open a new PR
with suggested changes.

One EP corresponds to one "feature" or "enhancement" for its whole lifecycle. Once a feature has become
"implemented", major changes should get new EPs.
-->
# EP-[ID]: [Feature/Enhancement Name]

<!--
This is the title of your EP. Keep it short, simple, and descriptive. A good
title can help communicate what the EP is and should be considered as part of
any review.
-->

* Issue: [#ID](URL to GitHub issue)

<!--
A table of contents is helpful for quickly jumping to sections of a EP and for
highlighting any additional information provided beyond the standard EP
template.

Ensure the TOC is wrapped with
  <code>&lt;!-- toc --&rt;&lt;!-- /toc --&rt;</code>
tags, and then generate with `hack/update-toc.sh`.
-->

## Background

<!--
Provide a brief overview of the feature/enhancement, including relevant background information, origin, and sponsors.
Highlight the primary purpose and how it fits within the broader ecosystem.

Include Motivation, concise overview of goals, challenges, and trade-offs.

-->

## Motivation

<!--
This section is for explicitly listing the motivation, goals, and non-goals of
this EP. Describe why the change is important and the benefits to users. The
motivation section can optionally provide links to [experience reports] to
demonstrate the interest in a EP within the wider Kubernetes community.

[experience reports]: https://github.com/golang/go/wiki/ExperienceReports
-->

## Goals

<!--

List the specific goals of the EP. What is it trying to achieve? How will we
know that this has succeeded?

Include specific, actionable outcomes. Ensure that the goals focus on the scope of
the proposed feature.
-->


## Non-Goals

<!--
What is out of scope for this EP? Listing non-goals helps to focus discussion
and make progress.
-->

## Implementation Details

<!--
This section should contain enough information that the specifics of your
change are understandable. This may include API specs (though not always
required) or even code snippets. If there's any ambiguity about HOW your
proposal will be implemented, this is the place to discuss them.


### Configuration
Specify changes to any configuration APIs, CRDs, or user-facing options to enable/disable
the feature. Include references to relevant files or configurations that need updates.

### Plugin
Describe how the feature will be added as a plugin (if applicable).
Include references to existing plugin frameworks or structures.
Highlight the plugin's responsibilities and integration points.

### Controllers
Outline the responsibilities of new or updated controllers.
Specify conditions for their operation and integration with existing resources.
Mention required RBAC updates or new permissions.

### Deployer
Detail deployment-specific updates, e.g., Helm chart modifications, custom deployer changes, etc.
Include any prerequisites or dependencies for deployment.

### Translator and Proxy Syncer
Specify updates to translators, syncers, or other intermediary components.
Highlight how these components interact with the feature's resources or backend.

### Reporting
Describe changes to status reporting or monitoring frameworks.
Include any caveats or limitations in initial reporting.
-->

### Test Plan

<!--
    Define the testing strategy for the feature.
    Include unit, integration, and end-to-end (e2e) tests.
    Specify any additional frameworks or tools required for testing.
-->

## Alternatives

<!--
Highlight potential challenges or trade-offs.
-->

## Open Questions

<!--
Include any unresolved questions or areas requiring feedback.
-->
