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

	// Dev build or go install. Version is empty.
	buildInfo, ok := debug.ReadBuildInfo()
	if ok {
		// go install ...@v0.2.0 sets Main.Version to "v0.2.0".
		// go install ...@latest also sets it to the resolved version.
		if v := buildInfo.Main.Version; v != "" && v != "(devel)" {
			Version = v // Populate so upgrade check works too.
			return fmt.Sprintf("goprove %s", v)
		}

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
