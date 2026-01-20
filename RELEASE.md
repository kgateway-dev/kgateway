# Kgateway releases

## Rolling `main` builds

Automation is in place to build and publish releases for all commits merged into the `main` branch.

This enables devs and users to have concrete artifacts for testing which contain features and bug fixes which have not yet made it into a patch or minor release.

The version is rolling, based on the next minor version release, e.g. `v2.2.0-main`.

The usable artifacts are pushed to GHCR and visible on the [packages page](https://github.com/orgs/kgateway-dev/packages?repo_name=kgateway).

Typically this will be consumed via the helm charts, and can be used directly, such as:
```bash
helm install kgateway-crds oci://cr.kgateway.dev/kgateway-dev/charts/kgateway-crds --version v2.2.0-main --namespace kgateway-system --create-namespace
helm install kgateway oci://cr.kgateway.dev/kgateway-dev/charts/kgateway --version v2.2.0-main --namespace kgateway-system --create-namespace
```

## Developer documentation

Please refer to [devel/contributing/releasing.md](devel/contributing/releasing.md).

## Release Checklist

The assigned release manager should use this checklist to validate a release candidate (RC) is ready to become the
final stable release.

> Tip: Copy this file into the release tracking issue/PR and fill in links as you go.

---

## Release metadata

- Release version: `vX.Y.Z`
- RC tag: `vX.Y.Z-beta.N`
- Release branch: `vX.Y.x`
- GitHub milestone: https://github.com/kgateway-dev/kgateway/milestones
- Release tracking issue/PR: `<link>`

---

## 0) Release preparation

- [ ] Release lead and backup assigned.
- [ ] Milestone is created with expected release date.
- [ ] Release checklist issue created and assigned to the release milestone.
- [ ] Scope confirmed: only release-blockers allowed after freeze (document exceptions in the tracking issue).
- [ ] Release branch created.
- [ ] All release-blocker issues are identified, assigned, and tracked against the milestone.
- [ ] All non-blockers are moved forward to the next milestone (or explicitly deferred with rationale).

---

## 1) Code health and CI gates

- [ ] All required CI checks are green on the RC tag (not just on `main`).
- [ ] No flaky tests open as release blockers (or flake is understood  quarantined with an issue linked).
- [ ] All “must-fix” regressions since the previous stable release are addressed (list in tracking issue).
- [ ] Dependency updates merged for the RC where required (Gateway API, Envoy/agentgateway, Istio, etc.).
- [ ] Version strings are correct across:
  - [ ] Helm charts (appVersion  chart version)
  - [ ] Container image tags
  - [ ] CLI/binaries (if any)
  - [ ] Docs examples (install/upgrade snippets)

---

## 2) Test matrix

### Core functional validation

- [ ] Fresh install works via the documented “Get started” path for both data planes you support (Envoy / agentgateway as applicable).
- [ ] Upgrade works from the previous stable minor (X.(Y-1).*) to this release (X.Y.*), following the docs.
- [ ] Upgrade works from the latest patch in this minor’s RC line (if multiple RCs were cut).

### Platform matrix (minimum)

- [ ] Kubernetes versions in the support matrix validated .
- [ ] Gateway API versions in the support matrix validated (standard and experimental).
- [ ] Gateway API Inference Extension versions in the support matrix validated.

---

## 3) Conformance Reporting

- [ ] Gateway API conformance has been published for latest release candidate.
- [ ] Gateway API Inference Extension conformance has been published for latest release candidate.

---

## 4) Documentation readiness

- [ ] Kgateway [docs](https://github.com/kgateway-dev/kgateway.dev) are updated to support latest release candidate.
- [ ] Gateway API Inference Extension [docs](https://gateway-api-inference-extension.sigs.k8s.io/) are updated to support latest release candidate.
- [ ] llm-d [docs](https://llm-d.ai/) are updated to support latest release candidate.
- [ ] vLLM production-stack [docs](https://docs.vllm.ai/projects/production-stack/en/latest/) are updated to support latest release candidate.
- [ ] What other 3rd party docs should be updated?

---

## 5) Migration tool readiness (ingress2gateway)

- [ ] Migration tool (https://github.com/kgateway-dev/ingress2gateway) is updated to support latest release candidate.
- [ ] Docs “Migrate from Ingress” references the correct ingress2gateway version and usage.

---

## 6) Release artifacts

### GitHub release hygiene

- [ ] Git tag `vX.Y.Z` exists and points at the intended commit.
- [ ] GitHub Release is created from the tag and includes:
  - [ ] Human-readable release notes (curated; not just auto-generated)
  - [ ] Links to full changelog/compare view
  - [ ] Others?

### Container images

- [ ] Images are published for the final tag and are pullable by digest.
- [ ] Multi-arch images verified (where applicable).
- [ ] Image vulnerability scan completed; no Critical/High vulns without documented exceptions.
- [ ] Image provenance/signing completed (cosign/SLSA/etc. as your project standard).

### Helm

- [ ] Helm charts are published to the expected registry/repo and install cleanly:
- [ ] Example manifests (quickstarts) install cleanly against the final version.

---

## 7) Project management

- [ ] Close out release milestone (https://github.com/kgateway-dev/kgateway/milestones), e.g. move forward any non release blockers, etc.
- [ ] All PRs included in the release are labeled correctly.
- [ ] Backport PRs are opened/merged for any critical fixes to supported stable branches.
- [ ] Next milestone is created and seeded with carry-overs.

---

## 8) Release communications

- [ ] Release announcement drafted.
- [ ] Announcement published in the kgateway community channel.
- [ ] Blog post published for major/minor releases.

---

## 9) Post-release follow-ups

- [ ] Release retrospective scheduled:
  - [ ] What went well
  - [ ] What was painful
  - [ ] Action items filed under “Release Improvements”
