package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pasteit.toml")
	content := "[default]\ntoken = \"mytoken\"\nurl = \"https://gitlab.com\"\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	p, err := loadConfigFromPath(path, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Token != "mytoken" {
		t.Errorf("token = %q, want %q", p.Token, "mytoken")
	}
	if p.URL != "https://gitlab.com" {
		t.Errorf("url = %q, want %q", p.URL, "https://gitlab.com")
	}
}

func TestLoadConfigMultipleProfiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pasteit.toml")
	content := `
[default]
token = "token1"
url   = "https://gitlab.com"

[github]
token = "token2"
url   = "https://github.com"

[work]
token = "token3"
url   = "https://git.example.com"
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	def, err := loadConfigFromPath(path, "default")
	if err != nil {
		t.Fatalf("unexpected error loading default: %v", err)
	}
	if def.Token != "token1" {
		t.Errorf("default token = %q, want %q", def.Token, "token1")
	}

	gh, err := loadConfigFromPath(path, "github")
	if err != nil {
		t.Fatalf("unexpected error loading github: %v", err)
	}
	if gh.URL != "https://github.com" {
		t.Errorf("github url = %q, want %q", gh.URL, "https://github.com")
	}

	work, err := loadConfigFromPath(path, "work")
	if err != nil {
		t.Fatalf("unexpected error loading work: %v", err)
	}
	if work.URL != "https://git.example.com" {
		t.Errorf("work url = %q, want %q", work.URL, "https://git.example.com")
	}
}

func TestLoadConfigProfileNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pasteit.toml")
	content := "[default]\ntoken = \"tok\"\nurl = \"https://gitlab.com\"\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfigFromPath(path, "nonexistent"); err == nil {
		t.Error("expected error for missing profile, got nil")
	}
}

func TestLoadConfigMissingToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pasteit.toml")
	content := "[default]\nurl = \"https://gitlab.com\"\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfigFromPath(path, "default"); err == nil {
		t.Error("expected error for missing token, got nil")
	}
}

func TestLoadConfigMissingURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pasteit.toml")
	content := "[default]\ntoken = \"mytoken\"\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfigFromPath(path, "default"); err == nil {
		t.Error("expected error for missing url, got nil")
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	if _, err := loadConfigFromPath("/nonexistent/path/pasteit.toml", "default"); err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestIsGitHub(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://github.com", true},
		{"https://gitlab.com", false},
		{"https://git.example.com", false},
		{"https://github.com/user", true},
	}
	for _, c := range cases {
		if got := isGitHub(c.url); got != c.want {
			t.Errorf("isGitHub(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

func TestCreateGitLabSnippetSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/snippets" {
			t.Errorf("path = %s, want /api/v4/snippets", r.URL.Path)
		}
		if r.Header.Get("PRIVATE-TOKEN") != "testtoken" {
			t.Errorf("PRIVATE-TOKEN = %q, want %q", r.Header.Get("PRIVATE-TOKEN"), "testtoken")
		}

		var payload glRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if payload.Visibility != "private" {
			t.Errorf("visibility = %q, want %q", payload.Visibility, "private")
		}
		if len(payload.Files) != 1 || payload.Files[0].FilePath != "hello.txt" {
			t.Errorf("unexpected files: %+v", payload.Files)
		}
		if payload.Files[0].Content != "hello world" {
			t.Errorf("content = %q, want %q", payload.Files[0].Content, "hello world")
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(glResponse{WebURL: "https://gitlab.com/snippets/1"})
	}))
	defer srv.Close()

	p := profile{Token: "testtoken", URL: srv.URL}
	url, err := createSnippet(p, []byte("hello world"), "hello.txt", "my snippet", "private")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://gitlab.com/snippets/1" {
		t.Errorf("url = %q, want %q", url, "https://gitlab.com/snippets/1")
	}
}

func TestCreateGitLabSnippetPublic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload glRequest
		json.NewDecoder(r.Body).Decode(&payload)
		if payload.Visibility != "public" {
			t.Errorf("visibility = %q, want %q", payload.Visibility, "public")
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(glResponse{WebURL: "https://gitlab.com/s/1"})
	}))
	defer srv.Close()

	p := profile{Token: "tok", URL: srv.URL}
	if _, err := createSnippet(p, []byte("data"), "f.txt", "title", "public"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateGitLabSnippetServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := profile{Token: "badtoken", URL: srv.URL}
	if _, err := createSnippet(p, []byte("data"), "f.txt", "title", "private"); err == nil {
		t.Error("expected error for 401 response, got nil")
	}
}

func TestCreateGitHubGistSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gists" {
			t.Errorf("path = %s, want /gists", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer ghtoken" {
			t.Errorf("Authorization = %q, want %q", r.Header.Get("Authorization"), "Bearer ghtoken")
		}

		var payload ghRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if payload.Public {
			t.Error("expected public = false for private gist")
		}
		if _, ok := payload.Files["hello.txt"]; !ok {
			t.Errorf("expected file hello.txt in payload, got %+v", payload.Files)
		}
		if payload.Files["hello.txt"].Content != "hello world" {
			t.Errorf("content = %q, want %q", payload.Files["hello.txt"].Content, "hello world")
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(ghResponse{HTMLURL: "https://gist.github.com/abc123"})
	}))
	defer srv.Close()

	// point the GitHub gist function at the test server by temporarily overriding the URL
	orig := githubAPIURL
	githubAPIURL = srv.URL + "/gists"
	defer func() { githubAPIURL = orig }()

	p := profile{Token: "ghtoken", URL: "https://github.com"}
	url, err := createSnippet(p, []byte("hello world"), "hello.txt", "my gist", "private")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://gist.github.com/abc123" {
		t.Errorf("url = %q, want %q", url, "https://gist.github.com/abc123")
	}
}

func TestCreateGitHubGistPublic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload ghRequest
		json.NewDecoder(r.Body).Decode(&payload)
		if !payload.Public {
			t.Error("expected public = true for public gist")
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(ghResponse{HTMLURL: "https://gist.github.com/xyz"})
	}))
	defer srv.Close()

	orig := githubAPIURL
	githubAPIURL = srv.URL + "/gists"
	defer func() { githubAPIURL = orig }()

	p := profile{Token: "tok", URL: "https://github.com"}
	if _, err := createSnippet(p, []byte("data"), "f.txt", "title", "public"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHTTPClientTimeout(t *testing.T) {
	if httpClient.Timeout != 10*time.Second {
		t.Errorf("httpClient.Timeout = %v, want 10s", httpClient.Timeout)
	}
}
