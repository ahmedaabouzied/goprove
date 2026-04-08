package version

import (
	"fmt"
	"runtime/debug"
)

// LDFlag target
var Commit string

// LDFlag target
var Date string

var Version string // LDFlag target

func Info() string {
	if Version != "" {
		if Commit != "" {
			return fmt.Sprintf("goprove %s (%s) built %s", Version, Commit, Date)
		}
		return fmt.Sprintf("goprove %s", Version)
	}

	// Dev build — no ldflags and no module version.
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		commit := ""
		dirty := false
		suffix := ""
		for _, s := range buildInfo.Settings {
			switch s.Key {
			case "vcs.revision":
				commit = s.Value
			case "vcs.modified":
				dirty = s.Value == "true"
			}
		}
		if dirty {
			suffix = "-dirty"
		}
		if len(commit) > 7 {
			commit = commit[:7]
		}
		return fmt.Sprintf("goprove dev (%s%s)", commit, suffix)
	}

	return "goprove dev (unknown)"
}

// LDFlag target
func init() {
	if Version != "" {
		return
	}
	// go install ...@v0.2.0 sets Main.Version to "v0.2.0".
	// go install ...@latest also sets it to the resolved version.
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		if v := buildInfo.Main.Version; v != "" && v != "(devel)" {
			Version = v
		}
	}
}
