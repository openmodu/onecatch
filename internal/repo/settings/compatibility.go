package settings

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/openmodu/onecatch/internal/domain/harnesses"
	domainsettings "github.com/openmodu/onecatch/internal/domain/settings"
)

// Decode only installed harnesses. A newer release may persist a runtime whose
// settings vocabulary (or even JSON shape) this binary does not understand.
// Its original JSON stays on disk and is preserved on subsequent saves.
func decodeCompatibleSettings(raw json.RawMessage) (domainsettings.Settings, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return domainsettings.Settings{}, err
	}
	if runtimes, ok := object["runtimes"]; ok {
		var entries map[string]json.RawMessage
		if err := json.Unmarshal(runtimes, &entries); err != nil {
			return domainsettings.Settings{}, err
		}
		for id := range entries {
			if _, known := harnesses.Find(id); !known {
				delete(entries, id)
			}
		}
		filtered, err := json.Marshal(entries)
		if err != nil {
			return domainsettings.Settings{}, err
		}
		object["runtimes"] = filtered
	}
	filtered, err := json.Marshal(object)
	if err != nil {
		return domainsettings.Settings{}, err
	}
	var value domainsettings.Settings
	err = json.Unmarshal(filtered, &value)
	return value, err
}

// Merge by the schema this binary owns, not by whichever fields happen to be
// nonempty. An omitted known field clears its old value; an unknown field is
// preserved verbatim, including large JSON numbers and nested future options.
func mergeCompatibleSettings(original json.RawMessage, value domainsettings.Settings) (json.RawMessage, error) {
	fresh, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return mergeKnownFields(original, fresh, reflect.TypeOf(value))
}

func mergeKnownFields(original, fresh json.RawMessage, typ reflect.Type) (json.RawMessage, error) {
	var oldObject, newObject map[string]json.RawMessage
	if typ.Kind() != reflect.Struct || json.Unmarshal(fresh, &newObject) != nil || newObject == nil {
		return fresh, nil
	}
	if json.Unmarshal(original, &oldObject) != nil || oldObject == nil {
		oldObject = map[string]json.RawMessage{}
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		next, present := newObject[name]
		if !present {
			delete(oldObject, name)
			continue
		}
		var err error
		if typ == reflect.TypeOf(domainsettings.Settings{}) && name == "runtimes" {
			next, err = mergeRuntimeSettings(oldObject[name], next)
		} else {
			next, err = mergeKnownFields(oldObject[name], next, field.Type)
		}
		if err != nil {
			return nil, err
		}
		oldObject[name] = next
	}
	return json.Marshal(oldObject)
}

func mergeRuntimeSettings(original, fresh json.RawMessage) (json.RawMessage, error) {
	oldEntries := map[string]json.RawMessage{}
	if len(original) > 0 && string(original) != "null" {
		if err := json.Unmarshal(original, &oldEntries); err != nil {
			return nil, err
		}
	}
	var newEntries map[string]json.RawMessage
	if err := json.Unmarshal(fresh, &newEntries); err != nil {
		return nil, err
	}
	for id, next := range newEntries {
		merged, err := mergeKnownFields(oldEntries[id], next, reflect.TypeOf(domainsettings.RuntimeSettings{}))
		if err != nil {
			return nil, err
		}
		oldEntries[id] = merged
	}
	return json.Marshal(oldEntries)
}
