package log

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/mattn/go-isatty"
)

//Console logger, print log message to console, and each message level has
//different color
type console struct {
	level     Level
	colors    []string
	formatter Formatter
}

func Colorful() Option {
	return func(l Logger) {
		if c, ok := l.(*console); ok {
			if runtime.GOOS != "windows" {
				c.colors = []string{
					LevelPanic:   "",
					LevelFatal:   "\033[35m",
					LevelError:   "\033[31m",
					LevelWarn:    "\033[33m",
					LevelInfo:    "\033[36m",
					LevelVerbose: "\033[37m",
					LevelDebug:   "\033[32m",
					LevelTrace:   "\033[34m",
				}
			}
		}
	}
}

//NewConsoleLogger creates a new console logger
func NewConsoleLogger(level Level, options ...Option) Logger {
	cl := &console{
		level:     level,
		formatter: new(TextFormatter),
	}

	for _, option := range options {
		option(cl)
	}

	return cl
}

func (c *console) Name() string {
	return Console
}

func (c *console) Level() Level {
	return c.level
}

func (c *console) Write(msg *Message) {
	if len(c.colors) >= int(msg.Level) && isatty.IsTerminal(os.Stdout.Fd()) {
		fmt.Fprintf(os.Stdout, "%s\033[0m\n", strings.Replace(c.Format(msg), msg.Level.Tag(), c.colors[msg.Level]+msg.Level.Tag(), 1))
		return
	}

	fmt.Fprintln(os.Stdout, c.Format(msg))
}

func (c *console) Close() error {
	return nil
}

func (c *console) Format(msg *Message) string {
	if c.formatter == nil {
		c.formatter = new(TextFormatter)
	}

	return c.formatter.Format(msg)
}
