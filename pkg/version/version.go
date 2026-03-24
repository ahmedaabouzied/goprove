package version

import (
	"fmt"
	"runtime/debug"
)

var Version string // LDFlag target
var Commit string  // LDFlag target
var Date string    // LDFlag target

func Info() string {
	if Version != "" {
		return fmt.Sprintf("goprove %s (%s) built %s", Version, Commit, Date)
	}

	// Dev build. Version is empty
	buildInfo, ok := debug.ReadBuildInfo()
	if ok {
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
