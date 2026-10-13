package keeldesktop

import (
	"testing"

	"github.com/dyike/keel/ui/el"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func TestTraeModelMenuShowsProviderCapacity(t *testing.T) {
	a := conversationFixture(t)
	a.task.Harness = "trae"
	a.runtime.SetValue("trae")
	a.profile.key = a.runtimeProfileKey()
	load, queue, zero := 264, int64(125), int64(0)
	a.profile.configuration = agentrun.HarnessConfiguration{Models: []agentrun.HarnessModel{
		{Model: "busy", DisplayName: "GPT-6.1-Sol", Load: &agentrun.HarnessModelLoad{Percent: &load, QueueSize: &queue}},
		{Model: "idle", DisplayName: "Seed-Code", Load: &agentrun.HarnessModelLoad{QueueSize: &zero}},
		{Model: "unknown", DisplayName: "Unknown"},
	}}
	a.buildRuntimeProfileMenu()
	a.profile.menu.SetValue(true)
	router, frame, _ := alignmentRouter(el.Root(a), 1120, 800, 1)
	frame()
	for _, label := range []string{"负载 264% · 排队 125 人", "暂无排队"} {
		if _, ok := alignmentNode(router.AppendSemantics(nil), label, false); !ok {
			t.Fatalf("model menu lost provider capacity: %s", label)
		}
	}
	if modelCapacityDescription(nil) != "" || modelCapacityDescription(&agentrun.HarnessModelLoad{}) != "" {
		t.Fatal("missing capacity became a reported zero")
	}
}
