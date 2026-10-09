package keeldesktop

import (
	"strings"
	"testing"
)

func TestSVGArcFlagExpansionPreservesCoordinates(t *testing.T) {
	for _, tc := range []struct{ Input, Want string }{
		{"M9.064 3.344a4.578 4.578 0 012.285-.312", "M 9.064 3.344 a 4.578 4.578 0 0 1 2.285 -.312"},
		{"M0,0 2,3 4,5z", "M 0 0 L 2 3 L 4 5 z"},
		{"m3 4a.637.637 0 000 1.272h3.636a.637.637 0 100-1.272z", "m 3 4 a .637 .637 0 0 0 0 1.272 h 3.636 a .637 .637 0 1 0 0 -1.272 z"},
		{"M1e-3 -2E+1L.5.7", "M 1e-3 -2E+1 L .5 .7"},
	} {
		got, err := expandSVGPath(tc.Input)
		if err != nil || strings.TrimSpace(got) != tc.Want {
			t.Errorf("%q: %q, %v; want %q", tc.Input, got, err, tc.Want)
		}
	}
	for _, bad := range []string{"M1", "L x 2", "M0 0 A1 1 0 2 0 1 1", "Z 4"} {
		if _, err := expandSVGPath(bad); err == nil {
			t.Errorf("invalid path accepted: %s", bad)
		}
	}
}
