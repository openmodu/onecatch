package desktop

import (
	"slices"
	"testing"
)

// Retaining a window means the application can be left holding webviews nobody
// can see. What the main window does as it closes hangs entirely on this split:
// release the invisible ones, and quit only if nothing is left on screen. Get it
// wrong in one direction and the application never exits; wrong in the other and
// it tears a settings panel out from under the user.
func TestHiddenRetainedWindowsSeparatesWhatTheUserCanSee(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		retained     []string
		visible      []string
		wantHidden   []string
		wantStillVis bool
	}{
		{
			name: "nothing retained",
		},
		{
			name:       "prewarmed but never opened",
			retained:   []string{settingsWindowName},
			wantHidden: []string{settingsWindowName},
		},
		{
			name:       "opened and closed again",
			retained:   []string{settingsWindowName, workflowsWindowName},
			visible:    []string{workflowsWindowName},
			wantHidden: []string{settingsWindowName},
			// The workflow window is still on screen, so the application must
			// outlive the workbench.
			wantStillVis: true,
		},
		{
			name:         "everything still on screen",
			retained:     []string{settingsWindowName, workflowsWindowName},
			visible:      []string{settingsWindowName, workflowsWindowName},
			wantStillVis: true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			controller := newAuxiliaryWindowController(nil)
			for _, name := range testCase.retained {
				controller.retained[name] = nil
			}
			for _, name := range testCase.visible {
				controller.visible[name] = true
			}

			hidden, stillVisible := controller.hiddenRetainedWindows()
			slices.Sort(hidden)
			want := slices.Clone(testCase.wantHidden)
			slices.Sort(want)
			if !slices.Equal(hidden, want) {
				t.Errorf("hidden = %v, want %v", hidden, want)
			}
			if stillVisible != testCase.wantStillVis {
				t.Errorf("stillVisible = %t, want %t", stillVisible, testCase.wantStillVis)
			}
		})
	}
}
