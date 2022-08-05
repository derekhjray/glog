package log

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
)

type TextFormatter struct {
	bp sync.Pool
}

func (tf *TextFormatter) acquire() *bytes.Buffer {
	if v := tf.bp.Get(); v != nil {
		return v.(*bytes.Buffer)
	}

	return bytes.NewBuffer(make([]byte, 0, 256))
}

func (tf *TextFormatter) recycle(buf *bytes.Buffer) {
	buf.Reset()
	tf.bp.Put(buf)
}

func (tf *TextFormatter) Format(msg *Message) (message string) {
	buf := tf.acquire()

	buf.WriteString(msg.Timestamp.Format(defaultTimelayout))
	buf.WriteByte(' ')
	buf.WriteString(msg.Level.Tag())
	if msg.Filename != "" && msg.Function != "" {
		buf.WriteString(" [")
		buf.WriteString(msg.Filename)
		buf.WriteByte(':')
		buf.WriteString(strconv.Itoa(msg.Line))
		buf.WriteString(" - ")
		buf.WriteString(msg.Function)
		buf.WriteByte(']')
	}

	buf.WriteByte(' ')
	buf.WriteString(msg.Message)

	if len(msg.Fields) > 0 {
		sep := ""
		buf.WriteString(" (")
		for key, value := range msg.Fields {
			buf.WriteString(sep)
			buf.WriteString(key)
			buf.WriteString(fmt.Sprintf(" = %v", value))
			if sep == "" {
				sep = ", "
			}
		}
		buf.WriteByte(')')
	}

	message = buf.String()
	tf.recycle(buf)

	return message
}

type JSONFormatter struct{}

func (jf *JSONFormatter) Format(msg *Message) string {
	if len(msg.Fields) > 0 {
		msg.Fields["level"] = msg.Level
		msg.Fields["message"] = msg.Message
		msg.Fields["timestamp"] = msg.Timestamp

		if msg.Function != "" && msg.Filename != "" {
			msg.Fields["line"] = msg.Line
			msg.Fields["function"] = msg.Function
			msg.Fields["filename"] = msg.Filename
		}

		data, err := json.Marshal(msg.Fields)
		if err != nil {
			fmt.Println(err)
			return ""
		}

		return string(data)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		fmt.Println(err)
		return ""
	}

	return string(data)
}
