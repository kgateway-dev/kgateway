---
name: New Kgateway Release
about: Propose a new release
title: Release v0.x.0
labels: ''
assignees: ''

---

## Introduction

The assigned release manager should use this checklist to validate a release candidate (RC) is ready to become the
final stable release.

---

## Release metadata

- Release version: `vX.Y.Z`
- RC tag: `vX.Y.Z-beta.N`
- Release branch: `vX.Y.x`
- GitHub milestone: `<LINK_TO_RELEASE_MILESTONE>`

---

## Release Preparation

- [ ] Release lead and backup assigned.
- [ ] Milestone is created with expected release date.
- [ ] Scope confirmed (milestone approved by Product and Release Managers).
- [ ] Release branch created.
- [ ] All release-blocker issues are identified, assigned, and tracked against the milestone.
- [ ] All non-blockers are moved forward to the next milestone (or explicitly deferred with rationale).

---

## Code Health and CI Gates

- [ ] All required CI checks are green on the RC tag (not just on `main`).
- [ ] Dependency updates merged for the RC where required (Gateway API, Envoy/agentgateway, Istio, etc.).
- [ ] Version strings are correct across:
  - [ ] Helm charts
  - [ ] Container image tags

---

## Test matrix

### Core functional validation

- [ ] Fresh install works via the documented “Get started” path for both data planes you support.

### Platform Matrix

- [ ] Kubernetes versions in the support matrix validated.
- [ ] Gateway API versions in the support matrix validated (standard and experimental).

---

## Conformance Reporting

- [ ] Gateway API conformance has been published for latest release candidate.

---

## Documentation Readiness

- [ ] Kgateway [docs](https://github.com/kgateway-dev/kgateway.dev) are updated to support latest release candidate.

---

## Migration Readiness

- [ ] Migration tool (https://github.com/kgateway-dev/ingress2gateway) is updated to support latest release candidate.
- [ ] Docs “Migrate from Ingress” references the correct ingress2gateway version and usage.

---

## Release Artifacts

### GitHub release hygiene

- [ ] Git tag `vX.Y.Z` exists and points at the intended commit.
- [ ] GitHub Release is created from the tag and includes:
  - [ ] Human-readable release notes
  - [ ] Links to full changelog

### Container Images

- [ ] Images are published for the final tag.

### Helm

- [ ] Helm charts are [published](https://github.com/orgs/kgateway-dev/packages?repo_name=kgateway).
- [ ] Example manifests (quickstarts) install cleanly against the final version.

---

## Project Management

- [ ] Close out release milestone (https://github.com/kgateway-dev/kgateway/milestones), e.g. move forward any non release blockers, etc.
- [ ] All PRs included in the release are labeled correctly.
- [ ] Backport PRs are opened/merged for any critical fixes to supported stable branches.
- [ ] Next milestone is created and seeded with carry-overs.

---

## Release Communications

- [ ] Release announcement drafted.
- [ ] Announcement published in the kgateway community channel.
- [ ] Blog post published for major/minor releases.
