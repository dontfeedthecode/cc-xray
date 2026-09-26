#!/bin/sh
# Preflight for /ccxray-demo. The demo needs Go on this Claude Code session's
# PATH (it runs go build and go test) and ccxray running in another terminal.
# This checks both, installs ccxray from this checkout when none is found, and
# says what the user has to do, if anything. Runs under sh on macOS and Linux
# and under Git Bash, which is Claude Code's Bash tool on Windows.
#
# The first line is one of:
#   ready    ccxray is running and Go works: carry on with the demo
#   start    ccxray is installed but not running; the lines after say how
#   restart  Go is installed, but this Claude Code session started before it
#   no-go    Go is not installed
#   error    something failed; the output says what
# It always exits 0, so it never draws a failure row on the panel it checks for.

case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) win=1 exe=.exe ;;
  *) win= exe= ;;
esac

# ccxray should start where Claude Code runs; it is built from this checkout.
here=$PWD
root=$(cd "$(dirname "$0")/../../.." && pwd)

# native writes a path the way the user's own terminal wants it.
native() { if [ -n "$win" ]; then cygpath -w "$1"; else printf '%s\n' "$1"; fi; }

running() {
  if [ -n "$win" ]; then
    # the doubled slashes stop Git Bash rewriting the switches as paths
    tasklist.exe //FI "IMAGENAME eq ccxray.exe" //NH 2>/dev/null | grep -qi 'ccxray\.exe'
  else
    pgrep -x ccxray >/dev/null 2>&1
  fi
}

# Go on PATH, or failing that where its installers put it. Found only there,
# it was installed after Claude Code started, which inherited the old PATH.
go=$(command -v go 2>/dev/null) stale=
if [ -z "$go" ]; then
  pf=
  [ -n "$win" ] && [ -n "${PROGRAMFILES:-}" ] && pf="$(cygpath -u "$PROGRAMFILES")/Go/bin/go.exe"
  for g in "$pf" /usr/local/go/bin/go /opt/homebrew/bin/go; do
    if [ -n "$g" ] && [ -x "$g" ]; then go=$g stale=1; break; fi
  done
fi

if [ -z "$go" ]; then
  echo "no-go"
  echo "The demo runs go build and go test, and Go is not installed."
  if [ -n "$win" ]; then
    echo "Install it with: winget install GoLang.Go"
  elif [ "$(uname -s)" = Darwin ]; then
    echo "Install it with: brew install go   (or from https://go.dev/dl/)"
  else
    echo "Install it from https://go.dev/dl/ or your package manager."
  fi
  echo "Then start Claude Code again from a new terminal window, so it can see Go, and rerun /ccxray-demo."
  exit 0
fi

# Where go install puts binaries.
bin=$("$go" env GOBIN)
[ -n "$bin" ] || bin="$("$go" env GOPATH)/bin"
[ -n "$win" ] && bin=$(cygpath -u "$bin")

# Use any ccxray already on PATH or already built; otherwise build this one.
ccxray=$(command -v ccxray 2>/dev/null) installed=
if [ -z "$ccxray" ] && [ -x "$bin/ccxray$exe" ]; then ccxray="$bin/ccxray$exe"; fi
if [ -z "$ccxray" ]; then
  if ! out=$(cd "$root" && "$go" install ./cmd/ccxray 2>&1); then
    echo "error"
    echo "go install ./cmd/ccxray failed:"
    echo "$out"
    exit 0
  fi
  ccxray="$bin/ccxray$exe" installed=1
fi

if [ -n "$stale" ]; then
  echo "restart"
  echo "Go is installed at $(native "$go"), but this Claude Code session started before it was, so its shell cannot find go, and the demo runs go build and go test."
  [ -n "$installed" ] && echo "ccxray has been installed to $(native "$ccxray")."
  echo "Quit Claude Code and start it again from a new terminal window (in the desktop app, quit and reopen the app), then rerun /ccxray-demo."
  exit 0
fi

if running; then
  echo "ready"
  echo "A ccxray process is running. If its panel still says Waiting, it was started for another project: restart it in $(native "$here"), or with --all."
  exit 0
fi

# The command as the user will type it: bare if the binary's folder is on
# PATH, which a terminal opened now will share, otherwise by full path. On
# Windows it is written for PowerShell, which Windows Terminal opens.
case ":$PATH:" in
  *":$(dirname "$ccxray"):"*) run=ccxray ;;
  *) if [ -n "$win" ]; then run="& '$(native "$ccxray")'"; else run="'$ccxray'"; fi ;;
esac
if [ -n "$win" ]; then then_=";"; else then_=" &&"; fi
echo "start"
[ -n "$installed" ] && echo "Installed ccxray from this checkout to $(native "$ccxray")."
echo "ccxray is not running. Start it in a second terminal:"
echo "cd '$(native "$here")'$then_ $run"
echo "Or from any folder: $run --all"
exit 0
