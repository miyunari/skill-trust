package claudecode

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redhat-et/skill-trust/pkg/preflight"
)

// HookInput is the JSON structure Claude Code sends to hooks.
type HookInput struct {
	SessionID      string `json:"session_id"`
	Cwd            string `json:"cwd"`
	HookEventName  string `json:"hook_event_name"`
	ToolName       string `json:"tool_name"`
	ToolInput      any    `json:"tool_input"`
	PermissionMode string `json:"permission_mode"`
	Source         string `json:"source"`
	Prompt         string `json:"prompt"`
}

// Config holds the configuration for hook execution.
type Config struct {
	Policy      string                                                         // Path to the policy file
	Strict      bool                                                           // Strict mode flag
	StateDir    string                                                         // Directory for state files
	PluginRoot  string                                                         // Plugin root directory (unused currently)
	CLIVersion  string                                                         // CLI version string
	Locations   Locations                                                      // Skill search locations
	Now         func() time.Time                                               // Time function (for testing)
	NewVerifier func(policyPath string) (preflight.Verifier, error) // Verifier constructor
}

// PreToolUse handles the PreToolUse hook event. Returns nil for allowed tools
// with no output, or JSON for denied/warning outcomes.
func PreToolUse(cfg Config, in HookInput) []byte {
	// Extract candidate paths from tool input
	candidates := CandidatePaths(in.ToolInput, in.Cwd)

	// Discover all skills
	discovered := cfg.Locations.Discover()

	// For Skill tool, also resolve the skill name
	if in.ToolName == "Skill" {
		if m, ok := in.ToolInput.(map[string]any); ok {
			if skillName, ok := m["skill"].(string); ok && skillName != "" {
				roots := ResolveSkillNames(skillName, discovered)
				for _, root := range roots {
					candidates = append(candidates, root)
				}
			}
		}
	}

	// Find skill roots for all candidates
	var skillRoots []string
	seen := make(map[string]bool)
	for _, candidate := range candidates {
		if root, found := cfg.Locations.RootOf(candidate, discovered); found && !seen[root] {
			skillRoots = append(skillRoots, root)
			seen[root] = true
		}
	}

	// If no skill roots, allow without constructing verifier
	if len(skillRoots) == 0 {
		return nil
	}

	// Construct verifier lazily
	verifier, err := cfg.NewVerifier(cfg.Policy)
	if err != nil {
		// Verifier construction failed - treat as a block in strict mode
		if cfg.Strict {
			return denyJSON(fmt.Sprintf("policy error: %v", err))
		}
		return warningJSON(fmt.Sprintf("policy error (non-strict mode): %v", err))
	}

	// Load previous state
	state := LoadState(cfg.StateDir, in.SessionID)

	// Check each skill root
	now := time.Now()
	if cfg.Now != nil {
		now = cfg.Now()
	}

	var outcomes []Outcome
	for _, root := range skillRoots {
		prev, hasPrev := state[root]
		var prevPtr *Record
		if hasPrev {
			prevPtr = &prev
		}

		outcome, newRec := Decide(verifier, root, prevPtr, cfg.Strict, now)
		outcomes = append(outcomes, outcome)

		// Update state if we got a new record
		if newRec != nil {
			state[root] = *newRec
		}
	}

	// Save updated state
	if err := SaveState(cfg.StateDir, in.SessionID, state); err != nil {
		// Log but don't fail - state save is best-effort
	}

	// Determine overall result: deny if any is blocked, warn if any has warnings
	var blocked []Outcome
	var warnings []Outcome

	for _, o := range outcomes {
		if !o.Allowed && o.Block != nil {
			blocked = append(blocked, o)
		} else if o.Allowed && o.Warning != "" {
			warnings = append(warnings, o)
		}
	}

	// If any blocked, return deny
	if len(blocked) > 0 {
		// Use the first blocked reason
		return denyJSON(Reason(blocked[0]))
	}

	// If any warnings (report-only mode), return warning
	if len(warnings) > 0 {
		var parts []string
		for _, w := range warnings {
			parts = append(parts, w.Warning)
		}
		return warningJSON(strings.Join(parts, "\n"))
	}

	// All allowed, no warnings
	return nil
}

// UserPromptSubmit handles the UserPromptSubmit hook event for slash commands.
func UserPromptSubmit(cfg Config, in HookInput) []byte {
	// Check if prompt starts with /
	trimmed := strings.TrimSpace(in.Prompt)
	if !strings.HasPrefix(trimmed, "/") {
		return nil
	}

	// Extract skill name (first token without the /)
	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return nil
	}
	skillName := strings.TrimPrefix(parts[0], "/")

	// Discover all skills
	discovered := cfg.Locations.Discover()

	// Resolve skill name to roots
	roots := ResolveSkillNames(skillName, discovered)
	if len(roots) == 0 {
		// Unknown skill - allow
		return nil
	}

	// Construct verifier lazily
	verifier, err := cfg.NewVerifier(cfg.Policy)
	if err != nil {
		if cfg.Strict {
			return blockPromptJSON(fmt.Sprintf("policy error: %v", err))
		}
		return warningPromptJSON(fmt.Sprintf("policy error (non-strict mode): %v", err))
	}

	// Load previous state
	state := LoadState(cfg.StateDir, in.SessionID)

	now := time.Now()
	if cfg.Now != nil {
		now = cfg.Now()
	}

	// Check each resolved root
	var outcomes []Outcome
	for _, root := range roots {
		prev, hasPrev := state[root]
		var prevPtr *Record
		if hasPrev {
			prevPtr = &prev
		}

		outcome, newRec := Decide(verifier, root, prevPtr, cfg.Strict, now)
		outcomes = append(outcomes, outcome)

		// Update state if we got a new record
		if newRec != nil {
			state[root] = *newRec
		}
	}

	// Save updated state
	SaveState(cfg.StateDir, in.SessionID, state)

	// Determine overall result
	var blocked []Outcome
	var warnings []Outcome

	for _, o := range outcomes {
		if !o.Allowed && o.Block != nil {
			blocked = append(blocked, o)
		} else if o.Allowed && o.Warning != "" {
			warnings = append(warnings, o)
		}
	}

	// If any blocked, return block
	if len(blocked) > 0 {
		return blockPromptJSON(Reason(blocked[0]))
	}

	// If any warnings (report-only mode), return warning
	if len(warnings) > 0 {
		var parts []string
		for _, w := range warnings {
			parts = append(parts, w.Warning)
		}
		return warningPromptJSON(strings.Join(parts, "\n"))
	}

	// All allowed
	return nil
}

// SessionStart handles the SessionStart hook event. It discovers all skills,
// verifies each one, saves fresh state, prunes old state, and returns a system
// message summarizing the results.
func SessionStart(cfg Config, in HookInput) []byte {
	// Prune old state (>7 days)
	if cfg.Now != nil {
		PruneStates(cfg.StateDir, 7*24*time.Hour, cfg.Now())
	} else {
		PruneStates(cfg.StateDir, 7*24*time.Hour, time.Now())
	}

	// Discover all skills
	discovered := cfg.Locations.Discover()

	// If no skills, return minimal message
	if len(discovered) == 0 {
		msg := "skill-trust: 0 skills discovered"
		if cfg.Policy == "" {
			msg += "; policy not configured"
		}
		return sessionStartJSON(msg, "")
	}

	// Construct verifier (even if policy missing, to get consistent error handling)
	var verifier preflight.Verifier
	var verifierErr error
	if cfg.Policy != "" {
		verifier, verifierErr = cfg.NewVerifier(cfg.Policy)
	} else {
		verifierErr = fmt.Errorf("policy not configured")
	}

	now := time.Now()
	if cfg.Now != nil {
		now = cfg.Now()
	}

	// Verify all discovered skills (fresh baseline, no previous state)
	var allowed []Skill
	var blocked []struct {
		Skill
		Code string
	}

	newState := make(State)

	for _, skill := range discovered {
		if verifierErr != nil {
			// Policy/verifier error - treat as block in strict mode
			if cfg.Strict {
				code := "ST-PREFLIGHT-VERIFIER-FAILURE"
				blocked = append(blocked, struct {
					Skill
					Code string
				}{skill, code})
			} else {
				allowed = append(allowed, skill)
			}
			continue
		}

		outcome, newRec := Decide(verifier, skill.Root, nil, cfg.Strict, now)

		if outcome.Allowed {
			allowed = append(allowed, skill)
		} else if outcome.Block != nil {
			blocked = append(blocked, struct {
				Skill
				Code string
			}{skill, string(outcome.Block.Code)})
		}

		// Save the new record
		if newRec != nil {
			newState[skill.Root] = *newRec
		}
	}

	// Save fresh state
	SaveState(cfg.StateDir, in.SessionID, newState)

	// Build system message
	var msgParts []string
	msgParts = append(msgParts, fmt.Sprintf("skill-trust: %d skills verified, %d blocked",
		len(allowed), len(blocked)))

	if len(blocked) > 0 {
		var names []string
		for _, b := range blocked {
			names = append(names, fmt.Sprintf("%s: %s", b.Name, b.Code))
		}
		msgParts = append(msgParts, fmt.Sprintf("(%s)", strings.Join(names, ", ")))
	}

	// Add version warning placeholder (Task 5)
	// versionWarning(cfg) would go here

	if cfg.Policy == "" {
		msgParts = append(msgParts, "; policy not configured")
	}

	systemMessage := strings.Join(msgParts, " ")

	// Build additional context for blocked skills
	var additionalContext string
	if len(blocked) > 0 {
		var lines []string
		lines = append(lines, "The following skills are blocked and should not be used:")
		for _, b := range blocked {
			lines = append(lines, fmt.Sprintf("  - %s (%s)", b.Name, b.Code))
		}
		additionalContext = strings.Join(lines, "\n")
	}

	return sessionStartJSON(systemMessage, additionalContext)
}

// Helper functions for JSON output

func denyJSON(reason string) []byte {
	out := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":             "PreToolUse",
			"permissionDecision":        "deny",
			"permissionDecisionReason":  reason,
		},
	}
	data, _ := json.Marshal(out)
	return data
}

func warningJSON(context string) []byte {
	out := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":      "PreToolUse",
			"additionalContext":  context,
		},
	}
	data, _ := json.Marshal(out)
	return data
}

func blockPromptJSON(reason string) []byte {
	out := map[string]any{
		"decision": "block",
		"reason":   reason,
	}
	data, _ := json.Marshal(out)
	return data
}

func warningPromptJSON(context string) []byte {
	out := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     "UserPromptSubmit",
			"additionalContext": context,
		},
	}
	data, _ := json.Marshal(out)
	return data
}

func sessionStartJSON(systemMessage, additionalContext string) []byte {
	out := map[string]any{
		"systemMessage": systemMessage,
		"hookSpecificOutput": map[string]any{
			"hookEventName": "SessionStart",
		},
	}
	if additionalContext != "" {
		out["hookSpecificOutput"].(map[string]any)["additionalContext"] = additionalContext
	}
	data, _ := json.Marshal(out)
	return data
}
