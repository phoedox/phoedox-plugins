//go:build wasip1

package integ

import "github.com/phoedox/phoedox-sdk/go/pdk"

// GraphMatcher indexes the entries of the project. The host returns at most 500
// entries per query; when that limit is reached, further names are looked
// up on demand. Matching is best effort and never fails a tool call.
func GraphMatcher() *Matcher {
	list, err := pdk.Graph.Query(pdk.Query{Limit: 500})
	if err != nil {
		pdk.Warn("cannot read entries for matching: " + err.Error())
		return NewMatcher(nil)
	}
	m := NewMatcher(list)
	if len(list) >= 500 {
		m.Lookup = func(q string) []pdk.Entity {
			l, _ := pdk.Graph.Query(pdk.Query{Q: q, Limit: 20})
			return l
		}
	}
	return m
}
