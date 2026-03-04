# snippets

Generate status reports for Jira issues via the REST API. Supports JQL queries, subtasks, linked issues, and markdown output.

## Installation

Download the latest [release](https://github.com/zachariahcox/snippets/releases)
```bash
tag=$(curl -s https://api.github.com/repos/zachariahcox/snippets/releases/latest | jq -r '.tag_name'
curl -L -o /usr/local/bin/snippets https://github.com/zachariahcox/snippets/releases/download/$tag/snippets-linux-amd64
chmod +x /usr/local/bin/snippets
```

...or better if you have the gh cli
```bash
gh release download --repo zachariahcox/snippets --pattern "snippets-linux-amd64"
sudo install -m 755 snippets-linux-amd64 /usr/local/bin/snippets
```

Install via `make install`
```bash
git clone https://github.com/zachariahcox/snippets
cd snippets
make build install
```

## Usage

### Credentials

Set required environment variables, or use an optional credentials file.

**Environment variables:**

```bash
export JIRA_SERVER="https://mycompany.atlassian.net"
export JIRA_API_TOKEN="your-token"
export JIRA_EMAIL="you@company.com"   # required for Jira Cloud
```

**Optional credentials file:** `~/.snippets/creds.sh`

If the file exists, it is sourced in a subprocess and can export the same variables. Environment variables take precedence over the file. Useful for keeping tokens out of your shell profile or for different Jira instances.

```bash
mkdir -p ~/.snippets
cat > ~/.snippets/creds.sh << 'EOF'
export JIRA_SERVER="https://mycompany.atlassian.net"
export JIRA_API_TOKEN="your-token"
export JIRA_EMAIL="you@company.com"
EOF
chmod 600 ~/.snippets/creds.sh
```

Required: `JIRA_SERVER`, `JIRA_API_TOKEN`. Optional: `JIRA_EMAIL` (needed for Jira Cloud).

### Running reports

Run with issue keys or a JQL query:

```bash
snippets PROJECT-123 PROJECT-456
snippets --jql "project = MYPROJ AND status != Done"
snippets --include-subtasks --output-file status.md PROJECT-123
```

Use `snippets -h` for all options.

Use `--json` to interop with other tools.
