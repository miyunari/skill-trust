# skill-trust architecture

This document defines the initial design boundary for signing and verifying
Agent Skills. It is a design contract for the implementation backlog.

## System model

```text
skill directory
      │
      ▼
canonical manifest ──► manifest digest ──► Sigstore/RHTAS signature bundle
      │                                           │
      └────────────── distributed in .skill-trust/┘
                                                  │
                                                  ▼
                                    verifier + organization policy
                                                  │
                                      allow or fail closed
                                                  │
                                                  ▼
                                      harness loads/executes skill
```

The signed object is a canonical manifest of the skill contents. The signature
does not cover the `.skill-trust/` directory because that directory contains
the manifest and bundle needed to verify the content and would otherwise make
the digest self-referential.

## Skill boundary

The input is a directory containing a required `SKILL.md` file. Every regular
file below that directory is included except files below `.skill-trust/`.
This includes:

- Markdown instructions and YAML frontmatter;
- Bash, Python, JavaScript, and other scripts;
- references and documentation;
- templates and other assets;
- hidden files; and
- files in directories not named by the Agent Skills conventions.

The first implementation rejects symlinks, device files, sockets, FIFOs, and
other special entries. This prevents a manifest from silently describing a
different object on another host. A later format version may define explicit
symlink semantics.

## Canonical manifest rules

The manifest implementation must:

1. Require `SKILL.md` at the skill root.
2. Walk the directory without following symlinks.
3. Reject absolute paths, `..` path components, duplicate normalized paths,
   and unsupported entry types.
4. Represent paths as normalized relative paths with `/` separators.
5. Sort entries lexicographically by normalized path.
6. Hash regular-file contents with SHA-256 in v1.
7. Include executable permission bits for regular files.
8. Exclude timestamps, owners, groups, device numbers, absolute paths, and
   host-specific metadata.
9. Serialize the manifest with a versioned, deterministic JSON encoding.
10. Hash the canonical manifest bytes and use that digest as the signed
    subject.

The manifest format must include its version and hash algorithm so future
format changes fail clearly instead of being interpreted as equivalent.

## Control files

Reserved files are stored under `.skill-trust/`:

```text
.skill-trust/
├── manifest.json
├── bundle.sigstore.json
└── report.json              # optional generated verification report
```

The v1 verifier must require `manifest.json` and a supported signature bundle.
The optional report is informational and is never trusted as verification
input.

The verifier recomputes the manifest from the directory, compares it with the
embedded manifest, and verifies the signature over the recomputed manifest
digest. A changed, missing, malformed, or extra control file must not create a
second signed content boundary.

## Signing modes

### Keyless mode

Keyless signing is the first implementation target. The signer obtains an
ephemeral certificate from Fulcio or an RHTAS-compatible service, records the
signature and transparency evidence in a bundle, and binds policy to the
certificate identity and issuer.

Public Sigstore is used for repeatable CI tests. RHTAS is supported by loading
configured service URLs and trusted-root material rather than hard-coding the
public-good endpoints.

### Key-based mode

Key-based signing uses the same canonical manifest and bundle verification
path. Only signer acquisition and signer-policy configuration differ. The
implementation must not create a separate skill-specific cryptographic format.

## Verification policy

An allow decision requires all of the following:

- the recomputed manifest matches the embedded manifest;
- the signature is cryptographically valid;
- the bundle's certificate, timestamp, transparency evidence, and trust root
  satisfy the configured verification mode;
- at least one signature matches the configured signer identity/issuer policy;
- required provenance constraints match; and
- all required verification material is available, either from the bundle or
  configured local trust roots.

Multiple signatures may be present. One policy-compliant signature is enough
for an allow decision, but every discovered signature and its result is
reported.

Missing, invalid, stale, ambiguous, or unverifiable evidence produces a block
decision. The default is fail closed.

## Harness preflight contract

A harness adapter must perform this sequence:

1. Resolve a skill path without executing or loading its instructions.
2. Call the verifier with the configured policy and trust roots.
3. Load `SKILL.md` only after an allow result.
4. Associate the active skill with the verified manifest digest.
5. Re-verify before skill-associated script or tool execution if the skill
   changed, was reloaded, or crossed a process boundary.
6. Block execution and report the expected/observed digest on mismatch.

The adapter must expose structured block reasons for logging and user-facing
messages. It must not implement a second manifest algorithm.

## OpenCode reference integration

The OpenCode adapter will use its skill permission and plugin pre-execution
surfaces. It must verify before allowing the `skill` action and must guard
skill-associated execution through the adapter's verified-skill state.

The example must cover both:

- a signed skill whose Bash/Python helper executes successfully; and
- a helper modified after signing that is denied before execution.

The adapter is a reference implementation, not a claim that every OpenCode
execution path is protected when the adapter is disabled or bypassed.

## Distribution

Skills are distributed as directories or portable packages containing the skill
contents and `.skill-trust/` metadata. OCI registries are not required. If a
package is archived, extraction must preserve regular-file contents and
executable bits before verification; archive timestamps and owners are not part
of the skill identity.

## Non-goals

This architecture does not provide:

- proof that instructions are safe or non-malicious;
- runtime sandboxing;
- host permission enforcement;
- secret scanning as a complete security control;
- automatic revocation of already-installed skills; or
- protection against a harness that intentionally bypasses the verifier.

