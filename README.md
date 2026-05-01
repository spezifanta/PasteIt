# PasteIt

Share file content easily by creating [GitLab snippets](https://docs.gitlab.com/ee/user/snippets.html) or [GitHub gists](https://docs.github.com/en/get-started/writing-on-github/editing-and-sharing-content-with-gists/creating-gists) from the command line. The URL is printed and copied to your clipboard automatically.

## Quick example

```bash
$ cat example.txt | pasteit
Created private snippet. URL copied to clipboard:
https://gitlab.com/-/snippets/1234567
```

## Installation

```bash
go build -ldflags="-s -w" -o pasteit
sudo mv pasteit /usr/local/bin/
```

## Setup

On first use, PasteIt will run a setup wizard to create `~/.config/pasteit.toml`:

```
pasteit: no config found — running first-time setup
Config will be saved to: /home/user/.config/pasteit.toml

URL (e.g. https://gitlab.com or https://github.com): https://gitlab.com
Personal access token (input hidden):
Profile "default" saved.
```

Running `pasteit -P work` when the `work` profile doesn't exist will trigger the wizard again for that profile, appending it to the existing config.



## Config

`~/.config/pasteit.toml` supports multiple profiles for different instances and providers. The provider is detected automatically from the URL — any profile with `github.com` in the URL uses the GitHub Gists API, everything else is treated as a GitLab instance (including self-hosted GitLab).

```toml
[default]
token = "your-gitlab-token"
url   = "https://gitlab.com"

[work]
token = "your-work-gitlab-token"
url   = "https://git.example.com"

[github]
token = "your-github-token"
url   = "https://github.com"
```

- GitLab token needs the `api` scope
- GitHub token needs the `gist` scope

## Usage

```bash
# Upload a file
pasteit main.go

# Pipe content
cat main.go | pasteit
echo "hello" | pasteit

# Set snippet filename and title
cat main.go | pasteit -f main.go -t "my script"

# Make snippet public (default: private)
pasteit main.go -p

# Use a specific profile
pasteit main.go -P work
cat main.go | pasteit -P work -t "work snippet"
```

## Options

| Flag | Long | Description |
|------|------|-------------|
| `-f` | `--file` | Filename shown in the snippet / gist |
| `-t` | `--title` | Snippet title or gist description (default: `PasteIt snippet`) |
| `-p` | `--public` | Make snippet public (default: `private`) |
| `-P` | `--profile` | Config profile to use (default: `default`) |
|      | `--version` | Print version and exit |

## License

MIT — see [LICENSE](LICENSE).

## AI

This software was built with the assistance of [Claude](https://claude.ai) by Anthropic.
