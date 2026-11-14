// Matching of monitored hosts to Phoedox entries (used by the monitoring
// plugins). No build tags, so it is testable natively.
package integ

import (
	"net/netip"
	"net/url"
	"strings"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// Entry is the Phoedox entry a monitored object belongs to.
type Entry struct {
	Slug string `json:"slug"`
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// addrAttrs are the attributes that hold a host name or an address.
var addrAttrs = []string{"hostname", "fqdn", "dns_name", "ip", "ipv4", "ipv6", "address", "host"}

// maxLookups bounds the extra graph queries per tool call.
const maxLookups = 25

// Matcher finds the entry for a host name, address or display name.
// Addresses (attrs) win over names, names over short host names; a short
// name shared by two entries matches neither.
type Matcher struct {
	byAddr  map[string]*Entry
	byName  map[string]*Entry
	byShort map[string]*Entry // nil value: ambiguous
	known   map[string]bool
	// Lookup, when set, searches the graph for a name (used when the first
	// query may not have returned every entry).
	Lookup  func(q string) []pdk.Entity
	lookups int
	asked   map[string]bool
}

// NewMatcher indexes entries.
func NewMatcher(list []pdk.Entity) *Matcher {
	m := &Matcher{byAddr: map[string]*Entry{}, byName: map[string]*Entry{}, byShort: map[string]*Entry{}, known: map[string]bool{}, asked: map[string]bool{}}
	for _, e := range list {
		m.add(e)
	}
	return m
}

func (m *Matcher) add(e pdk.Entity) {
	if e.Slug == "" || m.known[e.Slug] {
		return
	}
	m.known[e.Slug] = true
	ent := &Entry{Slug: e.Slug, Kind: e.Kind, Name: e.Name}
	put := func(idx map[string]*Entry, k string) {
		if _, ok := idx[k]; k != "" && !ok {
			idx[k] = ent
		}
	}
	short := func(k string) {
		if k == "" {
			return
		}
		if old, ok := m.byShort[k]; ok && (old == nil || old.Slug != ent.Slug) {
			m.byShort[k] = nil
			return
		}
		m.byShort[k] = ent
	}
	shorts := map[string]bool{}
	for _, a := range addrAttrs {
		for _, v := range strs(e.Attrs[a]) {
			h := normHost(v)
			put(m.byAddr, h)
			shorts[shortName(h)] = true
		}
	}
	n := normName(e.Name)
	put(m.byName, n)
	put(m.byName, "slug:"+e.Slug)
	if !strings.Contains(n, " ") {
		shorts[shortName(n)] = true
	}
	for s := range shorts {
		short(s)
	}
}

// Match returns the entry for the first candidate that matches, or nil.
func (m *Matcher) Match(candidates ...string) *Entry {
	if m == nil {
		return nil
	}
	if e := m.match(candidates); e != nil || m.Lookup == nil {
		return e
	}
	added := false
	for _, c := range candidates {
		q := shortName(normHost(c))
		if isIP(q) {
			q = normHost(c)
		}
		if len(q) < 2 || m.asked[q] || m.lookups >= maxLookups {
			continue
		}
		m.asked[q] = true
		m.lookups++
		for _, e := range m.Lookup(q) {
			m.add(e)
			added = true
		}
	}
	if !added {
		return nil
	}
	return m.match(candidates)
}

func (m *Matcher) match(candidates []string) *Entry {
	for _, c := range candidates {
		if e := m.byAddr[normHost(c)]; e != nil && normHost(c) != "" {
			return e
		}
	}
	for _, c := range candidates {
		n := normName(c)
		if n == "" {
			continue
		}
		if e := m.byName[n]; e != nil {
			return e
		}
		if e := m.byName["slug:"+Slugify(c)]; e != nil {
			return e
		}
	}
	for _, c := range candidates {
		h := normHost(c)
		if h == "" || isIP(h) || strings.Contains(h, " ") {
			continue
		}
		if e := m.byAddr[shortName(h)]; e != nil {
			return e
		}
		if e := m.byName[shortName(h)]; e != nil {
			return e
		}
		if e := m.byShort[shortName(h)]; e != nil {
			return e
		}
	}
	return nil
}

// strs reads a string or a list of strings.
func strs(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		var out []string
		for _, i := range x {
			if s, ok := i.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return x
	}
	return nil
}

func normName(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }

// normHost turns a URL, host:port, [v6]:port or address/mask into a lower
// case host name or address.
func normHost(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	if strings.Contains(s, "://") {
		if u, err := url.Parse(s); err == nil && u.Hostname() != "" {
			return strings.TrimSuffix(u.Hostname(), ".")
		}
	}
	if strings.HasPrefix(s, "[") {
		if i := strings.Index(s, "]"); i > 0 {
			s = s[1:i]
		}
	} else if strings.Count(s, ":") == 1 && !strings.Contains(s, " ") {
		s = s[:strings.Index(s, ":")]
	}
	if ip := StripMask(s); isIP(ip) {
		return ip
	}
	return strings.TrimSuffix(s, ".")
}

// shortName is the first label of a host name ("web01.example.org" → "web01").
func shortName(h string) string {
	if isIP(h) {
		return h
	}
	if i := strings.IndexByte(h, '.'); i > 0 {
		return h[:i]
	}
	return h
}

func isIP(s string) bool {
	_, err := netip.ParseAddr(s)
	return err == nil
}
