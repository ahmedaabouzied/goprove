package updater

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSemver(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  semver
		ok    bool
	}{
		{"v0.1.0", semver{0, 1, 0}, true},
		{"v1.2.3", semver{1, 2, 3}, true},
		{"v10.20.30", semver{10, 20, 30}, true},
		{"v0.0.0", semver{0, 0, 0}, true},
		{"0.1.0", semver{}, false},    // missing v prefix
		{"dev", semver{}, false},      // not semver
		{"", semver{}, false},         // empty
		{"v1.2", semver{}, false},     // incomplete
		{"v1", semver{}, false},       // incomplete
		{"vabc", semver{}, false},     // not numbers
		{"v1.2.x", semver{}, false},   // non-numeric patch
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got, ok := parseSemver(tt.input)
			require.Equal(t, tt.ok, ok, "parseSemver(%q) ok", tt.input)
			if ok {
				require.Equal(t, tt.want, got, "parseSemver(%q) value", tt.input)
			}
		})
	}
}

func TestSemver_LessThan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		a, b semver
		want bool
	}{
		{"major less", semver{0, 9, 9}, semver{1, 0, 0}, true},
		{"major greater", semver{2, 0, 0}, semver{1, 9, 9}, false},
		{"minor less", semver{1, 0, 9}, semver{1, 1, 0}, true},
		{"minor greater", semver{1, 2, 0}, semver{1, 1, 9}, false},
		{"patch less", semver{1, 2, 3}, semver{1, 2, 4}, true},
		{"patch greater", semver{1, 2, 4}, semver{1, 2, 3}, false},
		{"equal", semver{1, 2, 3}, semver{1, 2, 3}, false},
		{"zero", semver{0, 0, 0}, semver{0, 0, 1}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, tt.a.lessThan(tt.b))
		})
	}
}

func TestIsNewerVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		current, latest string
		want            bool
	}{
		// Upgrade available
		{"patch bump", "v0.1.0", "v0.1.1", true},
		{"minor bump", "v0.1.0", "v0.2.0", true},
		{"major bump", "v0.1.0", "v1.0.0", true},
		{"multi-digit", "v0.9.0", "v0.10.0", true},

		// Up to date or ahead
		{"same version", "v0.2.0", "v0.2.0", false},
		{"current is newer", "v0.3.0", "v0.2.0", false},
		{"current major ahead", "v2.0.0", "v1.9.9", false},

		// Invalid versions — never nag
		{"dev build", "dev", "v0.1.0", false},
		{"empty current", "", "v0.1.0", false},
		{"empty latest", "v0.1.0", "", false},
		{"both empty", "", "", false},
		{"current no prefix", "0.1.0", "v0.2.0", false},
		{"latest no prefix", "v0.1.0", "0.2.0", false},
		{"current garbage", "vx.y.z", "v0.2.0", false},
		{"latest garbage", "v0.1.0", "vx.y.z", false},
		{"unknown current", "goprove dev (unknown)", "v0.1.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, IsNewerVersion(tt.current, tt.latest),
				"IsNewerVersion(%q, %q)", tt.current, tt.latest)
		})
	}
}
