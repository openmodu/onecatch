package agentrun

import "time"

// fixedNow returns a deterministic clock so event timestamps do not make
// assertions depend on wall time.
func fixedNow() nowFunc {
	instant := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	return func() time.Time {
		instant = instant.Add(time.Millisecond)
		return instant
	}
}

// argValue returns the argument following flag, or empty when the flag is
// absent or ends the list.
func argValue(args []string, flag string) string {
	for index, arg := range args {
		if arg == flag && index+1 < len(args) {
			return args[index+1]
		}
	}
	return ""
}

// containsArgs reports whether every wanted flag appears in args.
func containsArgs(args []string, wanted ...string) bool {
	present := make(map[string]struct{}, len(args))
	for _, arg := range args {
		present[arg] = struct{}{}
	}
	for _, want := range wanted {
		if _, ok := present[want]; !ok {
			return false
		}
	}
	return true
}
