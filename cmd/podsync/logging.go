package main

import (
	"bytes"

	log "github.com/sirupsen/logrus"

	"github.com/yangtfu/podsync/services/update"
)

// Whole-line highlight for entries flagged with update.FieldHighlight:
// blue background with bright text.
const (
	highlightStart = "\x1b[44;97m"
	highlightEnd   = "\x1b[0m"
)

// lineHighlightFormatter renders flagged entries with the background colour
// spanning the complete line (time, level, message and fields) so they are easy to
// spot while scrolling, and delegates everything else to inner unchanged.
type lineHighlightFormatter struct {
	inner log.Formatter
}

func (f *lineHighlightFormatter) Format(entry *log.Entry) ([]byte, error) {
	flagged, _ := entry.Data[update.FieldHighlight].(bool)
	if !flagged {
		return f.inner.Format(entry)
	}

	// Hide the marker field itself.
	clean := *entry
	clean.Data = make(log.Fields, len(entry.Data))
	for key, value := range entry.Data {
		if key != update.FieldHighlight {
			clean.Data[key] = value
		}
	}

	line, err := f.inner.Format(&clean)
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.WriteString(highlightStart)
	out.Write(bytes.TrimRight(line, "\n"))
	out.WriteString(highlightEnd)
	out.WriteByte('\n')
	return out.Bytes(), nil
}
