package claudecode

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redhat-et/skill-trust/pkg/preflight"
)

// Outcome is the result of a preflight decision.
type Outcome struct {
	Root    string                // Skill root
	Allowed bool                  // Whether to allow
	Warning string                // Warning message (report-only mode)
	Block   *preflight.BlockError // Block error if denied
}

// Decide implements the Claude Code adapter decision table. It runs full
// verification and decides whether to allow or block based on the result,
// previous record, and strict mode.
//
// Returns the outcome and a new record to store (nil = leave unchanged).
func Decide(v preflight.Verifier, root string, prev *Record, strict bool, now time.Time) (Outcome, *Record) {
	// Run verification
	result, err := v.VerifySkill(root)

	// Handle verifier errors
	if err != nil {
		outcome := Outcome{
			Root:    root,
			Allowed: false,
			Block: &preflight.BlockError{
				Code:    preflight.CodeVerifierFailure,
				Message: fmt.Sprintf("verification of %s failed: %v", root, err),
			},
		}
		// In non-strict mode with no previous record, allow with warning
		if !strict && prev == nil {
			outcome.Allowed = true
			outcome.Warning = fmt.Sprintf("skill %s verification failed but allowed in non-strict mode: %v", root, err)
			outcome.Block = nil
		}
		return outcome, nil
	}

	// Determine if unsigned
	unsigned := isUnsigned(root, result)

	// Decision table implementation
	if result.Allowed() {
		// Verification now: allow
		if prev == nil {
			// No previous record → allow, record digest
			newRecord := &Record{
				Allowed:    true,
				Digest:     result.ManifestDigest,
				VerifiedAt: now,
			}
			return Outcome{
				Root:    root,
				Allowed: true,
			}, newRecord
		}

		if prev.Allowed {
			// Previous record was allowed
			if prev.Digest == result.ManifestDigest {
				// Same digest → allow (no new record needed)
				return Outcome{
					Root:    root,
					Allowed: true,
				}, nil
			}

			// Different digest → block CodeChangedAfterVerify (sticky)
			return Outcome{
				Root:    root,
				Allowed: false,
				Block: &preflight.BlockError{
					Code:           preflight.CodeChangedAfterVerify,
					Message:        fmt.Sprintf("skill %s changed after it was verified", root),
					ExpectedDigest: prev.Digest,
					ObservedDigest: result.ManifestDigest,
				},
			}, nil
		}

		// Previous record was not allowed → block CodeChangedAfterVerify (sticky)
		return Outcome{
			Root:    root,
			Allowed: false,
			Block: &preflight.BlockError{
				Code:           preflight.CodeChangedAfterVerify,
				Message:        fmt.Sprintf("skill %s was previously blocked", root),
				ExpectedDigest: prev.Digest,
				ObservedDigest: result.ManifestDigest,
			},
		}, nil
	}

	// Verification now: block
	if prev != nil && prev.Allowed {
		// Was previously allowed → block CodeChangedAfterVerify (sticky)
		return Outcome{
			Root:    root,
			Allowed: false,
			Block: &preflight.BlockError{
				Code:               preflight.CodeChangedAfterVerify,
				Message:            fmt.Sprintf("skill %s changed or no longer satisfies policy after it was verified", root),
				ExpectedDigest:     prev.Digest,
				ObservedDigest:     result.ObservedDigest,
				UnderlyingFindings: result.Findings,
			},
		}, nil
	}

	// No previous allowed record (none or not-allowed)
	if strict {
		// Strict mode → block
		code := preflight.CodeVerifierFailure
		if unsigned {
			code = preflight.CodeNotVerified
		}

		first := ""
		if len(result.Findings) > 0 {
			first = fmt.Sprintf("[%s] %s", result.Findings[0].Code, result.Findings[0].Message)
		}

		return Outcome{
			Root:    root,
			Allowed: false,
			Block: &preflight.BlockError{
				Code:               code,
				Message:            fmt.Sprintf("skill %s blocked: %s", root, first),
				ExpectedDigest:     result.ManifestDigest,
				ObservedDigest:     result.ObservedDigest,
				UnderlyingFindings: result.Findings,
			},
		}, nil
	}

	// Non-strict mode → allow with warning
	first := ""
	if len(result.Findings) > 0 {
		first = fmt.Sprintf("[%s] %s", result.Findings[0].Code, result.Findings[0].Message)
	}

	return Outcome{
		Root:    root,
		Allowed: true,
		Warning: fmt.Sprintf("skill %s does not satisfy policy but allowed in non-strict mode: %s", root, first),
	}, nil
}

// isUnsigned determines if a skill is unsigned based on the controller ruling.
// A root is unsigned iff:
// 1. The directory <root>/.skill-trust does not exist, OR
// 2. The result findings contain a ST-METADATA-* code
func isUnsigned(root string, result preflight.Result) bool {
	// Check if .skill-trust directory exists
	skillTrustDir := filepath.Join(root, ".skill-trust")
	if _, err := os.Stat(skillTrustDir); os.IsNotExist(err) {
		return true
	}

	// Check for ST-METADATA-* codes in findings
	for _, f := range result.Findings {
		if strings.HasPrefix(f.Code, "ST-METADATA-") {
			return true
		}
	}

	return false
}

// Reason renders an Outcome as a single human-readable string with code,
// message, root, digests when present, and up to 5 underlying findings.
func Reason(o Outcome) string {
	if o.Allowed && o.Warning != "" {
		return o.Warning
	}

	if o.Block == nil {
		return fmt.Sprintf("skill %s allowed", o.Root)
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("[%s] %s", o.Block.Code, o.Block.Message))
	parts = append(parts, fmt.Sprintf("root: %s", o.Root))

	if o.Block.ExpectedDigest != "" && o.Block.ObservedDigest != "" {
		parts = append(parts, fmt.Sprintf("expected digest %s", o.Block.ExpectedDigest))
		parts = append(parts, fmt.Sprintf("observed digest %s", o.Block.ObservedDigest))
	}

	// Include up to 5 underlying findings
	limit := 5
	for i, f := range o.Block.UnderlyingFindings {
		if i >= limit {
			break
		}
		parts = append(parts, fmt.Sprintf("  [%s] %s", f.Code, f.Message))
	}

	return strings.Join(parts, "\n")
}
