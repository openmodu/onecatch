package desktop

import (
	"testing"

	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func TestFileChangesSurviveStoredEventFold(t *testing.T) {
	change := agentrun.NewFileChange("a.go", "update", "b.go", "@@ -1 +1 @@\n-old\n+new")
	event := agentrun.Event{Kind: agentrun.KindFileChange, Text: change.Path, FileChanges: []agentrun.FileChange{change}, Failed: true}
	views, _ := foldRuntimeEventViews("step", []domainworkflows.RuntimeEvent{runtimeEvent(t, 1, event)}, false)
	if len(views) != 1 || !views[0].Failed || len(views[0].FileChanges) != 1 || views[0].FileChanges[0] != change {
		t.Fatalf("stored edit lost its diff or state: %+v", views)
	}
}

func TestFileChangesRecoverOlderRuntimePayload(t *testing.T) {
	for _, raw := range []string{
		`{"item":{"changes":[{"path":"a.go","kind":{"type":"update"},"diff":"@@ -1 +1 @@\n-old\n+new"}]}}`,
		`{"params":{"item":{"changes":[{"path":"a.go","kind":{"type":"update"},"diff":"@@ -1 +1 @@\n-old\n+new"}]}}}`,
	} {
		event := agentrun.Event{Kind: agentrun.KindFileChange, Text: "a.go", Raw: raw}
		views, _ := foldRuntimeEventViews("step", []domainworkflows.RuntimeEvent{runtimeEvent(t, 1, event)}, false)
		if len(views) != 1 || len(views[0].FileChanges) != 1 || views[0].FileChanges[0].Diff != "@@ -1 +1 @@\n-old\n+new" {
			t.Fatalf("old runtime payload lost diff: %+v", views)
		}
	}
}
