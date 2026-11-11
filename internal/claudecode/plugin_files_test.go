package claudecode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPluginFiles verifies that the Claude Code plugin files are well-formed
// and reference the correct hook events.
func TestPluginFiles(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	pluginDir := filepath.Join(repoRoot, "adapters", "claude-code")
	pluginJSONPath := filepath.Join(pluginDir, ".claude-plugin", "plugin.json")
	hooksJSONPath := filepath.Join(pluginDir, "hooks", "hooks.json")
	marketplaceJSONPath := filepath.Join(repoRoot, ".claude-plugin", "marketplace.json")

	// Expected hook events that the CLI registers
	validEvents := map[string]bool{
		"session-start":       true,
		"pre-tool-use":        true,
		"user-prompt-submit": true,
	}

	// Test plugin.json
	t.Run("PluginJSON", func(t *testing.T) {
		data, err := os.ReadFile(pluginJSONPath)
		if err != nil {
			t.Fatalf("failed to read plugin.json: %v", err)
		}

		var plugin struct {
			Name        string `json:"name"`
			Version     string `json:"version"`
			Description string `json:"description"`
			Repository  string `json:"repository"`
			Hooks       string `json:"hooks"`
			UserConfig  map[string]interface{} `json:"userConfig"`
		}

		if err := json.Unmarshal(data, &plugin); err != nil {
			t.Fatalf("failed to parse plugin.json: %v", err)
		}

		if plugin.Name != "skill-trust" {
			t.Errorf("expected name 'skill-trust', got %q", plugin.Name)
		}
		if plugin.Version != "0.2.0" {
			t.Errorf("expected version '0.2.0', got %q", plugin.Version)
		}
		if plugin.Hooks != "./hooks/hooks.json" {
			t.Errorf("expected hooks path './hooks/hooks.json', got %q", plugin.Hooks)
		}
		if plugin.Repository != "https://github.com/redhat-et/skill-trust" {
			t.Errorf("expected repository 'https://github.com/redhat-et/skill-trust', got %q", plugin.Repository)
		}
	})

	// Test hooks.json
	t.Run("HooksJSON", func(t *testing.T) {
		data, err := os.ReadFile(hooksJSONPath)
		if err != nil {
			t.Fatalf("failed to read hooks.json: %v", err)
		}

		var hooks struct {
			Hooks struct {
				SessionStart []struct {
					Matcher string `json:"matcher"`
					Hooks   []struct {
						Type    string `json:"type"`
						Command string `json:"command"`
					} `json:"hooks"`
				} `json:"SessionStart"`
				PreToolUse []struct {
					Matcher string `json:"matcher"`
					Hooks   []struct {
						Type    string `json:"type"`
						Command string `json:"command"`
					} `json:"hooks"`
				} `json:"PreToolUse"`
				UserPromptSubmit []struct {
					Hooks []struct {
						Type    string `json:"type"`
						Command string `json:"command"`
					} `json:"hooks"`
				} `json:"UserPromptSubmit"`
			} `json:"hooks"`
		}

		if err := json.Unmarshal(data, &hooks); err != nil {
			t.Fatalf("failed to parse hooks.json: %v", err)
		}

		// Collect all hook commands
		var commands []string
		for _, group := range hooks.Hooks.SessionStart {
			for _, h := range group.Hooks {
				commands = append(commands, h.Command)
			}
		}
		for _, group := range hooks.Hooks.PreToolUse {
			for _, h := range group.Hooks {
				commands = append(commands, h.Command)
			}
		}
		for _, group := range hooks.Hooks.UserPromptSubmit {
			for _, h := range group.Hooks {
				commands = append(commands, h.Command)
			}
		}

		// Verify each command references a valid event
		for _, cmd := range commands {
			if !strings.Contains(cmd, "hook claude-code ") {
				t.Errorf("hook command does not contain 'hook claude-code ': %q", cmd)
				continue
			}

			// Extract the event name from the command
			parts := strings.Split(cmd, "hook claude-code ")
			if len(parts) < 2 {
				t.Errorf("malformed hook command: %q", cmd)
				continue
			}
			eventPart := strings.TrimSpace(parts[1])
			// The event name should be the last token in the command
			tokens := strings.Fields(eventPart)
			if len(tokens) == 0 {
				t.Errorf("no event found in hook command: %q", cmd)
				continue
			}
			event := tokens[len(tokens)-1]

			if !validEvents[event] {
				t.Errorf("hook command references unknown event %q (command: %q)", event, cmd)
			}
		}
	})

	// Test marketplace.json
	t.Run("MarketplaceJSON", func(t *testing.T) {
		data, err := os.ReadFile(marketplaceJSONPath)
		if err != nil {
			t.Fatalf("failed to read marketplace.json: %v", err)
		}

		var marketplace struct {
			Plugins []struct {
				Name   string `json:"name"`
				Source string `json:"source"`
			} `json:"plugins"`
		}

		if err := json.Unmarshal(data, &marketplace); err != nil {
			t.Fatalf("failed to parse marketplace.json: %v", err)
		}

		found := false
		for _, p := range marketplace.Plugins {
			if p.Name == "skill-trust" {
				found = true
				source := filepath.Join(repoRoot, p.Source)
				pluginJSON := filepath.Join(source, ".claude-plugin", "plugin.json")
				if _, err := os.Stat(pluginJSON); err != nil {
					t.Errorf("marketplace entry source %q does not contain .claude-plugin/plugin.json: %v", p.Source, err)
				}
				break
			}
		}

		if !found {
			t.Error("marketplace.json does not contain a 'skill-trust' plugin entry")
		}
	})
}
