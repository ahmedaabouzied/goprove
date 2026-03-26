package updater

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/ahmedaabouzied/goprove/pkg/version"
)

type semver struct {
	major, minor, patch int
}

func (a semver) lessThan(b semver) bool {
	if a.major != b.major {
		return a.major < b.major
	}
	if a.minor != b.minor {
		return a.minor < b.minor
	}
	return a.patch < b.patch
}

func parseSemver(s string) (semver, bool) {
	s, found := strings.CutPrefix(s, "v")
	if !found {
		return semver{}, false
	}
	var v semver
	n, err := fmt.Sscanf(s, "%d.%d.%d", &v.major, &v.minor, &v.patch)
	if err != nil || n != 3 {
		return semver{}, false
	}
	return v, true
}

// IsNewerVersion returns true if latest is a newer semver than current.
// Returns false if either string is not a valid semver (e.g., "dev", "").
func IsNewerVersion(current, latest string) bool {
	cur, ok := parseSemver(current)
	if !ok {
		return false
	}
	lat, ok := parseSemver(latest)
	if !ok {
		return false
	}
	return cur.lessThan(lat)
}

func CheckForUpdates() string {
	currentVersion := version.Version
	wg := sync.WaitGroup{}
	defer wg.Wait()

	entry, err := ReadCache()
	if err != nil {
		// No cache. First run. Trigger background fetch
		wg.Go(backgroundFetch)
		return ""
	}
	if IsStale(entry, 12*time.Hour) {
		wg.Go(backgroundFetch)
	}
	if IsNewerVersion(currentVersion, entry.LatestVersion) {
		return entry.LatestVersion
	}
	return ""
}

func Upgrade() error {
	latest, err := FetchLatestVersion()
	if err != nil {
		return fmt.Errorf("failed to check latest version: %w", err)
	}
	if !IsNewerVersion(version.Version, latest) {
		fmt.Fprintf(os.Stderr, "Already up to date (%s)\n", version.Version)
		return nil
	}
	fmt.Fprintf(os.Stderr, "Upgrading goprove %s → %s\n", version.Version, latest)
	cmd := exec.Command("go", "install", "github.com/ahmedaabouzied/goprove/cmd/goprove@latest")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("upgrade failed: %w", err)
	}
	// Update cache so the "new version available" notification goes away.
	_ = WriteCache(CacheEntry{LatestVersion: latest, CheckedAt: time.Now()})
	fmt.Fprintf(os.Stderr, "Upgraded to %s\n", latest)
	return nil
}

func backgroundFetch() {
	latest, err := FetchLatestVersion()
	if err != nil {
		return
	}
	_ = WriteCache(CacheEntry{LatestVersion: latest, CheckedAt: time.Now()})
}
