# snippets

Generate status reports for Jira issues via the REST API. Supports JQL queries, subtasks, linked issues, and markdown output.

## Installation

Download the latest release: [Releases](https://github.com/zachariahcox/snippets/releases)

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
