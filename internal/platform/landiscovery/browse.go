package landiscovery

import (
	"sort"
	"strings"
)

// Candidate is an untrusted LAN advertisement, not a paired identity. The
// one-time code exchange still authenticates the first connection.
type Candidate struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Addresses []string `json:"addresses"`
}

func candidate(instance, host string, addresses []string) Candidate {
	sort.Strings(addresses) // Prefer IPv4, while retaining IPv6 for manual fallback.
	name := strings.TrimSuffix(strings.TrimSuffix(host, "."), ".local")
	if name == "" {
		name = instance
	}
	return Candidate{ID: instance, Name: name, Addresses: addresses}
}

func sortedCandidates(items map[string]Candidate) []Candidate {
	result := make([]Candidate, 0, len(items))
	for _, item := range items {
		if len(item.Addresses) > 0 {
			result = append(result, item)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].ID < result[j].ID
		}
		return result[i].Name < result[j].Name
	})
	return result
}
