# kgateway Security Incident Response Process

This document outlines the process the kgateway team follows when handling security vulnerabilities reported in our open-source applications and libraries (CLI, etc.). Our goal is to be transparent with our community about how we triage, fix, and disclose security issues.

This process applies only to the open-source tools maintained by the kgateway project.

---

### Phase 1: Triage & Validation

This initial phase begins when we receive a security vulnerability report. The primary goal is to understand, reproduce, and assess the potential impact of the reported issue.

**Process Overview:**

When a report is received, a designated maintainer who is part of the security group will:

1. **Acknowledge the Report**: Privately acknowledge receipt of the report from the person who submitted it.
2. **Validate the Vulnerability**: Thoroughly review the report and work to reproduce the vulnerability. This helps us understand the conditions required for an exploit.
3. **Assess Impact**: Determine the potential impact on the confidentiality, integrity, and availability (CIA) of a user's environment. We analyze what data could be at risk and what an attacker might be able to achieve.
4. **Determine Severity**: Assign an initial severity level (e.g., Critical, High, Medium, Low) based on the impact and exploitability.
5. **Decision**:

- If the report is not a vulnerability or is otherwise invalid, we will inform the reporter of our reasoning.
- If the vulnerability is validated, we will proceed to the Mitigation & Remediation phase.

---

### Phase 2: Mitigation & Remediation

Once a vulnerability is confirmed, our focus shifts to fixing the issue and preventing potential exploitation.

**Process Overview:**

Our remediation efforts include the following steps:

1. **Discuss and Plan the Fix**: The team will identify the root cause and discuss potential approaches to resolve the vulnerability. The proposed solution must be agreed upon by the security group before implementation begins.
2. **Develop the Fix**: Once the approach is agreed upon, develop a code patch to resolve the vulnerability.
3. **Code Review**: The proposed fix will be submitted as a pull request and undergo a thorough review by maintainers to ensure it is effective and does not introduce new issues.
4. **Testing**: The patch is tested to confirm that it successfully resolves the vulnerability.
5. **Merge and Release**: Once approved, the fix is merged into the main codebase and scheduled for inclusion in the next official release of the application. The fix will be backported to all versions in the support window.

---

### Phase 3: Scoping & Impact Analysis

During this phase, we analyze the codebase to understand the full scope of the vulnerability.

**Process Overview:**

The team will perform an analysis to determine:

1. **Affected Versions**: Identify all previous versions of the kgateway open-source application that contain the vulnerability.
2. **Introduction Point**: Pinpoint when the vulnerability was first introduced into the codebase.
3. **User Impact**: Define the potential impact on users running affected versions. This includes clarifying what an attacker could do and what conditions must be met to exploit the vulnerability.

---

### Phase 4: Notification & Disclosure

Transparency with our users is critical. This phase is about communicating the vulnerability and its solution to the community in a clear and timely manner.

**Process Overview:**

Following the release of a patched version, we will execute our public disclosure process.

1. **Publish Security Advisory**: We will publish a release and any corresponding documentation for mitigating the vulnerability. This will include:

   - A description of the vulnerability and its potential impact.
   - A list of all affected application versions.
   - The version number that contains the fix.
   - Instructions and recommendations for users to upgrade.
   - Credit to the security researcher who discovered and reported the issue (with their permission).

2. **Community Communication**: We will announce the security patch through our public channels, including the application's release notes, directing users to the GitHub Security Advisory for details.
