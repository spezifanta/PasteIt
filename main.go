package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/atotto/clipboard"
	"github.com/spf13/pflag"
	"golang.org/x/term"
)

const version = "1.0.0"

var httpClient = &http.Client{Timeout: 10 * time.Second}
var githubAPIURL = "https://api.github.com/gists"

type profile struct {
	Token string `toml:"token"`
	URL   string `toml:"url"`
}

type configFile map[string]profile

// GitLab types
type glFile struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

type glRequest struct {
	Title      string   `json:"title"`
	Visibility string   `json:"visibility"`
	Files      []glFile `json:"files"`
}

type glResponse struct {
	WebURL string `json:"web_url"`
}

// GitHub types
type ghFileContent struct {
	Content string `json:"content"`
}

type ghRequest struct {
	Description string                   `json:"description"`
	Public      bool                     `json:"public"`
	Files       map[string]ghFileContent `json:"files"`
}

type ghResponse struct {
	HTMLURL string `json:"html_url"`
}

func colorize(s, code string) string {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}

func isGitHub(url string) bool {
	return strings.Contains(url, "github.com")
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "pasteit.toml"), nil
}

func runWizard(path, profileName string) (profile, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return profile{}, fmt.Errorf("cannot open terminal for setup: %w", err)
	}
	defer tty.Close()

	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		fmt.Fprintln(tty, "pasteit: no config found — running first-time setup")
	} else {
		fmt.Fprintf(tty, "pasteit: profile %q not found — adding it now\n", profileName)
	}
	fmt.Fprintln(tty, "Config will be saved to:", path)
	fmt.Fprintln(tty)

	reader := bufio.NewReader(tty)

	fmt.Fprint(tty, "URL (e.g. https://gitlab.com or https://github.com): ")
	rawURL, err := reader.ReadString('\n')
	if err != nil {
		return profile{}, fmt.Errorf("failed to read URL: %w", err)
	}
	url := strings.TrimSpace(rawURL)
	url = strings.TrimRight(url, "/")
	if url == "" {
		return profile{}, fmt.Errorf("URL cannot be empty")
	}

	fmt.Fprint(tty, "Personal access token (input hidden): ")
	tokenBytes, err := term.ReadPassword(int(tty.Fd()))
	fmt.Fprintln(tty)
	if err != nil {
		return profile{}, fmt.Errorf("failed to read token: %w", err)
	}
	token := strings.TrimSpace(string(tokenBytes))
	if token == "" {
		return profile{}, fmt.Errorf("token cannot be empty")
	}

	p := profile{Token: token, URL: url}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return profile{}, fmt.Errorf("failed to write config: %w", err)
	}
	defer f.Close()
	entry := fmt.Sprintf("\n[%s]\ntoken = %q\nurl   = %q\n", profileName, p.Token, p.URL)
	if _, err := f.WriteString(entry); err != nil {
		return profile{}, fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Fprintf(tty, "Profile %q saved to %s\n\n", profileName, path)
	return p, nil
}

func parseConfigFile(path string) (configFile, error) {
	var cfg configFile
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("cannot read config %s: %w", path, err)
	}
	return cfg, nil
}

func loadConfigFromPath(path, profileName string) (profile, error) {
	cfg, err := parseConfigFile(path)
	if err != nil {
		return profile{}, err
	}
	p, ok := cfg[profileName]
	if !ok {
		return profile{}, fmt.Errorf("profile %q not found in %s", profileName, path)
	}
	if p.Token == "" {
		return profile{}, fmt.Errorf("token missing in profile %q", profileName)
	}
	if p.URL == "" {
		return profile{}, fmt.Errorf("url missing in profile %q", profileName)
	}
	return p, nil
}

func loadConfig(profileName string) (profile, error) {
	path, err := configPath()
	if err != nil {
		return profile{}, err
	}

	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		return runWizard(path, profileName)
	}

	cfg, err := parseConfigFile(path)
	if err != nil {
		return profile{}, err
	}
	if _, ok := cfg[profileName]; !ok {
		return runWizard(path, profileName)
	}

	p := cfg[profileName]
	if p.Token == "" {
		return profile{}, fmt.Errorf("token missing in profile %q", profileName)
	}
	if p.URL == "" {
		return profile{}, fmt.Errorf("url missing in profile %q", profileName)
	}
	return p, nil
}

func stdinIsPipe() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) == 0
}

func printHelp() {
	fmt.Printf(`PasteIt %s - Create GitLab Snippets and GitHub Gists via CLI

Usage:
  pasteit <file>                    upload a file as a snippet
  cat file | pasteit                pipe content as a snippet
  pasteit <file> -f name.py         upload file, override snippet filename
  cat file | pasteit -f name        pipe content with a specific filename
  pasteit <file> -P work            use the "work" profile

Options:
  -f, --file        filename shown in the snippet / gist
  -t, --title       snippet title or gist description (default: "pasteit snippet")
  -p, --public      make snippet/gist public (default: private)
  -P, --profile     config profile to use (default: "default")
      --version     print version and exit

Config: ~/.config/pasteit.toml
  [default]
  token = "your-gitlab-token"
  url   = "https://gitlab.com"

  [github]
  token = "your-github-token"
  url   = "https://github.com"

  [work]
  token = "your-work-token"
  url   = "https://git.example.com"
`, version)
}

func main() {
	var (
		snippetName = pflag.StringP("file", "f", "", "filename shown in the snippet / gist")
		title       = pflag.StringP("title", "t", "PasteIt snippet", "snippet title or gist description")
		public      = pflag.BoolP("public", "p", false, "make snippet/gist public (default: private)")
		profileName = pflag.StringP("profile", "P", "default", "config profile to use")
		ver         = pflag.Bool("version", false, "print version and exit")
	)
	pflag.Usage = printHelp
	pflag.Parse()

	if *ver {
		fmt.Println("PasteIt", version)
		os.Exit(0)
	}

	isPipe := stdinIsPipe()
	inputArg := pflag.Arg(0)

	if inputArg == "" && !isPipe {
		printHelp()
		os.Exit(0)
	}

	p, err := loadConfig(*profileName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pasteit:", err)
		os.Exit(1)
	}

	var content []byte
	switch {
	case inputArg != "":
		var err error
		content, err = os.ReadFile(inputArg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "pasteit: cannot read file:", err)
			os.Exit(1)
		}
		if *snippetName == "" {
			*snippetName = filepath.Base(inputArg)
		}
	case isPipe:
		var err error
		content, err = io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "pasteit: failed to read stdin:", err)
			os.Exit(1)
		}
		if *snippetName == "" {
			*snippetName = "snippet.txt"
		}
	}

	if len(bytes.TrimSpace(content)) == 0 {
		fmt.Fprintln(os.Stderr, "pasteit: no input provided")
		os.Exit(1)
	}

	visibility := "private"
	if *public {
		visibility = "public"
	}

	url, err := createSnippet(p, content, *snippetName, *title, visibility)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pasteit:", err)
		os.Exit(1)
	}

	kind := "snippet"
	if isGitHub(p.URL) {
		kind = "gist"
	}

	label := visibility
	if visibility == "public" {
		label = colorize("public", "1;33")
	}

	if err := clipboard.WriteAll(url); err != nil {
		fmt.Fprintf(os.Stderr, "pasteit: could not copy to clipboard: %v\n", err)
		fmt.Printf("Created %s %s:\n%s\n", label, kind, url)
	} else {
		fmt.Printf("Created %s %s. URL copied to clipboard:\n%s\n", label, kind, url)
	}
}

func createSnippet(p profile, content []byte, name, title, visibility string) (string, error) {
	if isGitHub(p.URL) {
		return createGitHubGist(p, content, name, title, visibility)
	}
	return createGitLabSnippet(p, content, name, title, visibility)
}

func createGitLabSnippet(p profile, content []byte, name, title, visibility string) (string, error) {
	payload := glRequest{
		Title:      title,
		Visibility: visibility,
		Files:      []glFile{{FilePath: name, Content: string(content)}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to encode request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, p.URL+"/api/v4/snippets", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("PRIVATE-TOKEN", p.Token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		errBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitLab returned %d: %s", resp.StatusCode, errBody)
	}

	var result glResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	return result.WebURL, nil
}

func createGitHubGist(p profile, content []byte, name, title, visibility string) (string, error) {
	payload := ghRequest{
		Description: title,
		Public:      visibility == "public",
		Files:       map[string]ghFileContent{name: {Content: string(content)}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to encode request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, githubAPIURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		errBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub returned %d: %s", resp.StatusCode, errBody)
	}

	var result ghResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	return result.HTMLURL, nil
}
