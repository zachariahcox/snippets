package main

import (
	"testing"
)

func TestLogLevelConstants(t *testing.T) {
	if LogLevelError >= LogLevelWarning || LogLevelWarning >= LogLevelInfo || LogLevelInfo >= LogLevelDebug {
		t.Error("log levels should be ordered Error < Warning < Info < Debug")
	}
}

func TestLogFunctionsNoPanic(t *testing.T) {
	// Ensure log helpers don't panic when called
	prev := logLevel
	defer func() { logLevel = prev }()
	logLevel = LogLevelDebug
	logDebug("test %s", "message")
	logInfo("test %s", "message")
	logWarning("test %s", "message")
	logError("test %s", "message")
}
