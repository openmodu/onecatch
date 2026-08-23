package harnesses

import "testing"

func TestCatalogEntriesAreComplete(t *testing.T) {
	seen := make(map[string]struct{})
	for _, harness := range Catalog() {
		if harness.ID == "" || harness.Name == "" || harness.Command == "" {
			t.Fatalf("incomplete catalog entry: %+v", harness)
		}
		if _, ok := seen[harness.ID]; ok {
			t.Fatalf("duplicate harness id %q", harness.ID)
		}
		seen[harness.ID] = struct{}{}
		// Every harness has to be reachable somehow, and the first integration
		// is what an unconfigured harness runs through.
		if len(harness.Integrations) == 0 {
			t.Fatalf("harness %q declares no integration", harness.ID)
		}
		if !harness.SupportsIntegration(harness.DefaultIntegration()) {
			t.Fatalf("harness %q cannot run its own default integration", harness.ID)
		}
	}
}

func TestCatalogIsNotMutableThroughItsAccessors(t *testing.T) {
	first := Catalog()
	first[0] = Harness{ID: "tampered"}
	if Catalog()[0].ID == "tampered" {
		t.Fatal("Catalog returned the shared slice rather than a copy")
	}
	if IDs()[0] == "tampered" {
		t.Fatal("IDs reflected a mutated copy")
	}
}

func TestSupportsEffortAndProvider(t *testing.T) {
	grok, ok := Find("grok")
	if !ok {
		t.Fatal("grok is missing from the catalog")
	}
	// Empty always passes: it means "leave the harness's own default".
	if !grok.SupportsEffort("") || !grok.SupportsProvider("") {
		t.Fatal("an unset effort or provider must be accepted")
	}
	if !grok.SupportsEffort("xhigh") || grok.SupportsEffort("max") {
		t.Fatalf("grok efforts = %v", grok.Efforts)
	}
	// Grok has a fixed provider, so naming one is a configuration error rather
	// than a setting that is quietly ignored.
	if grok.SupportsProvider("xai") {
		t.Fatal("a harness with no provider vocabulary must reject one")
	}

	dsh, _ := Find("dsh")
	if dsh.SupportsEffort("high") {
		t.Fatal("dsh exposes no reasoning control and must reject an effort")
	}
	if !dsh.SupportsProvider("pi-ai") || dsh.SupportsProvider("openai") {
		t.Fatalf("dsh providers = %v", dsh.Providers)
	}
}

// A harness that cannot resume must say so, because the product offers a
// continue action and has to know not to.
func TestResumeClaimsAreExplicit(t *testing.T) {
	dsh, _ := Find("dsh")
	if dsh.CanResume {
		t.Fatal("the DeepSeek Harness headless profile cannot resume")
	}
	for _, id := range []string{"codex", "claude", "modu", "pi", "grok"} {
		harness, _ := Find(id)
		if !harness.CanResume {
			t.Fatalf("harness %q is expected to resume", id)
		}
	}
}

func TestRemoteFSCapabilitiesAreExplicit(t *testing.T) {
	for _, id := range []string{"codex", "claude", "modu"} {
		harness, _ := Find(id)
		if !harness.SupportsRemoteFS {
			t.Fatalf("harness %q must support remote FS", id)
		}
	}
	for _, id := range []string{"pi", "grok", "dsh"} {
		harness, _ := Find(id)
		if harness.SupportsRemoteFS {
			t.Fatalf("harness %q must not advertise remote FS", id)
		}
	}
}

func TestFindAndIsKnown(t *testing.T) {
	if _, ok := Find("gemini"); ok {
		t.Fatal("an uncatalogued harness must not be found")
	}
	if IsKnown("gemini") {
		t.Fatal("an uncatalogued harness must not be known")
	}
	if len(IDs()) != len(Catalog()) {
		t.Fatal("IDs and Catalog disagree on how many harnesses exist")
	}
}
