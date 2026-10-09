package keeldesktop

import (
	"reflect"
	"testing"

	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

func reviewFixture() domainworkflows.Definition {
	return domainworkflows.Definition{ID: "loop", Name: "实现与审查", EntryStepID: "implement", Policy: domainworkflows.Policy{MaxTransitions: 20}, Steps: []domainworkflows.Step{{ID: "implement", Transitions: map[string]string{"ready_for_review": "review", "need_human": "$pause"}}, {ID: "review", Transitions: map[string]string{"changes_requested": "implement", "approved": "$done", "need_human": "$pause"}}}}
}

func TestGuidedWorkflowDoesNotHideCustomBranches(t *testing.T) {
	d := reviewFixture()
	s, ok := simpleReviewWorkflow(d)
	if !ok || s.Rounds != 10 || !s.Retry {
		t.Fatalf("simple: %+v %v", s, ok)
	}
	d.Steps[1].Transitions["custom"] = "$fail"
	if _, ok := simpleReviewWorkflow(d); ok {
		t.Fatal("guided form would hide custom branch")
	}
	before := domainworkflows.Normalize(d)
	if setReviewBehavior(&d, false, 3) {
		t.Fatal("custom graph was changed")
	}
	if !reflect.DeepEqual(before, domainworkflows.Normalize(d)) {
		t.Fatal("custom transition lost")
	}
}

func TestGuidedReviewPolicyRoundTrip(t *testing.T) {
	d := reviewFixture()
	d.Steps[0], d.Steps[1] = d.Steps[1], d.Steps[0]
	if !setReviewBehavior(&d, false, 3) {
		t.Fatal("could not configure pause")
	}
	s, ok := simpleReviewWorkflow(d)
	if !ok || s.Retry || s.Rounds != 3 || d.Policy.MaxTransitions != 6 || d.Steps[0].Transitions["changes_requested"] != "$pause" {
		t.Fatalf("pause rule: %+v", d)
	}
	if !setReviewBehavior(&d, true, 5) || d.Steps[0].Transitions["changes_requested"] != "implement" {
		t.Fatal("retry rule not restored")
	}
	if setReviewBehavior(&d, true, 0) || setReviewBehavior(&d, true, 51) {
		t.Fatal("out-of-range rounds accepted")
	}
	d.Policy.MaxTransitions = 5
	if _, ok := simpleReviewWorkflow(d); ok {
		t.Fatal("odd transition bound loses a turn in guided mode")
	}
}
