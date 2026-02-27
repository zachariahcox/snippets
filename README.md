# snippets

Generate status reports for Jira issues via the REST API. Supports JQL queries, subtasks, linked issues, and markdown output.

## Installation

Download the latest release: [Releases](https://github.com/zachariahcox/snippets/releases)
```bash
tag=$(curl -s https://api.github.com/repos/zachariahcox/snippets/releases/latest | jq -r '.tag_name'
curl -L -o /usr/local/bin/snippets https://github.com/zachariahcox/snippets/releases/download/$tag/snippets-linux-amd64
chmod +x /usr/local/bin/snippets
```

...or better if you have the gh cli
```bash
gh release download --repo zachariahcox/snippets --pattern "snippets-linux-amd64"
```

Install via `make install`
```bash
git clone https://github.com/zachariahcox/snippets
cd snippets
make build install
```

## Usage

Set required environment variables:

```bash
export JIRA_SERVER="https://mycompany.atlassian.net"
export JIRA_API_TOKEN="your-token"
export JIRA_EMAIL="you@company.com"   # required for Jira Cloud
```

Run with issue keys or a JQL query:

```bash
snippets PROJECT-123 PROJECT-456
snippets --jql "project = MYPROJ AND status != Done"
snippets --include-subtasks --output-file status.md PROJECT-123
```

Use `snippets -h` for all options.

Use `--json` to interop with other tools.
