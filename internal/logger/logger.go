package logger

import (
	"log"
	"strings"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

var currentLevel = LevelInfo

// SetLevel sets the global logging verbosity level ("debug", "info", "warn", "error").
func SetLevel(levelStr string) {
	switch strings.ToLower(strings.TrimSpace(levelStr)) {
	case "debug":
		currentLevel = LevelDebug
	case "warn", "warning":
		currentLevel = LevelWarn
	case "error":
		currentLevel = LevelError
	case "info":
		fallthrough
	default:
		currentLevel = LevelInfo
	}
}

// IsDebug returns true if the current log level is DEBUG.
func IsDebug() bool {
	return currentLevel <= LevelDebug
}

// Debugf logs a formatted message if the current level is DEBUG.
func Debugf(format string, v ...interface{}) {
	if currentLevel <= LevelDebug {
		log.Printf("[DEBUG] "+format, v...)
	}
}

// Infof logs a formatted message if the current level is INFO or lower.
func Infof(format string, v ...interface{}) {
	if currentLevel <= LevelInfo {
		log.Printf("[INFO] "+format, v...)
	}
}

// Warnf logs a formatted message if the current level is WARN or lower.
func Warnf(format string, v ...interface{}) {
	if currentLevel <= LevelWarn {
		log.Printf("[WARN] "+format, v...)
	}
}

// Errorf logs a formatted message if the current level is ERROR or lower.
func Errorf(format string, v ...interface{}) {
	if currentLevel <= LevelError {
		log.Printf("[ERROR] "+format, v...)
	}
}
