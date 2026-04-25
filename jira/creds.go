package jira

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const credsFileName = ".snippets/creds.sh"

// LoadJiraCustomFieldNames returns JIRA_DUE_DATE_FIELD and JIRA_TRENDING_STATUS_FIELD from the environment,
// then fills missing values from ~/.snippets/creds.sh when that file exists (same pattern as LoadJiraCreds).
func LoadJiraCustomFieldNames(credsFilePath string) (dueDateField, trendingStatusField string) {
	dueDateField = strings.TrimSpace(os.Getenv("JIRA_DUE_DATE_FIELD"))
	trendingStatusField = strings.TrimSpace(os.Getenv("JIRA_TRENDING_STATUS_FIELD"))

	var credsPath string
	if credsFilePath != "" {
		credsPath = credsFilePath
	} else {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			credsPath = ""
		} else {
			credsPath = filepath.Join(home, credsFileName)
		}
	}
	if credsPath == "" {
		return dueDateField, trendingStatusField
	}
	if _, statErr := os.Stat(credsPath); statErr != nil {
		return dueDateField, trendingStatusField
	}
	script := fmt.Sprintf(`. %q 2>/dev/null; echo "JIRA_DUE_DATE_FIELD=$JIRA_DUE_DATE_FIELD"; echo "JIRA_TRENDING_STATUS_FIELD=$JIRA_TRENDING_STATUS_FIELD"`, credsPath)
	cmd := exec.Command("sh", "-c", script)
	cmd.Env = os.Environ()
	out, cmdErr := cmd.Output()
	if cmdErr != nil {
		return dueDateField, trendingStatusField
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		i := strings.Index(line, "=")
		if i <= 0 {
			continue
		}
		key, val := line[:i], line[i+1:]
		val = strings.Trim(val, "\"")
		switch key {
		case "JIRA_DUE_DATE_FIELD":
			if dueDateField == "" && val != "" {
				dueDateField = strings.TrimSpace(val)
			}
		case "JIRA_TRENDING_STATUS_FIELD":
			if trendingStatusField == "" && val != "" {
				trendingStatusField = strings.TrimSpace(val)
			}
		}
	}
	return dueDateField, trendingStatusField
}

// LoadJiraCreds returns JIRA_SERVER, JIRA_API_TOKEN, and JIRA_EMAIL from the environment
// and optionally from a creds shell script (sourced in a subprocess). Env vars take precedence.
func LoadJiraCreds(credsFilePath string) (server, apiToken, email string, err error) {
	server = os.Getenv("JIRA_SERVER")
	apiToken = os.Getenv("JIRA_API_TOKEN")
	email = os.Getenv("JIRA_EMAIL")

	var credsPath string
	if credsFilePath != "" {
		credsPath = credsFilePath
	} else {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			credsPath = ""
		} else {
			credsPath = filepath.Join(home, credsFileName)
		}
	}
	if credsPath != "" {
		if _, statErr := os.Stat(credsPath); statErr == nil {
			script := fmt.Sprintf(`. %q 2>/dev/null; echo "JIRA_SERVER=$JIRA_SERVER"; echo "JIRA_API_TOKEN=$JIRA_API_TOKEN"; echo "JIRA_EMAIL=$JIRA_EMAIL"`, credsPath)
			cmd := exec.Command("sh", "-c", script)
			cmd.Env = os.Environ()
			out, cmdErr := cmd.Output()
			if cmdErr == nil {
				for _, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					i := strings.Index(line, "=")
					if i <= 0 {
						continue
					}
					key, val := line[:i], line[i+1:]
					val = strings.Trim(val, "\"")
					switch key {
					case "JIRA_SERVER":
						if server == "" && val != "" {
							server = val
						}
					case "JIRA_API_TOKEN":
						if apiToken == "" && val != "" {
							apiToken = val
						}
					case "JIRA_EMAIL":
						if email == "" && val != "" {
							email = val
						}
					}
				}
			}
		}
	}

	if server == "" {
		return "", "", "", fmt.Errorf("JIRA_SERVER is not set (set env var or export from ~/.snippets/creds.sh)")
	}
	if apiToken == "" {
		return "", "", "", fmt.Errorf("JIRA_API_TOKEN is not set (set env var or export from ~/.snippets/creds.sh)")
	}
	return server, apiToken, email, nil
}

// ResolveJiraConcurrency returns max parallel Jira calls: flag > 0 wins, else JIRA_CONCURRENCY, else 8.
func ResolveJiraConcurrency(flagVal int, env string) int {
	if flagVal > 0 {
		return flagVal
	}
	if env != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(env)); err == nil && n > 0 {
			return n
		}
	}
	return 8
}
