package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupHookTest creates a home directory with a skill and returns the skill root.
func setupHookTest(t *testing.T, home, skillName string, tamper bool) string {
	t.Helper()

	// Create skill directory
	skillRoot := filepath.Join(home, ".claude", "skills", skillName)
	if err := os.MkdirAll(filepath.Join(skillRoot, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Write skill files
	write := func(rel, content string, mode os.FileMode) {
		full := filepath.Join(skillRoot, rel)
		if err := os.WriteFile(full, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}

	write("SKILL.md", "---\nname: "+skillName+"\n---\nTest skill.\n", 0o644)
	write("scripts/run.sh", "#!/usr/bin/env bash\necho hi\n", 0o755)
	write("scripts/util.py", "print('hi')\n", 0o644)

	// Sign the skill
	keyPath, pubPath := newKey(t)
	policyPath := keyPolicy(t, pubPath)

	// Copy policy to home for tests
	policyDest := filepath.Join(home, "policy.json")
	policyData, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policyDest, policyData, 0o644); err != nil {
		t.Fatal(err)
	}

	// Sign
	code, out := runCLI(t, "sign", skillRoot, "--key", keyPath)
	if code != ExitSuccess {
		t.Fatalf("sign exit = %d, output:\n%s", code, out)
	}

	// Tamper if requested
	if tamper {
		write("SKILL.md", "---\nname: "+skillName+"\n---\nTampered!\n", 0o644)
	}

	return skillRoot
}

// runHook executes the hook command with JSON input and returns exit code and output.
func runHook(t *testing.T, subcommand string, input []byte, extraArgs ...string) (int, string) {
	t.Helper()

	args := append([]string{"hook", "claude-code", subcommand}, extraArgs...)

	// Reset global command state
	exitCode = ExitSuccess
	jsonOutput = false
	manifestWrite = false
	verifyPolicy = ""
	verifyReport = false
	signKey, signIDToken, signServices = "", "", ""
	hookPolicyPath = ""
	hookStrict = true

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs(args)
	rootCmd.SetIn(bytes.NewReader(input))

	err := rootCmd.Execute()
	code := exitCode
	if err != nil && code == ExitSuccess {
		code = ExitError
	}
	return code, buf.String()
}

func TestPreToolUseAllowsVerifiedSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_PLUGIN_DATA", filepath.Join(home, ".claude"))
	t.Setenv("CLAUDE_PROJECT_DIR", "")

	setupHookTest(t, home, "cli-test", false)
	policyPath := filepath.Join(home, "policy.json")

	input := `{
		"session_id": "test-session",
		"cwd": "/tmp",
		"hook_event_name": "PreToolUse",
		"tool_name": "Read",
		"tool_input": {
			"file_path": "` + filepath.Join(home, ".claude/skills/cli-test/SKILL.md") + `"
		},
		"permission_mode": "default"
	}`

	code, out := runHook(t, "pre-tool-use", []byte(input), "--policy", policyPath)
	if code != ExitSuccess {
		t.Fatalf("exit = %d, expected 0, output:\n%s", code, out)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected empty output for allowed skill, got: %s", out)
	}
}

func TestPreToolUseDeniesTamperedRead(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_PLUGIN_DATA", filepath.Join(home, ".claude"))
	t.Setenv("CLAUDE_PROJECT_DIR", "")

	setupHookTest(t, home, "cli-test", true)
	policyPath := filepath.Join(home, "policy.json")

	input := `{
		"session_id": "test-session",
		"cwd": "/tmp",
		"hook_event_name": "PreToolUse",
		"tool_name": "Read",
		"tool_input": {
			"file_path": "` + filepath.Join(home, ".claude/skills/cli-test/SKILL.md") + `"
		},
		"permission_mode": "default"
	}`

	code, out := runHook(t, "pre-tool-use", []byte(input), "--policy", policyPath)
	if code != ExitSuccess {
		t.Fatalf("exit = %d, expected 0 (hooks don't error on deny), output:\n%s", code, out)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}

	hookOutput, ok := result["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatalf("missing hookSpecificOutput: %v", result)
	}

	if hookOutput["permissionDecision"] != "deny" {
		t.Errorf("permissionDecision = %v, want deny", hookOutput["permissionDecision"])
	}

	reason := hookOutput["permissionDecisionReason"].(string)
	if !strings.Contains(reason, "ST-") {
		t.Errorf("reason should contain ST- code, got: %s", reason)
	}
}

func TestPreToolUseDeniesTamperedBash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_PLUGIN_DATA", filepath.Join(home, ".claude"))
	t.Setenv("CLAUDE_PROJECT_DIR", "")

	skillRoot := setupHookTest(t, home, "cli-test", true)
	policyPath := filepath.Join(home, "policy.json")

	input := `{
		"session_id": "test-session",
		"cwd": "/tmp",
		"hook_event_name": "PreToolUse",
		"tool_name": "Bash",
		"tool_input": {
			"command": "bash ` + filepath.Join(skillRoot, "scripts/run.sh") + `"
		},
		"permission_mode": "default"
	}`

	code, out := runHook(t, "pre-tool-use", []byte(input), "--policy", policyPath)
	if code != ExitSuccess {
		t.Fatalf("exit = %d, expected 0, output:\n%s", code, out)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}

	hookOutput := result["hookSpecificOutput"].(map[string]any)
	if hookOutput["permissionDecision"] != "deny" {
		t.Errorf("permissionDecision = %v, want deny", hookOutput["permissionDecision"])
	}
}

func TestPreToolUseDeniesTamperedPython(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_PLUGIN_DATA", filepath.Join(home, ".claude"))
	t.Setenv("CLAUDE_PROJECT_DIR", "")

	skillRoot := setupHookTest(t, home, "cli-test", true)
	policyPath := filepath.Join(home, "policy.json")

	input := `{
		"session_id": "test-session",
		"cwd": "` + skillRoot + `",
		"hook_event_name": "PreToolUse",
		"tool_name": "Bash",
		"tool_input": {
			"command": "python3 scripts/util.py"
		},
		"permission_mode": "default"
	}`

	code, out := runHook(t, "pre-tool-use", []byte(input), "--policy", policyPath)
	if code != ExitSuccess {
		t.Fatalf("exit = %d, expected 0, output:\n%s", code, out)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}

	hookOutput := result["hookSpecificOutput"].(map[string]any)
	if hookOutput["permissionDecision"] != "deny" {
		t.Errorf("permissionDecision = %v, want deny", hookOutput["permissionDecision"])
	}
}

func TestPreToolUseDeniesTamperedSkillTool(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_PLUGIN_DATA", filepath.Join(home, ".claude"))
	t.Setenv("CLAUDE_PROJECT_DIR", "")

	setupHookTest(t, home, "cli-test", true)
	policyPath := filepath.Join(home, "policy.json")

	input := `{
		"session_id": "test-session",
		"cwd": "/tmp",
		"hook_event_name": "PreToolUse",
		"tool_name": "Skill",
		"tool_input": {
			"skill": "cli-test"
		},
		"permission_mode": "default"
	}`

	code, out := runHook(t, "pre-tool-use", []byte(input), "--policy", policyPath)
	if code != ExitSuccess {
		t.Fatalf("exit = %d, expected 0, output:\n%s", code, out)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}

	hookOutput := result["hookSpecificOutput"].(map[string]any)
	if hookOutput["permissionDecision"] != "deny" {
		t.Errorf("permissionDecision = %v, want deny", hookOutput["permissionDecision"])
	}
}

func TestPreToolUseChangedAfterVerifyIsSticky(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_PLUGIN_DATA", filepath.Join(home, ".claude"))
	t.Setenv("CLAUDE_PROJECT_DIR", "")

	skillRoot := setupHookTest(t, home, "cli-test", false)
	policyPath := filepath.Join(home, "policy.json")

	// Session start to record the verified state
	sessionInput := `{
		"session_id": "test-session",
		"cwd": "/tmp",
		"hook_event_name": "SessionStart"
	}`
	code, out := runHook(t, "session-start", []byte(sessionInput), "--policy", policyPath)
	if code != ExitSuccess {
		t.Fatalf("session-start exit = %d, output:\n%s", code, out)
	}

	// Tamper with the skill
	tamperedContent := "---\nname: cli-test\n---\nTampered after verify!\n"
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte(tamperedContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Try to use the skill - should be denied
	input := `{
		"session_id": "test-session",
		"cwd": "/tmp",
		"hook_event_name": "PreToolUse",
		"tool_name": "Skill",
		"tool_input": {
			"skill": "cli-test"
		},
		"permission_mode": "default"
	}`

	code, out = runHook(t, "pre-tool-use", []byte(input), "--policy", policyPath)
	if code != ExitSuccess {
		t.Fatalf("pre-tool-use exit = %d, output:\n%s", code, out)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}

	hookOutput := result["hookSpecificOutput"].(map[string]any)
	reason := hookOutput["permissionDecisionReason"].(string)
	if !strings.Contains(reason, "ST-PREFLIGHT-CHANGED-AFTER-VERIFY") {
		t.Errorf("expected ST-PREFLIGHT-CHANGED-AFTER-VERIFY, got: %s", reason)
	}

	// Re-sign with different content - should still be denied (sticky)
	keyPath, _ := newKey(t)
	differentContent := "---\nname: cli-test\n---\nDifferent content!\n"
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte(differentContent), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out = runCLI(t, "sign", skillRoot, "--key", keyPath)
	if code != ExitSuccess {
		t.Fatalf("re-sign exit = %d, output:\n%s", code, out)
	}

	code, out = runHook(t, "pre-tool-use", []byte(input), "--policy", policyPath)
	if code != ExitSuccess {
		t.Fatalf("pre-tool-use after re-sign exit = %d, output:\n%s", code, out)
	}

	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}

	hookOutput = result["hookSpecificOutput"].(map[string]any)
	reason = hookOutput["permissionDecisionReason"].(string)
	if !strings.Contains(reason, "ST-PREFLIGHT-CHANGED-AFTER-VERIFY") {
		t.Errorf("should still be denied after re-sign, got: %s", reason)
	}

	// New session should clear the sticky state
	code, out = runHook(t, "session-start", []byte(sessionInput), "--policy", policyPath)
	if code != ExitSuccess {
		t.Fatalf("new session-start exit = %d, output:\n%s", code, out)
	}

	// Now should be allowed (if we update the policy to accept the new key)
	// For this test, we'll just verify that a new session was recorded
}

func TestPreToolUseUnsignedStrictAndReportOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_PLUGIN_DATA", filepath.Join(home, ".claude"))
	t.Setenv("CLAUDE_PROJECT_DIR", "")

	// Create unsigned skill
	skillRoot := filepath.Join(home, ".claude", "skills", "unsigned-skill")
	if err := os.MkdirAll(skillRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte("# Unsigned"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a minimal policy (won't match unsigned skill)
	_, pubPath := newKey(t)
	policyPath := keyPolicy(t, pubPath)

	input := `{
		"session_id": "test-session",
		"cwd": "/tmp",
		"hook_event_name": "PreToolUse",
		"tool_name": "Skill",
		"tool_input": {
			"skill": "unsigned-skill"
		},
		"permission_mode": "default"
	}`

	// Strict mode (default) - should deny
	code, out := runHook(t, "pre-tool-use", []byte(input), "--policy", policyPath)
	if code != ExitSuccess {
		t.Fatalf("exit = %d, output:\n%s", code, out)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}

	hookOutput := result["hookSpecificOutput"].(map[string]any)
	if hookOutput["permissionDecision"] != "deny" {
		t.Errorf("strict mode should deny unsigned skill, got: %v", hookOutput)
	}

	reason := hookOutput["permissionDecisionReason"].(string)
	if !strings.Contains(reason, "ST-PREFLIGHT-NOT-VERIFIED") {
		t.Errorf("expected ST-PREFLIGHT-NOT-VERIFIED, got: %s", reason)
	}

	// Non-strict mode - should allow with warning
	code, out = runHook(t, "pre-tool-use", []byte(input), "--policy", policyPath, "--strict=false")
	if code != ExitSuccess {
		t.Fatalf("exit = %d, output:\n%s", code, out)
	}

	if strings.TrimSpace(out) == "" {
		t.Fatal("expected warning output in non-strict mode")
	}

	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}

	hookOutput = result["hookSpecificOutput"].(map[string]any)
	if _, hasDeny := hookOutput["permissionDecision"]; hasDeny {
		t.Errorf("non-strict mode should not deny, got: %v", hookOutput)
	}

	context, ok := hookOutput["additionalContext"].(string)
	if !ok || context == "" {
		t.Errorf("expected additionalContext warning, got: %v", hookOutput)
	}
}

func TestPreToolUseMissingPolicy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_PLUGIN_DATA", filepath.Join(home, ".claude"))
	t.Setenv("CLAUDE_PROJECT_DIR", "")

	setupHookTest(t, home, "cli-test", false)

	input := `{
		"session_id": "test-session",
		"cwd": "/tmp",
		"hook_event_name": "PreToolUse",
		"tool_name": "Skill",
		"tool_input": {
			"skill": "cli-test"
		},
		"permission_mode": "default"
	}`

	// Missing policy in strict mode should deny skill paths
	code, out := runHook(t, "pre-tool-use", []byte(input), "--policy", "/nonexistent/policy.json")
	if code != ExitSuccess {
		t.Fatalf("exit = %d, output:\n%s", code, out)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}

	hookOutput := result["hookSpecificOutput"].(map[string]any)
	if hookOutput["permissionDecision"] != "deny" {
		t.Errorf("missing policy should deny in strict mode, got: %v", hookOutput)
	}
}

func TestPreToolUseRelativePolicyPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_PLUGIN_DATA", filepath.Join(home, ".claude"))
	t.Setenv("CLAUDE_PROJECT_DIR", "")

	setupHookTest(t, home, "cli-test", false)

	// Copy policy to a relative location from cwd
	policyDir := t.TempDir()
	_, pubPath := newKey(t)
	policyPath := keyPolicy(t, pubPath)
	policyData, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	relPolicyPath := filepath.Join(policyDir, "policy.json")
	if err := os.WriteFile(relPolicyPath, policyData, 0o644); err != nil {
		t.Fatal(err)
	}

	input := `{
		"session_id": "test-session",
		"cwd": "` + policyDir + `",
		"hook_event_name": "PreToolUse",
		"tool_name": "Skill",
		"tool_input": {
			"skill": "cli-test"
		},
		"permission_mode": "default"
	}`

	// Use relative policy path - should work
	if err := os.Chdir(policyDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(home)

	code, out := runHook(t, "pre-tool-use", []byte(input), "--policy", "policy.json")
	if code != ExitSuccess {
		t.Fatalf("exit = %d, output:\n%s", code, out)
	}
}

func TestHookMalformedInput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Garbage input
	code, out := runHook(t, "pre-tool-use", []byte("not json"), "--policy", "/fake/policy.json")
	if code != ExitError {
		t.Errorf("malformed JSON should exit %d, got %d, output:\n%s", ExitError, code, out)
	}
	if !strings.Contains(out, "skill-trust:") {
		t.Errorf("expected error message on stderr, got: %s", out)
	}

	// Invalid tool_input type (should not error, just return empty)
	input := `{
		"session_id": "test",
		"cwd": "/tmp",
		"hook_event_name": "PreToolUse",
		"tool_name": "Bash",
		"tool_input": 42
	}`
	code, out = runHook(t, "pre-tool-use", []byte(input), "--policy", "/fake/policy.json")
	if code != ExitSuccess {
		t.Fatalf("numeric tool_input should not error, got exit %d, output:\n%s", code, out)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected empty output for non-map tool_input, got: %s", out)
	}
}

func TestUserPromptSubmitBlocksTamperedSlashSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_PLUGIN_DATA", filepath.Join(home, ".claude"))
	t.Setenv("CLAUDE_PROJECT_DIR", "")

	setupHookTest(t, home, "cli-test", true)
	policyPath := filepath.Join(home, "policy.json")

	// Slash command should be blocked
	input := `{
		"session_id": "test-session",
		"cwd": "/tmp",
		"hook_event_name": "UserPromptSubmit",
		"prompt": "/cli-test do something"
	}`

	code, out := runHook(t, "user-prompt-submit", []byte(input), "--policy", policyPath)
	if code != ExitSuccess {
		t.Fatalf("exit = %d, output:\n%s", code, out)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}

	if result["decision"] != "block" {
		t.Errorf("decision = %v, want block", result["decision"])
	}

	reason := result["reason"].(string)
	if !strings.Contains(reason, "ST-") {
		t.Errorf("reason should contain ST- code, got: %s", reason)
	}

	// Non-slash prompts should be ignored
	testCases := []string{
		`{"session_id":"test","cwd":"/tmp","hook_event_name":"UserPromptSubmit","prompt":"hello /cli-test"}`,
		`{"session_id":"test","cwd":"/tmp","hook_event_name":"UserPromptSubmit","prompt":"/unknown-skill"}`,
	}

	for _, tc := range testCases {
		code, out := runHook(t, "user-prompt-submit", []byte(tc), "--policy", policyPath)
		if code != ExitSuccess {
			t.Fatalf("exit = %d, output:\n%s", code, out)
		}
		if strings.TrimSpace(out) != "" {
			t.Errorf("expected empty output for %s, got: %s", tc, out)
		}
	}
}

func TestSessionStartSummary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_PLUGIN_DATA", filepath.Join(home, ".claude"))
	t.Setenv("CLAUDE_PROJECT_DIR", "")

	// Create a shared key for both skills
	keyPath, pubPath := newKey(t)
	policyPath := keyPolicy(t, pubPath)
	policyDest := filepath.Join(home, "policy.json")
	policyData, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policyDest, policyData, 0o644); err != nil {
		t.Fatal(err)
	}

	// Create good-skill (signed, not tampered)
	goodRoot := filepath.Join(home, ".claude", "skills", "good-skill")
	if err := os.MkdirAll(filepath.Join(goodRoot, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(goodRoot, "SKILL.md"), []byte("---\nname: good-skill\n---\nGood.\n"), 0o644)
	os.WriteFile(filepath.Join(goodRoot, "scripts/run.sh"), []byte("#!/usr/bin/env bash\necho hi\n"), 0o755)
	code, out := runCLI(t, "sign", goodRoot, "--key", keyPath)
	if code != ExitSuccess {
		t.Fatalf("sign good-skill exit = %d, output:\n%s", code, out)
	}

	// Create bad-skill (signed but tampered)
	badRoot := filepath.Join(home, ".claude", "skills", "bad-skill")
	if err := os.MkdirAll(filepath.Join(badRoot, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(badRoot, "SKILL.md"), []byte("---\nname: bad-skill\n---\nBad.\n"), 0o644)
	os.WriteFile(filepath.Join(badRoot, "scripts/run.sh"), []byte("#!/usr/bin/env bash\necho hi\n"), 0o755)
	code2, out2 := runCLI(t, "sign", badRoot, "--key", keyPath)
	if code2 != ExitSuccess {
		t.Fatalf("sign bad-skill exit = %d, output:\n%s", code2, out2)
	}
	// Tamper after signing
	os.WriteFile(filepath.Join(badRoot, "SKILL.md"), []byte("---\nname: bad-skill\n---\nTampered!\n"), 0o644)

	policyPath = policyDest

	input := `{
		"session_id": "test-session",
		"cwd": "/tmp",
		"hook_event_name": "SessionStart"
	}`

	exitCode, output := runHook(t, "session-start", []byte(input), "--policy", policyPath)
	if exitCode != ExitSuccess {
		t.Fatalf("exit = %d, output:\n%s", exitCode, output)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, output)
	}

	systemMsg, ok := result["systemMessage"].(string)
	if !ok || systemMsg == "" {
		t.Fatalf("missing systemMessage: %v", result)
	}

	// Should mention verified and blocked counts
	if !strings.Contains(systemMsg, "1 skills verified") {
		t.Errorf("systemMessage should mention 1 verified, got: %s", systemMsg)
	}
	if !strings.Contains(systemMsg, "1 blocked") {
		t.Errorf("systemMessage should mention 1 blocked, got: %s", systemMsg)
	}
	if !strings.Contains(systemMsg, "bad-skill") {
		t.Errorf("systemMessage should name blocked skill, got: %s", systemMsg)
	}

	hookOutput := result["hookSpecificOutput"].(map[string]any)
	additionalContext, ok := hookOutput["additionalContext"].(string)
	if !ok || additionalContext == "" {
		t.Fatalf("missing additionalContext: %v", hookOutput)
	}
	if !strings.Contains(additionalContext, "bad-skill") {
		t.Errorf("additionalContext should name blocked skill, got: %s", additionalContext)
	}
}

func TestExitCodeConstants(t *testing.T) {
	// Verify ExitError is 2 (required by the spec)
	if ExitError != 2 {
		t.Errorf("ExitError = %d, must be 2", ExitError)
	}
}
