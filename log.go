package log

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"
)

type Level uint8

// Log level
const (
	LevelPanic Level = iota
	LevelFatal
	LevelError
	LevelWarn
	LevelInfo
	LevelVerbose
	LevelDebug
	LevelTrace
)

func (l Level) String() string {
	switch l {
	case LevelPanic:
		return "Panic"
	case LevelFatal:
		return "Fatal"
	case LevelError:
		return "Error"
	case LevelWarn:
		return "Warn"
	case LevelInfo:
		return "Info"
	case LevelVerbose:
		return "Verbose"
	case LevelDebug:
		return "Debug"
	case LevelTrace:
		return "Trace"
	default:
		return "Unknown"
	}
}

func (l Level) Tag() string {
	switch l {
	case LevelPanic:
		return "[P]"
	case LevelFatal:
		return "[F]"
	case LevelError:
		return "[E]"
	case LevelWarn:
		return "[W]"
	case LevelInfo:
		return "[I]"
	case LevelVerbose:
		return "[V]"
	case LevelDebug:
		return "[D]"
	case LevelTrace:
		return "[T]"
	default:
		return "[U]"
	}
}

func (l Level) MarshalJSON() ([]byte, error) {
	str := l.String()
	data := make([]byte, 0, len(str)+2)
	data = append(data, '"')
	data = append(data, str...)
	data = append(data, '"')
	return data, nil
}

const (
	ModeRelease = "release"
	ModeDebug   = "debug"
)

const (
	Console = "console"
	File    = "file"
	Syslog  = "syslog"
)

// Capacity of buffer channel
const (
	BufferCapacity    = 8
	defaultTimelayout = "2006/01/02 15:04:05.000"
)

var (
	logctx *context = nil
)

func init() {
	logctx = &context{
		mode:        "debug",
		loggers:     make(map[string]Logger),
		initialized: 1,
	}

	logctx.regist(NewConsoleLogger(LevelDebug, Colorful()))
}

// Message represents Log message record, minimal log dispatch unit
type Message struct {
	Level     Level     `json:"level"`
	Filename  string    `json:"filename,omitempty"`
	Function  string    `json:"function,omitempty"`
	Line      int       `json:"line,omitempty"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Fields    Fields    `json:"-"`
}

type Option func(Logger)

type Formatter interface {
	Format(*Message) string
}

// Logger interface, all supported logger MUST implement this interface
type Logger interface {
	// Name returns logger's name
	Name() string
	// Level returns logger's level
	Level() Level
	// Write writes a message to logger
	Write(msg *Message)
	// Close closes a logger
	Close() error
}

type context struct {
	mode        string
	initialized uint32
	loggers     map[string]Logger
}

// internal log function
func (ctx *context) log(level Level, message string, fields ...Fields) {
	if len(message) == 0 {
		return
	}

	if ctx.mode == ModeRelease {
		skip := true
		for _, logger := range ctx.loggers {
			if logger.Level() >= level && level < LevelDebug {
				// logger found
				skip = false
				break
			}
		}

		if skip {
			return
		}
	}

	msg := &Message{
		Level:     level,
		Message:   message,
		Timestamp: time.Now(),
	}

	if len(fields) > 0 {
		msg.Fields = fields[0]
	}

	if level == LevelTrace {
		pc, file, line, ok := runtime.Caller(2)
		if ok {
			msg.Filename = filepath.Base(file)
			msg.Line = line
			msg.Function = runtime.FuncForPC(pc).Name()
		}
	}

	for _, logger := range ctx.loggers {
		if ctx.mode == ModeDebug || (logger.Level() >= level && level < LevelDebug) {
			logger.Write(msg)
		}
	}
}

func (ctx *context) regist(logger Logger) {
	if logger == nil {
		return
	}

	if l, ok := ctx.loggers[logger.Name()]; ok {
		l.Close()
	}

	ctx.loggers[logger.Name()] = logger
}

func (ctx *context) setMode(mode string) {
	if mode == ModeDebug || mode == "dev" || mode == "devel" {
		ctx.mode = ModeDebug
	} else if mode == ModeRelease {
		ctx.mode = ModeRelease
	} else {
		panic("invalid logger mode, only accept release/debug/dev/devel")
	}
}

func (ctx *context) setFormatter(logger string, formatter Formatter) error {
	if l, ok := ctx.loggers[logger]; ok {
		switch lg := l.(type) {
		case *console:
			lg.formatter = formatter
		case *file:
			lg.formatter = formatter
		case *syslog:
			lg.formatter = formatter
		}

		return nil
	}

	return fmt.Errorf("logger '%s' is not supported", logger)
}

func (ctx *context) close() {
	for key, logger := range ctx.loggers {
		logger.Close()
		delete(ctx.loggers, key)
	}

	atomic.StoreUint32(&ctx.initialized, 0)
}

// Regist adds a logger, Log package default add one logger(console), means default all
// the log message will print to console. But you can use this function to add new
// logger to log engine.
func Regist(logger Logger) {
	logctx.regist(logger)
}

// Close closes log engine
func Close() {
	logctx.close()
}

func SetMode(mode string) {
	logctx.setMode(mode)
}

func SetFormatter(logger string, formatter Formatter) error {
	return logctx.setFormatter(logger, formatter)
}

// Trace print trace message, which prints more details
func Trace(args ...interface{}) {
	logctx.log(LevelTrace, formatLogMessage(args...))
}

// Debug print debug message
func Debug(args ...interface{}) {
	logctx.log(LevelDebug, formatLogMessage(args...))
}

func Verbose(args ...interface{}) {
	logctx.log(LevelVerbose, formatLogMessage(args...))
}

// Info print information message
func Info(args ...interface{}) {
	logctx.log(LevelInfo, formatLogMessage(args...))
}

// Warning print warning message
func Warning(args ...interface{}) {
	logctx.log(LevelWarn, formatLogMessage(args...))
}

// Error print error message
func Error(args ...interface{}) {
	logctx.log(LevelError, formatLogMessage(args...))
}

// Fatal print fatal error message, and app will quit if this function called
func Fatal(args ...interface{}) {
	logctx.log(LevelFatal, formatLogMessage(args...))
	os.Exit(1)
}

// Panic print panic message, and app will trigger panic message if called
func Panic(args ...interface{}) {
	logctx.log(LevelPanic, formatLogMessage(args...))
	debug.PrintStack()
	os.Exit(1)
}

var fmtsign = regexp.MustCompile(`%[\+\-\#\s\d.]{0,}[vtTbcdoOqxXUeEfFgGsp]`)

func formatLogMessage(args ...interface{}) string {
	if len(args) == 0 {
		return ""
	}

	var msg string

	switch args[0].(type) {
	case string:
		msg = args[0].(string)
		if len(args) == 1 {
			return msg
		}

		if fmtsign.MatchString(msg) {
			msg += strings.Repeat(" %v", len(args)-1)
		}
	default:
		msg = fmt.Sprint(args[0])
		if len(args) == 1 {
			return msg
		}

		msg += strings.Repeat(" %v", len(args)-1)
	}

	return fmt.Sprintf(msg, args[1:]...)
}
