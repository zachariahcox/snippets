package main

import "log"

// Log levels
const (
	LogLevelError = iota
	LogLevelWarning
	LogLevelInfo
	LogLevelDebug
)

var logLevel = LogLevelWarning

func logDebug(format string, args ...any) {
	if logLevel >= LogLevelDebug {
		log.Printf("DEBUG: "+format, args...)
	}
}

func logInfo(format string, args ...any) {
	if logLevel >= LogLevelInfo {
		log.Printf("INFO: "+format, args...)
	}
}

func logWarning(format string, args ...any) {
	if logLevel >= LogLevelWarning {
		log.Printf("WARNING: "+format, args...)
	}
}

func logError(format string, args ...any) {
	log.Printf("ERROR: "+format, args...)
}
