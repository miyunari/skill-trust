package claudecode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Record is one verification record for a skill root.
type Record struct {
	Allowed    bool      `json:"allowed"`
	Digest     string    `json:"digest"`
	VerifiedAt time.Time `json:"verifiedAt"`
}

// State maps skill roots to their verification records.
type State map[string]Record

// StateDir returns the default state directory.
func StateDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "skill-trust")
}

// sanitizeSessionID cleans a session ID to prevent path traversal.
var sessionIDRegex = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func sanitizeSessionID(id string) string {
	if id == "" {
		return "unknown"
	}
	// Replace invalid characters with empty string
	clean := sessionIDRegex.ReplaceAllString(id, "")
	if clean == "" {
		return "unknown"
	}
	return clean
}

// LoadState reads the verification state for a session. Returns an empty
// state on missing or corrupt files.
func LoadState(dir, sessionID string) State {
	clean := sanitizeSessionID(sessionID)
	stateFile := filepath.Join(dir, "state-"+clean+".json")

	data, err := os.ReadFile(stateFile)
	if err != nil {
		return State{}
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}
	}

	return state
}

// SaveState persists the verification state for a session. Creates the
// directory if needed, writes atomically via temp+rename.
func SaveState(dir, sessionID string, s State) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	clean := sanitizeSessionID(sessionID)
	stateFile := filepath.Join(dir, "state-"+clean+".json")

	data, err := json.Marshal(s)
	if err != nil {
		return err
	}

	// Atomic write: temp file + rename
	tmpFile := stateFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0o600); err != nil {
		return err
	}

	return os.Rename(tmpFile, stateFile)
}

// PruneStates removes state files older than maxAge.
func PruneStates(dir string, maxAge time.Duration, now time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, e := range entries {
		// Only consider state-*.json files
		name := e.Name()
		if !strings.HasPrefix(name, "state-") || !strings.HasSuffix(name, ".json") {
			continue
		}

		info, err := e.Info()
		if err != nil {
			continue
		}

		age := now.Sub(info.ModTime())
		if age > maxAge {
			os.Remove(filepath.Join(dir, name))
		}
	}
}
