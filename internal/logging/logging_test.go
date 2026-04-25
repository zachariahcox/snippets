package logging

import "testing"

func TestLevelConstants(t *testing.T) {
	if LevelError >= LevelWarning || LevelWarning >= LevelInfo || LevelInfo >= LevelDebug {
		t.Error("log levels should be ordered Error < Warning < Info < Debug")
	}
}

func TestLogFunctionsNoPanic(t *testing.T) {
	prev := level
	defer func() { level = prev }()
	SetLevel(LevelDebug)
	Debug("test %s", "message")
	Info("test %s", "message")
	Warning("test %s", "message")
	Error("test %s", "message")
}
