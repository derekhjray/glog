package log

import (
	"os"
	"runtime/debug"
)

type Fields map[string]interface{}

func (f Fields) Trace(args ...interface{}) {
	logctx.log(LevelTrace, formatLogMessage(args...), f)
}

func (f Fields) Debug(args ...interface{}) {
	logctx.log(LevelDebug, formatLogMessage(args...), f)
}

func (f Fields) Verbose(args ...interface{}) {
	logctx.log(LevelVerbose, formatLogMessage(args...), f)
}

func (f Fields) Info(args ...interface{}) {
	logctx.log(LevelInfo, formatLogMessage(args...), f)
}

func (f Fields) Warning(args ...interface{}) {
	logctx.log(LevelWarn, formatLogMessage(args...), f)
}

func (f Fields) Error(args ...interface{}) {
	logctx.log(LevelError, formatLogMessage(args...), f)
}

func (f Fields) Fatal(args ...interface{}) {
	logctx.log(LevelFatal, formatLogMessage(args...), f)
	os.Exit(1)
}

func (f Fields) Panic(args ...interface{}) {
	logctx.log(LevelPanic, formatLogMessage(args...), f)
	debug.PrintStack()
	os.Exit(1)
}
