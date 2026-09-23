// Command ccxray renders a live Claude Code turn as a log table.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dontfeedthecode/ccxray/internal/discover"
	"github.com/dontfeedthecode/ccxray/internal/ui"
)

// set by the linker at release time:
//
//	-ldflags "-X main.version=v1.2.3 -X main.commit=abc1234"
var (
	version = "dev"
	commit  = "none"
)

// buildVersion falls back to the module version Go records for
// `go install …@vX.Y.Z`, which does not pass the release ldflags.
func buildVersion() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return version
}

func main() {
	var (
		project = flag.String("project", "", "working directory of the Claude Code session (default: cwd)")
		session = flag.String("session", "", "session id to follow (default: most recent)")
		ascii   = flag.Bool("ascii", false, "ASCII-only glyphs, for terminals that widen ambiguous runes")
		showVer = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("ccxray %s (%s)\n", buildVersion(), commit)
		return
	}

	cwd := *project
	if cwd == "" {
		var err error
		if cwd, err = os.Getwd(); err != nil {
			fmt.Fprintln(os.Stderr, "cannot determine working directory:", err)
			os.Exit(1)
		}
	}
	var dir, path string
	if *session != "" {
		dir = discover.ProjectDir(cwd)
		path = discover.Session(dir, *session)
		if _, err := os.Stat(path); err != nil {
			fmt.Fprintf(os.Stderr, "session %s not found\n  looked for %s\n", *session, path)
			os.Exit(1)
		}
		dir = "" // explicit session: do not auto-switch
	}

	// Without --session the panel starts empty and attaches to the first
	// session written after launch, in this directory or any parent, rather
	// than replaying whatever ran last.
	m := ui.NewModel(ui.Options{
		Dir: dir, Path: path, ASCII: *ascii,
		Watch: discover.Candidates(cwd), Since: time.Now(), CWD: cwd,
	})
	if _, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
