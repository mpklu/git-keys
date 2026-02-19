package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

// Level represents the logging level
type Level int

const (
	ERROR Level = iota
	WARN
	INFO
	DEBUG
	TRACE
)

var (
	levelNames = map[Level]string{
		ERROR: "ERROR",
		WARN:  "WARN",
		INFO:  "INFO",
		DEBUG: "DEBUG",
		TRACE: "TRACE",
	}

	currentLevel           = INFO
	output       io.Writer = os.Stderr
)

// SetLevel sets the global logging level
func SetLevel(level Level) {
	currentLevel = level
}

// SetLevelFromString sets the logging level from a string
func SetLevelFromString(levelStr string) error {
	switch strings.ToUpper(levelStr) {
	case "ERROR":
		currentLevel = ERROR
	case "WARN", "WARNING":
		currentLevel = WARN
	case "INFO":
		currentLevel = INFO
	case "DEBUG":
		currentLevel = DEBUG
	case "TRACE":
		currentLevel = TRACE
	default:
		return fmt.Errorf("invalid log level: %s", levelStr)
	}
	return nil
}

// SetOutput sets the output writer for logs
func SetOutput(w io.Writer) {
	output = w
}

func logf(level Level, format string, args ...interface{}) {
	if level <= currentLevel {
		levelName := levelNames[level]
		msg := fmt.Sprintf(format, args...)
		log.New(output, fmt.Sprintf("[%s] ", levelName), log.LstdFlags).Println(msg)
	}
}

// Error logs an error-level message
func Error(format string, args ...interface{}) {
	logf(ERROR, format, args...)
}

// Warn logs a warning-level message
func Warn(format string, args ...interface{}) {
	logf(WARN, format, args...)
}

// Info logs an info-level message
func Info(format string, args ...interface{}) {
	logf(INFO, format, args...)
}

// Debug logs a debug-level message
func Debug(format string, args ...interface{}) {
	logf(DEBUG, format, args...)
}

// Trace logs a trace-level message
func Trace(format string, args ...interface{}) {
	logf(TRACE, format, args...)
}
