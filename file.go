package log

import (
	"archive/tar"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	RotateByDuration = "RotateByDuration"
	RotateBySize     = "RotateBySize"

	SweepByFileCount = "SweepByFileCount"
	SweepByInterval  = "SweepByInterval"

	DefaultRotatePolicy   = RotateByDuration
	DefaultSweepPolicy    = SweepByFileCount
	DefaultRotateFileSize = 50 << 20           // rotate log file every 50M
	DefaultRotateDuration = 24 * time.Hour     // rotate log file every 24 hours
	DefaultSweepInterval  = 7 * 24 * time.Hour // sweep log file 7 days before
	DefaultSweepFileCount = 5

	defaultCacheSize         = 2 << 10
	defaultLogfileTimeLayout = "2006-01-02T150405"
)

var (
	defaultFnFormatter = func() string {
		return fmt.Sprintf("%s-%s.log", filepath.Base(os.Args[0]), time.Now().Format(defaultLogfileTimeLayout))
	}
	defaultFnRegex = regexp.MustCompile(fmt.Sprintf("%s-\\d+-\\d+-\\d+T\\d+\\.tgz", filepath.Base(os.Args[0])))
)

func RotatePolicy(policy string) Option {
	return func(logger Logger) {
		if f, ok := logger.(*file); ok {
			f.rotatePolicy = policy
		}
	}
}

func RotateDuration(duration time.Duration) Option {
	return func(logger Logger) {
		if f, ok := logger.(*file); ok {
			f.rotateDuration = duration
		}
	}
}

func RotateFileSize(size int64) Option {
	return func(logger Logger) {
		if f, ok := logger.(*file); ok {
			f.rotateFileSize = size
		}
	}
}

func SweepPolicy(policy string) Option {
	return func(logger Logger) {
		if f, ok := logger.(*file); ok {
			f.sweepPolicy = policy
		}
	}
}

func SweepInterval(interval time.Duration) Option {
	return func(logger Logger) {
		if f, ok := logger.(*file); ok {
			f.sweepInterval = interval
		}
	}
}

func SweepFileCount(count int) Option {
	return func(logger Logger) {
		if f, ok := logger.(*file); ok {
			f.sweepFileCount = count
		}
	}
}

func Path(path string, patterns ...string) Option {
	return func(l Logger) {
		if f, ok := l.(*file); ok {
			f.path = path
			if len(patterns) == 0 {
				f.format = defaultFnFormatter
				f.fnregex = defaultFnRegex
				return
			}

			pattern := patterns[0]

			for _, ch := range pattern {
				if os.IsPathSeparator(uint8(ch)) {
					panic("pattern contains path separator")
				}
			}

			var (
				prefix string
				suffix string
			)

			index := strings.LastIndexByte(pattern, '*')
			if index >= 0 {
				prefix = pattern[:index]
				suffix = pattern[index+1:]
			} else {
				prefix = pattern
			}

			if !strings.HasSuffix(suffix, ".log") {
				suffix = suffix + ".log"
			}

			f.fnregex = regexp.MustCompile(fmt.Sprintf("%s\\d+%s\\.tgz", prefix, strings.TrimSuffix(suffix, "log")))
			f.format = func() string {
				rand.Seed(time.Now().Unix())
				try := 0
				for try < 100 {
					filename := fmt.Sprintf("%s%d%s", prefix, rand.Int31(), suffix)
					if _, err := os.Stat(filepath.Join(f.path, filename)); err != nil && os.IsNotExist(err) {
						return filename
					}
					try++
				}

				return ""
			}
		}
	}
}

// file logger print log message to a specified file, log file will rotate to new
// file daily, and will cleanup old log files, the file logger engine cached one
// month's log data and will remove older log files, and you can change the rotate
// time duration and cached log interval
type file struct {
	level          Level
	path           string
	sweepPolicy    string
	sweepInterval  time.Duration
	sweepFileCount int
	rotatePolicy   string        // RotateByDuration, RotateBySize
	rotateDuration time.Duration // sweep log files with 'interval' days before
	rotateFileSize int64
	filesize       int64
	filename       string
	file           *os.File
	format         func() string
	fnregex        *regexp.Regexp
	buf            *bytes.Buffer
	formatter      Formatter
	messages       chan *Message
	closeNotify    chan struct{}
	closed         uint32
}

// NewFileLogger creates a file logger implementation
func NewFileLogger(level Level, options ...Option) Logger {
	f := &file{
		level:          level,
		path:           "",
		filename:       "",
		file:           nil,
		formatter:      new(TextFormatter),
		rotatePolicy:   DefaultRotatePolicy,
		rotateDuration: DefaultRotateDuration,
		rotateFileSize: DefaultRotateFileSize,
		sweepPolicy:    DefaultSweepPolicy,
		sweepFileCount: DefaultSweepFileCount,
		sweepInterval:  DefaultSweepInterval,
		filesize:       0,
		format:         defaultFnFormatter,
		fnregex:        defaultFnRegex,
		buf:            bytes.NewBuffer(make([]byte, 0, defaultCacheSize)),
		messages:       make(chan *Message, BufferCapacity),
		closeNotify:    make(chan struct{}),
	}

	for _, option := range options {
		option(f)
	}

	if len(f.path) > 0 {
		os.MkdirAll(f.path, 0770)
	} else {
		f.path = "."
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go f.run(wg.Done)

	wg.Wait()

	return f
}

func (f *file) run(ready func()) {
	var (
		elapse time.Duration
		err    error
	)

	if err = f.rotate(); err != nil {
		panic(err)
	}

	ready()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case msg := <-f.messages:
			f.write(msg)
		case <-ticker.C:
			elapse += time.Minute
			if (f.rotatePolicy == RotateBySize && f.filesize >= f.rotateFileSize) ||
				(f.rotatePolicy == RotateByDuration && elapse >= f.rotateDuration) {
				_ = f.rotate()
				elapse = 0
				go f.sweep()
			}
		case <-f.closeNotify:
			return
		}
	}
}

func (f *file) compress() error {
	var (
		gzfilename string
		gzw        *gzip.Writer
		gzfile     *os.File
		lfile      *os.File
		tw         *tar.Writer
		info       os.FileInfo
		header     *tar.Header
		e          error
	)

	return filepath.WalkDir(f.path, func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() || f.filename == d.Name() {
			return nil
		}

		if strings.HasSuffix(path, ".log") {
			if info, e = d.Info(); e != nil {
				return e
			}

			gzfilename = path[:len(path)-4] + ".tgz"
			if gzfile, e = os.Create(gzfilename); e != nil {
				return e
			}
			defer gzfile.Close()

			if lfile, e = os.Open(path); e != nil {
				return e
			}
			defer lfile.Close()

			if gzw, e = gzip.NewWriterLevel(gzfile, flate.BestCompression); e != nil {
				return e
			}
			defer func() {
				gzw.Flush()
				gzw.Close()
			}()

			tw = tar.NewWriter(gzw)
			defer tw.Close()

			if header, e = tar.FileInfoHeader(info, ""); e != nil {
				return e
			}

			if e = tw.WriteHeader(header); e != nil {
				return e
			}

			if _, e = io.Copy(tw, lfile); e != nil {
				return e
			}

			os.Remove(path)
		}

		return nil
	})
}

func (f *file) rotate() error {
	var (
		fname string
		err   error
	)

	if f.file != nil {
		f.file.Close()
	}

	// compress old log files
	if err = f.compress(); err != nil {
		f.write(&Message{
			Level:     LevelError,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	f.filesize = 0
	f.filename = f.format()
	if f.filename == "" {
		f.filename = defaultFnFormatter()
	}

	fname = f.path + "/" + f.filename
	f.file, err = os.OpenFile(fname, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0660)

	return err
}

type logfile struct {
	timestamp int64
	filename  string
}

type byTimestamp []*logfile

func (bts byTimestamp) Len() int {
	return len(bts)
}

func (bts byTimestamp) Less(i, j int) bool {
	return bts[i].timestamp > bts[j].timestamp
}

func (bts byTimestamp) Swap(i, j int) {
	bts[i], bts[j] = bts[j], bts[i]
}

func (f *file) sweep() {
	var files byTimestamp

	tm := time.Now()
	filepath.WalkDir(f.path, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fs.SkipDir
		}

		if path == f.path {
			return nil
		}

		if entry.IsDir() {
			return fs.SkipDir
		}

		if f.fnregex.MatchString(path) || (f.fnregex != defaultFnRegex && defaultFnRegex.MatchString(path)) {
			if fi, err := entry.Info(); err == nil {
				if f.sweepPolicy == SweepByInterval {
					if tm.Before(fi.ModTime().Add(f.sweepInterval)) {
						os.Remove(path)
					}
				} else if f.sweepPolicy == SweepByFileCount {
					if files == nil {
						files = make(byTimestamp, 0, 16)
					}
					files = append(files, &logfile{
						timestamp: fi.ModTime().Unix(),
						filename:  path,
					})
				}
			}
		}

		return nil
	})

	if len(files) > f.sweepFileCount && f.sweepFileCount > 0 {
		sort.Sort(files)
		for i := f.sweepFileCount; i < files.Len(); i++ {
			os.Remove(files[i].filename)
		}
	}
}

func (f *file) write(msg *Message) {
	if msg == nil {
		return
	}

	msgstr := f.Format(msg)

	if len(msgstr)+f.buf.Len() >= defaultCacheSize && f.file != nil {
		n, _ := f.file.Write(f.buf.Bytes())
		f.filesize += int64(n)
		f.buf.Reset()
	}

	f.buf.WriteString(msgstr)
	f.buf.WriteByte('\n')
}

func (f *file) flush() {
	if f.file != nil {
		n, _ := f.file.Write(f.buf.Bytes())
		f.filesize += int64(n)
		f.buf.Reset()
	}
}

func (f *file) Name() string {
	return File
}

func (f *file) Level() Level {
	return f.level
}

func (f *file) Write(msg *Message) {
	if atomic.LoadUint32(&f.closed) == 1 {
		return
	}

	f.messages <- msg
}

func (f *file) Close() error {
	if !atomic.CompareAndSwapUint32(&f.closed, 0, 1) {
		return nil
	}

	<-time.After(time.Second)

	close(f.closeNotify)
	close(f.messages)

	f.flush()

	if f.file != nil {
		f.file.Close()
	}

	return nil
}

func (f *file) Format(msg *Message) string {
	if f.formatter == nil {
		f.formatter = new(TextFormatter)
	}

	return f.formatter.Format(msg)
}
