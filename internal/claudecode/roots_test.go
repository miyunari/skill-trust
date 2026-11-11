package claudecode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultLocations(t *testing.T) {
	cwd := "/proj/apps/web"
	loc := DefaultLocations(cwd)

	if loc.Home == "" {
		t.Error("Home should not be empty")
	}
	if loc.Cwd != cwd {
		t.Errorf("Cwd = %s, want %s", loc.Cwd, cwd)
	}
	if loc.PluginsDir == "" {
		t.Error("PluginsDir should not be empty")
	}
}

func TestDiscoverLocations(t *testing.T) {
	tmp := t.TempDir()

	// Create home/.claude/skills/demo
	homeSkills := filepath.Join(tmp, "home", ".claude", "skills")
	demoDir := filepath.Join(homeSkills, "demo")
	os.MkdirAll(demoDir, 0o755)
	os.WriteFile(filepath.Join(demoDir, "SKILL.md"), []byte("demo skill"), 0o644)

	// Create project/.claude/skills/proj-skill
	projectDir := filepath.Join(tmp, "project")
	projectSkills := filepath.Join(projectDir, ".claude", "skills", "proj-skill")
	os.MkdirAll(projectSkills, 0o755)
	os.WriteFile(filepath.Join(projectSkills, "SKILL.md"), []byte("project skill"), 0o644)

	// Create repo/.git
	repoDir := filepath.Join(tmp, "repo")
	os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755)

	// Create repo/apps/.claude/skills/app-skill
	repoAppSkills := filepath.Join(repoDir, "apps", ".claude", "skills", "app-skill")
	os.MkdirAll(repoAppSkills, 0o755)
	os.WriteFile(filepath.Join(repoAppSkills, "SKILL.md"), []byte("app skill"), 0o644)

	// Create plugin via installed_plugins.json
	pluginsDir := filepath.Join(tmp, "plugins")
	cacheDir := filepath.Join(pluginsDir, "cache", "m", "p", "1.0", "skills", "s")
	os.MkdirAll(cacheDir, 0o755)
	os.WriteFile(filepath.Join(cacheDir, "SKILL.md"), []byte("plugin skill"), 0o644)

	pluginsJSON := filepath.Join(pluginsDir, "installed_plugins.json")
	installData := map[string]any{
		"version": 2,
		"plugins": map[string]any{
			"p@m": []map[string]any{
				{"installPath": filepath.Join(pluginsDir, "cache", "m", "p", "1.0")},
			},
		},
	}
	data, _ := json.Marshal(installData)
	os.WriteFile(pluginsJSON, data, 0o644)

	// Create a directory without SKILL.md (should be skipped)
	noSkillDir := filepath.Join(homeSkills, "no-skill")
	os.MkdirAll(noSkillDir, 0o755)

	loc := Locations{
		Home:       filepath.Join(tmp, "home"),
		Cwd:        filepath.Join(repoDir, "apps", "web"),
		ProjectDir: projectDir,
		PluginsDir: pluginsDir,
		Enterprise: []string{},
	}

	skills := loc.Discover()

	// Should find: demo, proj-skill, app-skill, p:s
	foundDemo := false
	foundProj := false
	foundApp := false
	foundPlugin := false

	for _, s := range skills {
		if s.Name == "demo" {
			foundDemo = true
		}
		if s.Name == "proj-skill" {
			foundProj = true
		}
		if s.Name == "app-skill" {
			foundApp = true
		}
		if s.Name == "p:s" {
			foundPlugin = true
		}
	}

	if !foundDemo {
		t.Error("Expected to find demo skill")
	}
	if !foundProj {
		t.Error("Expected to find proj-skill")
	}
	if !foundApp {
		t.Error("Expected to find app-skill")
	}
	if !foundPlugin {
		t.Error("Expected to find plugin skill p:s")
	}
}

func TestRootOfIgnoresStraySkillMD(t *testing.T) {
	tmp := t.TempDir()

	// Create repo/skills/demo/SKILL.md without .claude parent
	skillDir := filepath.Join(tmp, "repo", "skills", "demo")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("stray"), 0o644)

	loc := Locations{
		Home: tmp,
		Cwd:  tmp,
	}
	skills := loc.Discover()

	path := filepath.Join(skillDir, "SKILL.md")
	root, ok := loc.RootOf(path, skills)
	if ok {
		t.Errorf("RootOf(%q) = %q, want false (no .claude parent)", path, root)
	}
}

func TestRootOfNestedClaudeSkills(t *testing.T) {
	tmp := t.TempDir()

	// Create repo/apps/web/.claude/skills/deploy
	deployDir := filepath.Join(tmp, "repo", "apps", "web", ".claude", "skills", "deploy")
	os.MkdirAll(deployDir, 0o755)
	os.WriteFile(filepath.Join(deployDir, "SKILL.md"), []byte("deploy"), 0o644)

	scriptPath := filepath.Join(deployDir, "scripts", "x.sh")

	loc := Locations{
		Home: tmp,
		Cwd:  filepath.Join(tmp, "repo", "apps", "web"),
	}
	skills := loc.Discover()

	root, ok := loc.RootOf(scriptPath, skills)
	if !ok {
		t.Fatalf("RootOf(%q) returned false", scriptPath)
	}

	expectedRoot := NormalizePath(deployDir)
	if root != expectedRoot {
		t.Errorf("RootOf(%q) = %q, want %q", scriptPath, root, expectedRoot)
	}
}

func TestRootOfSymlinkedInstall(t *testing.T) {
	tmp := t.TempDir()

	// Create src/foo
	srcDir := filepath.Join(tmp, "src", "foo")
	os.MkdirAll(srcDir, 0o755)
	os.WriteFile(filepath.Join(srcDir, "SKILL.md"), []byte("foo"), 0o644)

	// Create home/.claude/skills/foo as symlink to src/foo
	homeSkills := filepath.Join(tmp, "home", ".claude", "skills")
	if err := os.MkdirAll(homeSkills, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(homeSkills, "foo")
	if err := os.Symlink(srcDir, symlinkPath); err != nil {
		t.Fatal(err)
	}

	// Verify symlink was created
	if _, err := os.Stat(symlinkPath); err != nil {
		t.Fatalf("symlink not accessible: %v", err)
	}

	// Check what's in homeSkills
	entries, err := os.ReadDir(homeSkills)
	if err != nil {
		t.Fatalf("cannot read homeSkills: %v", err)
	}
	t.Logf("homeSkills has %d entries", len(entries))
	for _, e := range entries {
		t.Logf("  Entry: %s, IsDir=%v", e.Name(), e.IsDir())
	}

	loc := Locations{
		Home: filepath.Join(tmp, "home"),
		Cwd:  tmp,
	}
	skills := loc.Discover()

	// Both paths should map to the realpath of src/foo
	srcRealpath := NormalizePath(srcDir)

	t.Logf("Discovered %d skills", len(skills))
	for _, s := range skills {
		t.Logf("  Skill: %s at %s", s.Name, s.Root)
	}
	t.Logf("srcDir = %s", srcDir)
	t.Logf("srcRealpath = %s", srcRealpath)

	root1, ok1 := loc.RootOf(filepath.Join(homeSkills, "foo", "SKILL.md"), skills)
	if !ok1 {
		t.Fatalf("RootOf via symlink returned false")
	}
	if root1 != srcRealpath {
		t.Errorf("RootOf via symlink = %q, want %q", root1, srcRealpath)
	}

	skillMDPath := filepath.Join(srcDir, "SKILL.md")
	t.Logf("Calling RootOf with %s", skillMDPath)
	root2, ok2 := loc.RootOf(skillMDPath, skills)
	if !ok2 {
		t.Fatalf("RootOf via real path returned false, discovered skills: %+v", skills)
	}
	if root2 != srcRealpath {
		t.Errorf("RootOf via real path = %q, want %q", root2, srcRealpath)
	}
}

func TestResolveSkillName(t *testing.T) {
	skills := []Skill{
		{Name: "deploy", Root: "/proj/.claude/skills/deploy"},
		{Name: "p:s", Root: "/plugins/p/skills/s"},
		{Name: "apps/web:deploy", Root: "/proj/apps/web/.claude/skills/deploy"},
	}

	tests := []struct {
		name     string
		input    string
		wantRoot string
		wantOk   bool
	}{
		{"ExactMatch", "deploy", "/proj/.claude/skills/deploy", true},
		{"PluginMatch", "p:s", "/plugins/p/skills/s", true},
		{"NestedMatch", "apps/web:deploy", "/proj/apps/web/.claude/skills/deploy", true},
		{"UnknownSkill", "unknown", "", false},
		{"AmbiguousSuffix", "deploy", "/proj/.claude/skills/deploy", true}, // exact match wins
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRoot, gotOk := ResolveSkillName(tt.input, skills)
			if gotOk != tt.wantOk {
				t.Errorf("ResolveSkillName(%q) ok = %v, want %v", tt.input, gotOk, tt.wantOk)
			}
			if gotRoot != tt.wantRoot {
				t.Errorf("ResolveSkillName(%q) root = %q, want %q", tt.input, gotRoot, tt.wantRoot)
			}
		})
	}

	// Test suffix match with exactly one candidate
	skillsSuffixOnly := []Skill{
		{Name: "apps/web:deploy", Root: "/proj/apps/web/.claude/skills/deploy"},
	}
	root, ok := ResolveSkillName("deploy", skillsSuffixOnly)
	if !ok || root != "/proj/apps/web/.claude/skills/deploy" {
		t.Errorf("ResolveSkillName(deploy) with suffix-only = (%q, %v), want match", root, ok)
	}
}
