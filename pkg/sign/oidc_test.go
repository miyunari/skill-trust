package sign

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestGitHubOIDCTokenMissingEnv(t *testing.T) {
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")

	if _, err := GitHubOIDCToken(t.Context(), "sigstore"); err == nil {
		t.Fatal("expected an error without runtime variables")
	}
}

func TestGitHubOIDCTokenFetch(t *testing.T) {
	var gotPath, gotQuery, gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"value": "test-token"})
	}))
	defer ts.Close()

	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", ts.URL)
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "runtime-secret")

	token, err := GitHubOIDCToken(t.Context(), "sigstore")
	if err != nil {
		t.Fatalf("GitHubOIDCToken: %v", err)
	}
	if token != "test-token" {
		t.Errorf("token = %q", token)
	}
	if gotAuth != "Bearer runtime-secret" {
		t.Errorf("authorization = %q", gotAuth)
	}
	q, err := url.ParseQuery(gotQuery)
	if err != nil {
		t.Fatal(err)
	}
	if q.Get("audience") != "sigstore" {
		t.Errorf("audience = %q", q.Get("audience"))
	}
	_ = gotPath
}

func TestGitHubOIDCTokenErrorStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "denied", http.StatusForbidden)
	}))
	defer ts.Close()

	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", ts.URL)
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "runtime-secret")

	_, err := GitHubOIDCToken(t.Context(), "sigstore")
	if err == nil {
		t.Fatal("expected an error for a failing endpoint")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention the status: %v", err)
	}
}

// TestGitHubOIDCTokenOversizedResponse covers the 1 MiB response cap: a
// body larger than the cap is reported as oversized rather than falling
// through to JSON parsing (where truncation would surface as a confusing
// syntax error).
func TestGitHubOIDCTokenOversizedResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(bytes.Repeat([]byte("a"), (1<<20)+1))
	}))
	defer ts.Close()

	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", ts.URL)
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "runtime-secret")

	_, err := GitHubOIDCToken(t.Context(), "sigstore")
	if err == nil {
		t.Fatal("expected an error for an oversized response")
	}
	if !errors.Is(err, ErrOIDCUnavailable) {
		t.Errorf("err = %v, want ErrOIDCUnavailable", err)
	}
	if !strings.Contains(err.Error(), "exceeds 1 MiB") {
		t.Errorf("error should mention the size cap: %v", err)
	}
}

func TestGitHubOIDCTokenInvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer ts.Close()

	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", ts.URL)
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "runtime-secret")

	if _, err := GitHubOIDCToken(t.Context(), "sigstore"); err == nil {
		t.Fatal("expected an error for a non-JSON response")
	}
}
