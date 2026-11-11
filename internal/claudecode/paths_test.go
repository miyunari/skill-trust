package claudecode

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizePath(t *testing.T) {
	// Test case: collapse symlinked ancestor to canonical path
	t.Run("CollapseSymlinkedAncestor", func(t *testing.T) {
		tmpReal := t.TempDir()
		canonical, err := filepath.EvalSymlinks(tmpReal)
		if err != nil {
			t.Fatal(err)
		}
		skillDir := filepath.Join(tmpReal, "demo")
		if err := os.Mkdir(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}

		tmpAlias := t.TempDir()
		alias := filepath.Join(tmpAlias, "alias")
		if err := os.Symlink(tmpReal, alias); err != nil {
			t.Fatal(err)
		}

		viaAlias := NormalizePath(filepath.Join(alias, "demo", "scripts", "run.sh"))
		if !strings.HasPrefix(viaAlias, canonical) {
			t.Errorf("expected path to start with %s, got %s", canonical, viaAlias)
		}
		if strings.HasPrefix(viaAlias, alias) {
			t.Errorf("expected path not to start with alias %s, got %s", alias, viaAlias)
		}
	})

	// Test case: resolve deepest existing ancestor for not-yet-existing path
	t.Run("DeepestExistingAncestor", func(t *testing.T) {
		tmpReal := t.TempDir()
		canonical, err := filepath.EvalSymlinks(tmpReal)
		if err != nil {
			t.Fatal(err)
		}

		tmpAlias := t.TempDir()
		alias := filepath.Join(tmpAlias, "alias")
		if err := os.Symlink(tmpReal, alias); err != nil {
			t.Fatal(err)
		}

		viaAlias := NormalizePath(filepath.Join(alias, "scripts", "not-yet.sh"))
		if !strings.HasPrefix(viaAlias, canonical) {
			t.Errorf("expected path to start with %s, got %s", canonical, viaAlias)
		}
	})

	// Test case: pass through paths that do not exist at all
	t.Run("NonexistentPath", func(t *testing.T) {
		p := "/nonexistent/skill/scripts/run.sh"
		got := NormalizePath(p)
		if got != p {
			t.Errorf("expected %s, got %s", p, got)
		}
	})
}

func TestCommandPaths(t *testing.T) {
	cwd := "/proj"

	tests := []struct {
		name     string
		command  string
		cwd      string
		expected []string
	}{
		{
			name:     "ExtractPathLikeTokens",
			command:  "bash /proj/scripts/run.sh",
			cwd:      cwd,
			expected: []string{"/proj/scripts/run.sh"},
		},
		{
			name:     "ExtractPythonPath",
			command:  "python .opencode/skill/demo/scripts/util.py",
			cwd:      cwd,
			expected: []string{"/proj/.opencode/skill/demo/scripts/util.py"},
		},
		{
			name:     "RelativeTokensAgainstCwd",
			command:  "./scripts/run.sh",
			cwd:      "/proj/skill",
			expected: []string{"/proj/skill/scripts/run.sh"},
		},
		{
			name:     "DoubleQuotedPaths",
			command:  `bash "/proj/my skill/run.sh"`,
			cwd:      cwd,
			expected: []string{"/proj/my skill/run.sh"},
		},
		{
			name:     "SingleQuotedPaths",
			command:  `bash '/proj/my skill/run.sh'`,
			cwd:      cwd,
			expected: []string{"/proj/my skill/run.sh"},
		},
		{
			name:     "IgnoreFlagsURLsRedirectsVariables",
			command:  "curl https://example.com/x.py -o out.py > log 2>&1 $SCRIPT",
			cwd:      cwd,
			expected: []string{},
		},
		{
			name:     "IgnoreTokensWithoutPathSeparator",
			command:  "echo hello python util.py",
			cwd:      cwd,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CommandPaths(tt.command, tt.cwd)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("CommandPaths(%q, %q) = %v, want %v", tt.command, tt.cwd, got, tt.expected)
			}
		})
	}
}

func TestCandidatePaths(t *testing.T) {
	t.Run("PathFieldsAndCommandFields", func(t *testing.T) {
		input := map[string]any{
			"path":    "src/a.ts",
			"command": "bash scripts/run.sh",
			"timeout": 5,
		}
		got := CandidatePaths(input, "/proj")
		// Should contain these paths (order doesn't matter, cwd always included)
		hasATS := false
		hasRunSh := false
		hasProj := false
		for _, p := range got {
			if p == "/proj/src/a.ts" {
				hasATS = true
			}
			if p == "/proj/scripts/run.sh" {
				hasRunSh = true
			}
			if p == "/proj" {
				hasProj = true
			}
		}
		if !hasATS || !hasRunSh || !hasProj {
			t.Errorf("got %v, missing expected paths", got)
		}
	})

	t.Run("CommonPathKeySpellings", func(t *testing.T) {
		input := map[string]any{
			"filePath":  "a",
			"workdir":   "/w",
			"directory": "d",
		}
		got := CandidatePaths(input, "/proj")
		// Should be normalized and deduplicated
		hasA := false
		hasW := false
		hasD := false
		for _, p := range got {
			if p == "/proj/a" {
				hasA = true
			}
			if p == "/w" {
				hasW = true
			}
			if p == "/proj/d" {
				hasD = true
			}
		}
		if !hasA || !hasW || !hasD {
			t.Errorf("got %v, missing expected paths", got)
		}
	})

	t.Run("IgnoreNonStringAndEmptyValues", func(t *testing.T) {
		input := map[string]any{
			"path":    3,
			"command": nil,
			"other":   "",
		}
		got := CandidatePaths(input, "/proj")
		if len(got) != 1 || got[0] != "/proj" {
			t.Errorf("got %v, want [/proj]", got)
		}
	})

	t.Run("NestedInput", func(t *testing.T) {
		input := map[string]any{
			"outer": map[string]any{
				"file_path": "nested/file.txt",
			},
			"arr": []any{
				map[string]any{"command": "bash scripts/x.sh"},
			},
		}
		got := CandidatePaths(input, "/root")
		hasNested := false
		hasScript := false
		for _, p := range got {
			if p == "/root/nested/file.txt" {
				hasNested = true
			}
			if p == "/root/scripts/x.sh" {
				hasScript = true
			}
		}
		if !hasNested || !hasScript {
			t.Errorf("got %v, missing nested paths", got)
		}
	})

	t.Run("NonexistentFileInRoot", func(t *testing.T) {
		root := t.TempDir()
		input := map[string]any{
			"file_path": filepath.Join(root, "scripts/new.sh"),
		}
		got := CandidatePaths(input, root)
		// Should normalize even if it doesn't exist
		if len(got) < 2 {
			t.Errorf("got %v, expected at least root and file path", got)
		}
	})

	t.Run("NonObjectInput", func(t *testing.T) {
		got := CandidatePaths("str", "/cwd")
		if len(got) != 1 || got[0] != "/cwd" {
			t.Errorf("got %v, want [/cwd]", got)
		}

		got = CandidatePaths(nil, "/cwd")
		if len(got) != 1 || got[0] != "/cwd" {
			t.Errorf("got %v, want [/cwd]", got)
		}
	})
}
