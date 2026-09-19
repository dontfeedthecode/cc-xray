// Package tail follows an append-only JSONL file, emitting whole lines.
package tail

import (
	"bytes"
	"io"
	"os"
	"time"
)

// Tailer reads only the bytes appended since the last call, keeping any
// partial trailing line for next time.
type Tailer struct {
	path    string
	off     int64
	partial []byte
	Bad     int // malformed/oversized lines dropped
}

func New(path string) *Tailer { return &Tailer{path: path} }

func (t *Tailer) Path() string { return t.path }

// Reset points the tailer at a new file, rewinding all state.
func (t *Tailer) Reset(path string) {
	t.path, t.off, t.partial = path, 0, nil
}

const maxLine = 8 << 20 // a single record should never approach this

// Read returns complete lines appended since the previous call.
func (t *Tailer) Read() ([][]byte, error) {
	f, err := os.Open(t.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	// Truncation or rotation: start over rather than reading garbage.
	if fi.Size() < t.off {
		t.off, t.partial = 0, nil
	}
	if fi.Size() == t.off {
		return nil, nil
	}
	if _, err := f.Seek(t.off, io.SeekStart); err != nil {
		return nil, err
	}

	buf, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	t.off += int64(len(buf))

	buf = append(t.partial, buf...)
	// Parse only up to the last complete newline; hold the remainder.
	idx := bytes.LastIndexByte(buf, '\n')
	if idx < 0 {
		if len(buf) > maxLine {
			t.Bad++
			t.partial = nil
		} else {
			t.partial = buf
		}
		return nil, nil
	}
	complete, rest := buf[:idx], buf[idx+1:]
	t.partial = append([]byte(nil), rest...)

	var out [][]byte
	for _, ln := range bytes.Split(complete, []byte{'\n'}) {
		if len(bytes.TrimSpace(ln)) == 0 {
			continue
		}
		out = append(out, append([]byte(nil), ln...))
	}
	return out, nil
}

// Changed reports whether the file has grown or been replaced since off.
func (t *Tailer) Changed() bool {
	fi, err := os.Stat(t.path)
	if err != nil {
		return false
	}
	return fi.Size() != t.off
}

// PollInterval is the fallback cadence when fsnotify is unavailable.
const PollInterval = 150 * time.Millisecond
