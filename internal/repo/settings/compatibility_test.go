package settings

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	domainsettings "github.com/openmodu/onecatch/internal/domain/settings"
	"github.com/openmodu/onecatch/pkg/localfile"
)

func TestNewerSettingsSurviveOlderReaderSave(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "settings.json")
	value := domainsettings.Defaults()
	codex := value.Runtimes["codex"]
	codex.Binary = "/old/binary"
	codex.MaxContextWindow = true
	value.Runtimes["codex"] = codex
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	document["futureSection"] = json.RawMessage(`{"exactCounter":9007199254740993,"enabled":false}`)
	var runtimes map[string]json.RawMessage
	_ = json.Unmarshal(document["runtimes"], &runtimes)
	// Deliberately incompatible with RuntimeSettings: unknown entries must be
	// opaque rather than decoded/validated using the current adapter schema.
	runtimes["future-harness"] = json.RawMessage(`{"enabled":"new-mode","options":[1,2,3]}`)
	var known map[string]json.RawMessage
	_ = json.Unmarshal(runtimes["codex"], &known)
	known["futureOption"] = json.RawMessage(`{"mode":"future","exactCounter":9007199254740993}`)
	runtimes["codex"], _ = json.Marshal(known)
	document["runtimes"], _ = json.Marshal(runtimes)
	var terminal map[string]json.RawMessage
	_ = json.Unmarshal(document["terminal"], &terminal)
	terminal["futureFont"] = json.RawMessage(`{"size":13}`)
	document["terminal"], _ = json.Marshal(terminal)
	if err = localfile.WriteJSONAtomic(path, document); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	repo := NewSettingsRepo(root)
	loaded, err := repo.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, present := loaded.Runtimes["future-harness"]; present {
		t.Fatal("unknown runtime leaked to execution/UI")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("read rewrote newer data")
	}
	codex = loaded.Runtimes["codex"]
	codex.Binary = ""
	codex.MaxContextWindow = false
	loaded.Runtimes["codex"] = codex
	loaded.Terminal.Theme = "paper"
	if _, err = repo.Save(context.Background(), loaded, loaded.Revision); err != nil {
		t.Fatal(err)
	}
	var saved map[string]json.RawMessage
	if err = localfile.ReadJSON(path, &saved); err != nil {
		t.Fatal(err)
	}
	var gotRuntimes map[string]json.RawMessage
	_ = json.Unmarshal(saved["runtimes"], &gotRuntimes)
	var gotKnown map[string]json.RawMessage
	_ = json.Unmarshal(gotRuntimes["codex"], &gotKnown)
	var gotTerminal map[string]json.RawMessage
	_ = json.Unmarshal(saved["terminal"], &gotTerminal)
	for name, pair := range map[string][2]json.RawMessage{
		"top-level": {saved["futureSection"], document["futureSection"]},
		"runtime":   {gotRuntimes["future-harness"], runtimes["future-harness"]},
		"nested":    {gotKnown["futureOption"], known["futureOption"]},
		"terminal":  {gotTerminal["futureFont"], terminal["futureFont"]},
	} {
		// Unmarshal into RawMessage maps above preserves exact numeric spelling.
		var compact [2]string
		for i, v := range pair {
			b, err := json.Marshal(json.RawMessage(v))
			if err != nil {
				t.Fatal(err)
			}
			compact[i] = string(b)
		}
		if compact[0] != compact[1] {
			t.Fatalf("lost %s: %s != %s", name, compact[0], compact[1])
		}
	}
	if _, present := gotKnown["binary"]; present {
		t.Fatal("cleared binary restored by merge")
	}
	if _, present := gotKnown["maxContextWindow"]; present {
		t.Fatal("false optional bool restored by merge")
	}
	reopened, err := NewSettingsRepo(root).Get(context.Background())
	if err != nil || reopened.Terminal.Theme != "paper" || reopened.Revision != loaded.Revision+1 {
		t.Fatalf("reopen: %+v %v", reopened, err)
	}
}

func TestIncompatibleSchemaAndInvalidKnownSettingsStayProtected(t *testing.T) {
	for _, futureSchema := range []bool{false, true} {
		root := t.TempDir()
		path := filepath.Join(root, "settings.json")
		value := domainsettings.Defaults()
		if futureSchema {
			value.SchemaVersion++
		} else {
			value.Terminal.Theme = "invalid-theme"
		}
		if err := localfile.WriteJSONAtomic(path, value); err != nil {
			t.Fatal(err)
		}
		before, _ := os.ReadFile(path)
		repo := NewSettingsRepo(root)
		if _, err := repo.Get(context.Background()); err == nil {
			t.Fatal("incompatible settings silently accepted")
		}
		if _, err := repo.Save(context.Background(), domainsettings.Defaults(), value.Revision); err == nil {
			t.Fatal("incompatible settings overwritten")
		}
		after, _ := os.ReadFile(path)
		if string(before) != string(after) {
			t.Fatal("failed read/save changed source")
		}
	}
}
