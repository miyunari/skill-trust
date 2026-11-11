package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/redhat-et/skill-trust/internal/claudecode"
	"github.com/redhat-et/skill-trust/pkg/preflight"
	"github.com/redhat-et/skill-trust/pkg/verify"
	"github.com/spf13/cobra"
)

var (
	hookPolicyPath string
	hookStrict     bool
)

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Claude Code hook integration",
	Long: `Hook commands integrate skill-trust into Claude Code's hook system.
These commands are not meant to be invoked directly by users.`,
}

var hookClaudeCodeCmd = &cobra.Command{
	Use:   "claude-code",
	Short: "Claude Code hook adapter",
	Long: `Claude Code hook adapter commands.
Reads JSON input from stdin and writes JSON output to stdout.`,
}

var sessionStartCmd = &cobra.Command{
	Use:   "session-start",
	Short: "Handle SessionStart hook event",
	Run: func(cmd *cobra.Command, args []string) {
		input, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "skill-trust: reading stdin: %v\n", err)
			exitCode = ExitError
			return
		}

		var hookInput claudecode.HookInput
		if err := json.Unmarshal(input, &hookInput); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "skill-trust: invalid JSON input: %v\n", err)
			exitCode = ExitError
			return
		}

		cfg := buildConfig(hookInput.Cwd)
		result := claudecode.SessionStart(cfg, hookInput)

		if result != nil {
			cmd.OutOrStdout().Write(result)
			cmd.OutOrStdout().Write([]byte("\n"))
		}
	},
}

var preToolUseCmd = &cobra.Command{
	Use:   "pre-tool-use",
	Short: "Handle PreToolUse hook event",
	Run: func(cmd *cobra.Command, args []string) {
		input, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "skill-trust: reading stdin: %v\n", err)
			exitCode = ExitError
			return
		}

		var hookInput claudecode.HookInput
		if err := json.Unmarshal(input, &hookInput); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "skill-trust: invalid JSON input: %v\n", err)
			exitCode = ExitError
			return
		}

		cfg := buildConfig(hookInput.Cwd)
		result := claudecode.PreToolUse(cfg, hookInput)

		if result != nil {
			cmd.OutOrStdout().Write(result)
			cmd.OutOrStdout().Write([]byte("\n"))
		}
	},
}

var userPromptSubmitCmd = &cobra.Command{
	Use:   "user-prompt-submit",
	Short: "Handle UserPromptSubmit hook event",
	Run: func(cmd *cobra.Command, args []string) {
		input, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "skill-trust: reading stdin: %v\n", err)
			exitCode = ExitError
			return
		}

		var hookInput claudecode.HookInput
		if err := json.Unmarshal(input, &hookInput); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "skill-trust: invalid JSON input: %v\n", err)
			exitCode = ExitError
			return
		}

		cfg := buildConfig(hookInput.Cwd)
		result := claudecode.UserPromptSubmit(cfg, hookInput)

		if result != nil {
			cmd.OutOrStdout().Write(result)
			cmd.OutOrStdout().Write([]byte("\n"))
		}
	},
}

func init() {
	// Add hook command tree
	rootCmd.AddCommand(hookCmd)
	hookCmd.AddCommand(hookClaudeCodeCmd)
	hookClaudeCodeCmd.AddCommand(sessionStartCmd)
	hookClaudeCodeCmd.AddCommand(preToolUseCmd)
	hookClaudeCodeCmd.AddCommand(userPromptSubmitCmd)

	// Add flags to all hook commands
	for _, cmd := range []*cobra.Command{sessionStartCmd, preToolUseCmd, userPromptSubmitCmd} {
		cmd.Flags().StringVar(&hookPolicyPath, "policy", "", "path to organization policy file")
		cmd.Flags().BoolVar(&hookStrict, "strict", getStrictDefault(), "strict mode (deny unsigned/unverified skills)")
	}
}

func getStrictDefault() bool {
	env := os.Getenv("SKILL_TRUST_STRICT")
	if env == "" {
		return true
	}
	strict, err := strconv.ParseBool(env)
	if err != nil {
		return true // Invalid value defaults to strict
	}
	return strict
}

func buildConfig(cwd string) claudecode.Config {
	// Resolve policy path - make absolute if relative
	policyPath := hookPolicyPath
	if policyPath != "" && !filepath.IsAbs(policyPath) {
		if abs, err := filepath.Abs(policyPath); err == nil {
			policyPath = abs
		}
	}

	return claudecode.Config{
		Policy:     policyPath,
		Strict:     hookStrict,
		StateDir:   claudecode.StateDir(),
		CLIVersion: Version().Version,
		Locations:  claudecode.DefaultLocations(cwd),
		NewVerifier: func(path string) (preflight.Verifier, error) {
			if path == "" {
				return nil, fmt.Errorf("policy path not configured")
			}
			p, err := verify.LoadPolicy(path)
			if err != nil {
				return nil, err
			}
			v, err := verify.New(p)
			if err != nil {
				return nil, err
			}
			return v.PreflightVerifier(), nil
		},
	}
}
