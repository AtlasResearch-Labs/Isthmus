package logger

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

type Logger struct {
	mu     sync.Mutex
	out    io.Writer
	level  Level
	prefix string
}

var defaultLogger = &Logger{
	out:   os.Stdout,
	level: LevelInfo,
}

func SetLevel(level Level) {
	defaultLogger.mu.Lock()
	defer defaultLogger.mu.Unlock()
	defaultLogger.level = level
}

func SetOutput(w io.Writer) {
	defaultLogger.mu.Lock()
	defer defaultLogger.mu.Unlock()
	defaultLogger.out = w
}

var (
	recentLogs   []string
	recentLogsMu sync.RWMutex
	maxLogs      = 200
)

func AddLogHook(entry string) {
	recentLogsMu.Lock()
	defer recentLogsMu.Unlock()
	recentLogs = append(recentLogs, entry)
	if len(recentLogs) > maxLogs {
		recentLogs = recentLogs[len(recentLogs)-maxLogs:]
	}
}

func GetRecentLogs() []string {
	recentLogsMu.RLock()
	defer recentLogsMu.RUnlock()
	res := make([]string, len(recentLogs))
	copy(res, recentLogs)
	return res
}

func (l *Logger) log(level Level, format string, args ...interface{}) {
	if level < l.level {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	var entry string
	if l.prefix != "" {
		entry = fmt.Sprintf("[%s] [%s] [%s] %s", timestamp, level.String(), l.prefix, msg)
	} else {
		entry = fmt.Sprintf("[%s] [%s] %s", timestamp, level.String(), msg)
	}
	fmt.Fprintln(l.out, entry)
	AddLogHook(entry)
}

func Debug(format string, args ...interface{}) {
	defaultLogger.log(LevelDebug, format, args...)
}

func Info(format string, args ...interface{}) {
	defaultLogger.log(LevelInfo, format, args...)
}

func Warn(format string, args ...interface{}) {
	defaultLogger.log(LevelWarn, format, args...)
}

func Error(format string, args ...interface{}) {
	defaultLogger.log(LevelError, format, args...)
}

func WithPrefix(prefix string) *Logger {
	return &Logger{
		out:    defaultLogger.out,
		level:  defaultLogger.level,
		prefix: prefix,
	}
}

func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(LevelDebug, format, args...)
}

func (l *Logger) Info(format string, args ...interface{}) {
	l.log(LevelInfo, format, args...)
}

func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(LevelWarn, format, args...)
}

func (l *Logger) Error(format string, args ...interface{}) {
	l.log(LevelError, format, args...)
}
