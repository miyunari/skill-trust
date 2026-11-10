# Project issue backlog

These issue drafts are ordered for implementation. They can be copied into
GitHub after review. Labels shown below are proposed labels.

## Epic 1: Trust model and policy

### 1. Publish the threat model and enforcement boundary

**Labels:** `design`, `documentation`, `security`

Define what a signature proves and what it does not prove.

**Acceptance criteria**

- Document content integrity, signer identity, provenance, transparency
  evidence, and runtime authorization as separate concepts.
- Document the fail-closed requirement for configured harness adapters.
- Document that a harness can bypass protection if it does not use the
  verifier.
- Identify static-analysis, prompt-injection, malware, runtime-isolation, and
  host-permission controls as separate concerns.

### 2. Define organization verification policy

**Labels:** `design`, `enhancement`, `security`

Define the policy inputs needed for an allow/block decision.

**Acceptance criteria**

- Policy can constrain signer identity, issuer, trust root, provenance, and
  transparency/timestamp requirements.
- Policy supports public Sigstore and RHTAS trust roots.
- One matching valid signature is sufficient by default.
- Missing or unverifiable evidence blocks by default.
- Policy evaluation is separate from manifest generation and cryptographic
  verification.

## Epic 2: Canonical skill identity

### 3. Implement the canonical skill manifest specification

**Labels:** `design`, `enhancement`

Define and version the manifest that represents an entire skill directory.

**Acceptance criteria**

- `SKILL.md` is required at the root.
- All regular files outside `.skill-trust/` are included.
- Paths are normalized and sorted deterministically.
- SHA-256 content digests and executable bits are represented.
- Timestamps, owners, absolute paths, and host-specific metadata are excluded.
- Symlinks and special filesystem entries are rejected in v1.
- Canonical serialization and manifest digest rules are documented with test
  vectors.

### 4. Define signed skill package layout

**Labels:** `design`, `documentation`

Define the embedded control directory and distribution rules.

**Acceptance criteria**

- `.skill-trust/manifest.json` and `.skill-trust/bundle.sigstore.json` are
  specified.
- `.skill-trust/` is excluded from the content manifest.
- The verifier rejects missing, malformed, duplicate, or mismatched metadata.
- Directory and portable archive distribution behavior is documented.
- OCI artifacts are explicitly out of scope for the skill package format.

## Epic 3: Sigstore and RHTAS integration

### 5. Build a Sigstore/RHTAS trust configuration layer

**Labels:** `enhancement`, `security`

Create one configuration model for public Sigstore and RHTAS endpoints.

**Acceptance criteria**

- Fulcio, Rekor, timestamp, and TUF/trusted-root configuration are supported.
- Public Sigstore is the default test configuration.
- RHTAS endpoints can be selected without changing the skill format.
- Trust-root loading and update behavior are documented.
- Verification can use bundled evidence without contacting a service at
  execution time.

### 6. Implement keyless skill signing

**Labels:** `enhancement`, `security`

Sign the canonical manifest digest using Sigstore-compatible keyless signing.

**Acceptance criteria**

- The command produces the manifest and bundle in `.skill-trust/`.
- Public Sigstore keyless signing works in a controlled test workflow.
- RHTAS keyless signing works with configured Fulcio/Rekor/trusted-root
  values.
- Bundle identity and provenance claims are preserved for verification policy.
- The skill content is never modified by the signing operation.

### 7. Implement key-based skill signing

**Labels:** `enhancement`, `security`

Add key-based signing using the same manifest and bundle model.

**Acceptance criteria**

- Local key signing works.
- KMS or hardware-backed key URIs are supported where the Sigstore library
  supports them.
- Key-based and keyless signatures produce the same verification result shape.
- Policy can distinguish and constrain key-based versus keyless identities.

## Epic 4: Go binary and library

### 8. Create the Go module and reusable verification API

**Labels:** `enhancement`

Create the portable Go implementation and public preflight API.

**Acceptance criteria**

- A Go module builds on supported desktop platforms.
- Manifest generation, signature verification, and policy evaluation are
  independently callable as library functions.
- The library never executes skill content.
- Verification results include structured findings and an overall allow/block
  decision.
- Errors distinguish malformed input, unsupported content, cryptographic
  failure, policy failure, and unavailable trust material.

### 9. Add the `skill-trust` CLI

**Labels:** `enhancement`

Expose the library through a stable binary interface.

**Acceptance criteria**

- Implement `manifest`, `sign`, `verify`, and `inspect` commands.
- Support human-readable and JSON output.
- Document stable exit codes.
- `verify` is fail closed by default.
- `verify` does not execute, import, or interpret skill scripts.
- `--help` includes examples for public Sigstore and RHTAS configuration.

## Epic 5: Harness enforcement

### 10. Define the harness preflight contract

**Labels:** `design`, `enhancement`, `security`

Define the integration contract used by OpenCode and future harness adapters.

**Acceptance criteria**

- The contract verifies before instruction loading.
- The contract associates active execution with a verified content digest.
- Content changes trigger re-verification.
- Block reasons are machine-readable and user-readable.
- The contract explicitly covers script execution across Bash, Python, and
  other helper runtimes.

### 11. Build the OpenCode reference adapter

**Labels:** `enhancement`, `documentation`

Provide a working OpenCode integration using its skill permissions and
pre-execution plugin surfaces.

**Acceptance criteria**

- A skill is verified before OpenCode loads it.
- A modified skill cannot be loaded or executed.
- A modified Bash helper is blocked before execution.
- A modified Python helper is blocked before execution.
- The adapter emits the expected and observed content digests.
- Configuration includes trusted signer and RHTAS/public Sigstore examples.
- The adapter does not reimplement manifest or signature logic.

## Epic 6: CI and release distribution

### 12. Add pull-request verification GitHub Action

**Labels:** `enhancement`, `github-actions`

Verify changed skills in pull requests.

**Acceptance criteria**

- The action discovers configured skill directories.
- It fails when content changes without a valid replacement signature.
- It publishes human-readable and JSON reports.
- It supports signer/provenance policy inputs.
- It can verify with public Sigstore or configured RHTAS trust material.

### 13. Add trusted release signing workflow

**Labels:** `enhancement`, `github-actions`, `security`

Sign approved skills in trusted release/tag workflows.

**Acceptance criteria**

- GitHub OIDC keyless signing is supported.
- Signing runs only from explicitly trusted branches/tags/environments.
- Release assets include the skill package, manifest, bundle, and verification
  report.
- The workflow documents identity and issuer constraints.
- The workflow can target public Sigstore and RHTAS.

## Epic 7: Test signing and tamper demonstrations

### 14. Create cross-platform signed skill fixtures

**Labels:** `testing`, `help wanted`

Create safe fixtures that exercise the complete signed boundary.

**Acceptance criteria**

- Fixtures cover Markdown, Bash, Python, references, assets, hidden files,
  executable bits, added files, deleted files, and renamed files.
- Fixtures contain no real secrets or harmful payloads.
- Manifest test vectors are stable across supported platforms.
- Every fixture has expected allow/block results.

### 15. Add public Sigstore end-to-end signing tests

**Labels:** `testing`, `github-actions`

Provide a repeatable keyless test-signing process using public Sigstore.

**Acceptance criteria**

- CI signs a disposable fixture using GitHub OIDC.
- CI verifies the resulting bundle and policy.
- CI modifies the fixture and demonstrates verification failure.
- CI demonstrates that the OpenCode example blocks the modified fixture.
- Test artifacts do not expose private signing material.

### 16. Add RHTAS integration tests and operator guide

**Labels:** `testing`, `documentation`, `rhtas`

Document and test the same flow against RHTAS.

**Acceptance criteria**

- Required RHTAS endpoints and trust-root inputs are documented.
- The test process works with an approved disposable/shared RHTAS instance.
- A signed RHTAS bundle verifies using the local binary.
- RHTAS verification failures are covered by fixtures.
- No test depends on an OCI registry.

## Epic 8: Enterprise lifecycle

### 17. Add signer rotation and revocation guidance

**Labels:** `design`, `security`, `documentation`

Define how organizations handle signer changes, trust-root updates, and
withdrawn skills.

**Acceptance criteria**

- Rotation and trust-root update procedures are documented.
- Stale or revoked evidence has a defined verification result.
- Existing packages remain inspectable but cannot execute under a policy that
  rejects their signer or evidence.
- The design distinguishes cryptographic validity from current authorization.

### 18. Add organization policy examples

**Labels:** `documentation`, `enhancement`

Provide policies for common enterprise cases.

**Acceptance criteria**

- Example policy trusts a specific RHTAS issuer and signer identity.
- Example policy trusts a GitHub Actions repository identity.
- Example policy requires offline-capable bundle verification.
- Example policy blocks unsigned or unknown skills.
- Every policy example has a passing and failing fixture.

## Definition of done for the MVP

The MVP is complete when all of the following work together:

1. A complete skill directory containing `SKILL.md`, Bash, Python, and asset
   files can be manifested and signed.
2. A local binary can verify it offline with public Sigstore trust material.
3. The same package can be verified against configured RHTAS services/trust
   roots.
4. A content modification produces a block decision.
5. The OpenCode reference adapter prevents the modified skill from loading or
   executing.
6. GitHub Actions can verify pull requests and sign trusted releases.
7. The repository contains a reproducible test-signing and tamper-demonstration
   workflow.

