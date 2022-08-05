package log

import (
	"fmt"
	slog "log/syslog"
	"strings"
	"sync"
	"sync/atomic"
)

func NewSysLogger(level Level, address string, options ...Option) Logger {
	var (
		err error
	)

	l := &syslog{
		level:  level,
		closed: 1,
	}

	network := ""
	if address != "" {
		network = "tcp"
		index := strings.Index(address, "://")
		if index > 0 {
			network = address[:index]
			address = address[index+3:]
		}
	}

	if l.writer, err = slog.Dial(network, address, slog.LOG_DEBUG, ""); err != nil {
		Error("Create syslog logger failed, %v", err)
		return nil
	}

	l.messages = make(chan *Message, BufferCapacity)
	l.closeNotify = make(chan struct{})

	for _, option := range options {
		option(l)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	go l.run(wg.Done)
	wg.Wait()

	return l
}

type syslog struct {
	level       Level
	writer      *slog.Writer
	messages    chan *Message
	formatter   Formatter
	closeNotify chan struct{}
	closed      uint32
}

func (l *syslog) Name() string {
	return Syslog
}

func (l *syslog) Write(msg *Message) {
	if msg == nil || l.writer == nil || atomic.LoadUint32(&l.closed) == 1 {
		return
	}

	l.messages <- msg
}

func (l *syslog) Level() Level {
	return l.level
}

func (l *syslog) Close() error {
	if !atomic.CompareAndSwapUint32(&l.closed, 0, 1) {
		return nil
	}

	close(l.closeNotify)
	close(l.messages)

	if l.writer != nil {
		return l.writer.Close()
	}

	return nil
}

func (l *syslog) Format(msg *Message) string {
	if msg == nil {
		return ""
	}

	if l.formatter == nil {
		if msg.Filename != "" && msg.Function != "" {
			return fmt.Sprintf("%s [%s:%d - %s] %s", msg.Level.Tag(), msg.Filename, msg.Line, msg.Function, msg.Message)
		}

		return fmt.Sprintf("%s %s", msg.Level.Tag(), msg.Message)
	}

	return l.formatter.Format(msg)
}

func (l *syslog) write(msg *Message) {
	msgstr := l.Format(msg)
	l.writer.Write([]byte(msgstr))
}

func (l *syslog) run(ready func()) {

	atomic.StoreUint32(&l.closed, 0)
	ready()

	for {
		select {
		case msg := <-l.messages:
			l.write(msg)
		case <-l.closeNotify:
			return
		}
	}
}
