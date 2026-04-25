package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/zachariahcox/snippets/internal/cache"
	"github.com/zachariahcox/snippets/internal/logging"
	"github.com/zachariahcox/snippets/jira"
)

// Version and BuildDate are set via ldflags when building with make.
var (
	Version   = "dev"
	BuildDate = "unknown"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	logging.SetLevel(logging.LevelWarning)

	if len(args) == 0 {
		printRootUsage(os.Stderr)
		return 1
	}

	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") {
		switch args[i] {
		case "-h", "--help":
			printRootUsage(os.Stdout)
			return 0
		case "--version", "-version":
			fmt.Printf("snippets %s (built %s)\n", Version, BuildDate)
			return 0
		case "-v", "--verbose":
			logging.SetLevel(logging.LevelDebug)
			i++
		default:
			fmt.Fprintf(os.Stderr, "unknown global flag %q\n\n", args[i])
			printRootUsage(os.Stderr)
			return 2
		}
	}

	rest := args[i:]
	if len(rest) == 0 {
		printRootUsage(os.Stderr)
		return 1
	}

	cmd := rest[0]
	cmdArgs := rest[1:]

	switch cmd {
	case "help":
		if len(cmdArgs) == 0 {
			printRootUsage(os.Stdout)
			return 0
		}
		switch cmdArgs[0] {
		case "jira":
			jira.PrintUsage(os.Stdout)
			fmt.Fprintln(os.Stdout, "\nRun snippets jira -h for all flags.")
			return 0
		default:
			fmt.Fprintf(os.Stderr, "unknown help topic %q\n", cmdArgs[0])
			return 2
		}

	case "jira":
		return jira.Run(cmdArgs)

	case "cache":
		return runCache(cmdArgs)

	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		printRootUsage(os.Stderr)
		return 2
	}
}

func runCache(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: snippets cache clear")
		return 1
	}
	switch args[0] {
	case "clear":
		if err := cache.Clear(); err != nil {
			logging.Error("Failed to clear cache: %v", err)
			return 1
		}
		fmt.Println("Cache cleared.")
		return 0
	case "-h", "--help":
		fmt.Fprintln(os.Stdout, "Usage: snippets cache clear")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown cache subcommand %q\n", args[0])
		fmt.Fprintln(os.Stderr, "Usage: snippets cache clear")
		return 2
	}
}

func printRootUsage(w io.Writer) {
	fmt.Fprintf(w, `snippets — small CLI tools

Usage:
  snippets [global options] <command> [command options] [arguments]

Global options:
  -v, --verbose     Enable debug logging for commands
  --version         Print version and exit
  -h, --help        Show this help (or use: snippets help <command>)

Commands:
  jira    Jira issue status reports (keys, JQL, markdown/JSON/CSV/…)
  cache   Manage the shared cache directory (~/.snippets/cache)
  help    Show help for a command

Examples:
  snippets jira PROJECT-123
  snippets -v jira --jql "project = MYPROJ AND status != Done"
  snippets cache clear

Run snippets jira -h for Jira-specific flags and environment variables.
`)
}
