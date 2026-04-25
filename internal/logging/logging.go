// Package logging provides level-gated logging for the snippets CLI and subcommands.
package logging

import "log"

// Log levels (higher value = more verbose).
const (
	LevelError = iota
	LevelWarning
	LevelInfo
	LevelDebug
)

var level = LevelWarning

// SetLevel sets the minimum log level.
func SetLevel(l int) {
	level = l
}

// Level returns the current minimum log level.
func Level() int {
	return level
}

// Debug logs at debug level.
func Debug(format string, args ...any) {
	if level >= LevelDebug {
		log.Printf("DEBUG: "+format, args...)
	}
}

// Info logs at info level.
func Info(format string, args ...any) {
	if level >= LevelInfo {
		log.Printf("INFO: "+format, args...)
	}
}

// Warning logs at warning level.
func Warning(format string, args ...any) {
	if level >= LevelWarning {
		log.Printf("WARNING: "+format, args...)
	}
}

// Error always logs (no level gate).
func Error(format string, args ...any) {
	log.Printf("ERROR: "+format, args...)
}
