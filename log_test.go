package log

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"testing"
)

func TestLog(t *testing.T) {

	fmt.Println("Start testing log...")

	// SetFormatter(Console, new(JSONFormatter))

	Fields{"name": "derek", "age": 35}.Debug("Test Fields")

	Debug("Test Debug")
	Info("Test Info")
	Warning("Test Warning")
	Verbose("Test Verbose")
	Trace("Test Trace")
	Error("Test Error")
	// Fatal("Test Fatal")
	Panic("Test Panic")
}

// 18446	     80701 ns/op	    1385 B/op	      26 allocs/op

func BenchmarkLog(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Fields{"name": "derek", "age": 35}.Debug("Test Fields")
		Debug("Test Debug")
		Info("Test Info")
		Warning("Test Warning")
		Verbose("Test Verbose")
		Trace("Test Trace")
		Error("Test Error")
	}
}

func TestWalk(t *testing.T) {
	root := ".."
	filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fs.SkipDir
		}

		t.Log(path)
		if path == root {
			return nil
		}

		if entry.IsDir() {
			return fs.SkipDir
		}

		return nil
	})
}

func TestRegex(t *testing.T) {
	re := regexp.MustCompile(`\d{4}-[0-1][0-2]-[0-3][0-9]T[0-2][0-9][0-6][0-9][0-6][0-9]\.log`)
	if re.MatchString("2001-02-31T030242.log") {
		t.Log("Match")
	}
}

func TestNewFileLogger(t *testing.T) {
	Regist(NewFileLogger(LevelDebug, Path("logs", "test-*")))
	Trace("trace")
	Debug("debug")
	Info("info")
	Close()
}

func TestFmtSign(t *testing.T) {
	signs := []string{
		"test%#v",
		"test%v",
		"test%-d",
		"test%+d",
		"test% d",
		"test%2.f",
		"test%2.3f",
	}

	for _, sign := range signs {
		if fmtsign.MatchString(sign) {
			t.Logf("%s Matched", sign)
		} else {
			t.Logf("%s Not Matched", sign)
		}
	}
}
