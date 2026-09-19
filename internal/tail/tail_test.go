package tail

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPartialLinesHeldBack(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	os.WriteFile(p, []byte(`{"a":1}`+"\n"+`{"b":2}`), 0o644)
	tl := New(p)
	lines, err := tl.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 {
		t.Fatalf("got %d complete lines, want 1 (second is partial)", len(lines))
	}
	// completing the partial line should now yield it
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("\n")
	f.Close()
	lines, _ = tl.Read()
	if len(lines) != 1 || string(lines[0]) != `{"b":2}` {
		t.Fatalf("partial not completed, got %q", lines)
	}
}

func TestTruncationResets(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	os.WriteFile(p, []byte("{\"a\":1}\n{\"a\":2}\n"), 0o644)
	tl := New(p)
	if l, _ := tl.Read(); len(l) != 2 {
		t.Fatalf("want 2 lines")
	}
	os.WriteFile(p, []byte("{\"c\":3}\n"), 0o644) // shrink
	l, err := tl.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(l) != 1 || string(l[0]) != `{"c":3}` {
		t.Fatalf("truncation not handled, got %q", l)
	}
}

func TestEmptyAndMissing(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "e.jsonl")
	os.WriteFile(p, nil, 0o644)
	if l, err := New(p).Read(); err != nil || len(l) != 0 {
		t.Fatalf("empty file: %v %v", l, err)
	}
	if _, err := New(filepath.Join(d, "nope.jsonl")).Read(); err == nil {
		t.Fatal("missing file should error")
	}
}
