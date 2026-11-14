// Logic of the Tailscale plugin, without build tags so it is testable
// natively: parsing, graph ops, hostname matching and the device list.
package main

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// hostKinds are the kinds a Tailscale node may be linked to.
var hostKinds = []string{"server", "vm", "lxc", "device"}

const (
	queryLimit = 500 // most entries the host returns per query
	maxOps     = 500 // most ops the host accepts per Apply
)

var tailnetRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@+-]{0,199}$|^-$`)

// tailnetSetting checks the tailnet setting; empty means "-" (the tailnet of
// the key).
func tailnetSetting(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "-", nil
	}
	if !tailnetRe.MatchString(s) || strings.Contains(s, "..") {
		return "", fmt.Errorf("the setting tailnet must be a tailnet ID or organization name, or - for the tailnet of the API key")
	}
	return s, nil
}

// httpError turns a non-200 answer into a clear message.
func httpError(status int) error {
	switch status {
	case 401:
		return fmt.Errorf("Tailscale rejected the API key (wrong, expired or revoked)")
	case 403:
		return fmt.Errorf("the Tailscale API key may not read the devices of this tailnet")
	case 404:
		return fmt.Errorf("Tailscale does not know this tailnet (check the setting tailnet, or use -)")
	case 429:
		return fmt.Errorf("Tailscale rate-limited the request; try again later")
	}
	return &integ.StatusError{Service: "Tailscale", Status: status}
}

// Device is a device of GET /api/v2/tailnet/{tailnet}/devices. Keys
// (machineKey, nodeKey) are not read.
type Device struct {
	ID                 string   `json:"id"`
	NodeID             string   `json:"nodeId"`
	Name               string   `json:"name"` // MagicDNS name, e.g. web01.tail1234.ts.net
	Hostname           string   `json:"hostname"`
	User               string   `json:"user"`
	OS                 string   `json:"os"`
	Addresses          []string `json:"addresses"`
	ClientVersion      string   `json:"clientVersion"`
	UpdateAvailable    bool     `json:"updateAvailable"`
	LastSeen           string   `json:"lastSeen"`
	Expires            string   `json:"expires"`
	KeyExpiryDisabled  bool     `json:"keyExpiryDisabled"`
	Authorized         bool     `json:"authorized"`
	IsExternal         bool     `json:"isExternal"`
	Tags               []string `json:"tags"`
	ConnectedToControl *bool    `json:"connectedToControl"`
}

func parseDevices(body []byte) ([]Device, error) {
	var r struct {
		Devices []Device `json:"devices"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("unexpected answer from Tailscale: %w", err)
	}
	return r.Devices, nil
}

// host is the device's hostname, else the first label of its MagicDNS name.
func (d Device) host() string {
	if h := strings.TrimSpace(d.Hostname); h != "" {
		return h
	}
	h, _, _ := strings.Cut(d.Name, ".")
	return h
}

// key is a stable ID: the device ID, else the node ID.
func (d Device) key() string {
	if d.ID != "" {
		return d.ID
	}
	return d.NodeID
}

// tailscaleIP is the first Tailscale IPv4 (100.64.0.0/10), else the first address.
func (d Device) tailscaleIP() string {
	cgnat := netip.MustParsePrefix("100.64.0.0/10")
	for _, a := range d.Addresses {
		if ip, err := netip.ParseAddr(a); err == nil && cgnat.Contains(ip) {
			return a
		}
	}
	if len(d.Addresses) > 0 {
		return d.Addresses[0]
	}
	return ""
}

// version drops the build suffix: "1.76.6-t2b6b5d7a9-g7f5f3d4b3" -> "1.76.6".
func version(v string) string {
	s, _, _ := strings.Cut(v, "-")
	return s
}

// hostEntries keeps the entries of the kinds a node may be linked to.
func hostEntries(list []pdk.Entity) []pdk.Entity {
	var out []pdk.Entity
	for _, e := range list {
		if slices.Contains(hostKinds, e.Kind) {
			out = append(out, e)
		}
	}
	return out
}

// newMatcher indexes the host entries for match.
func newMatcher(list []pdk.Entity) *integ.Matcher { return integ.NewMatcher(hostEntries(list)) }

// match finds the entry of a host kind for the device: its Tailscale
// addresses or its hostname against attrs (hostname, fqdn, ip, …) and names,
// case-insensitive, short names too (web01 = web01.lan). Ambiguous short
// names are not linked.
func match(d Device, m *integ.Matcher) (string, bool) {
	h := strings.ToLower(strings.TrimSpace(d.host()))
	if h == "" || h == "localhost" {
		return "", false
	}
	cands := append([]string{d.host()}, d.Addresses...)
	e := m.Match(cands...)
	if e == nil || !slices.Contains(hostKinds, e.Kind) {
		return "", false
	}
	return e.Slug, true
}

// graphOps: one tailscale_node per own device, linked related_to the matching
// host entry. Devices shared in from other tailnets are left out. It also
// returns the number of links.
func graphOps(devs []Device, m *integ.Matcher) (pdk.GraphOps, int) {
	var ops pdk.GraphOps
	links := 0
	for _, d := range devs {
		if d.IsExternal || d.key() == "" {
			continue
		}
		host := d.host()
		attrs := map[string]any{"hostname": host, "os": d.OS, "authorized": d.Authorized, "update_available": d.UpdateAvailable}
		if d.Name != "" {
			attrs["dns_name"] = strings.TrimSuffix(d.Name, ".")
		}
		if len(d.Addresses) > 0 {
			attrs["addresses"] = strings.Join(d.Addresses, ", ")
			attrs["tailscale_ip"] = d.tailscaleIP()
		}
		if v := version(d.ClientVersion); v != "" {
			attrs["client_version"] = v
		}
		if len(d.LastSeen) >= 10 {
			attrs["last_seen"] = d.LastSeen[:10]
		}
		if len(d.Tags) > 0 {
			attrs["tags"] = strings.Join(d.Tags, ", ")
		}
		name := host
		if name == "" {
			name = d.key()
		}
		ref := ops.UpsertNamed("tailscale_node", "ts-"+integ.Slugify(d.key()), name, attrs)
		if to, ok := match(d, m); ok {
			ops.Link(ref, "related_to", to, nil)
			links++
		}
	}
	return ops, links
}

// chunk splits ops into batches of at most max ops; a link stays in the batch
// of the upsert it refers to.
func chunk(ops pdk.GraphOps, max int) []pdk.GraphOps {
	var groups [][]pdk.GraphOp
	for _, o := range ops.Ops {
		if o.Op == "upsert" || len(groups) == 0 {
			groups = append(groups, nil)
		}
		groups[len(groups)-1] = append(groups[len(groups)-1], o)
	}
	var out []pdk.GraphOps
	var cur pdk.GraphOps
	for _, g := range groups {
		if len(cur.Ops)+len(g) > max && len(cur.Ops) > 0 {
			out = append(out, cur)
			cur = pdk.GraphOps{}
		}
		cur.Ops = append(cur.Ops, g...)
	}
	if len(cur.Ops) > 0 {
		out = append(out, cur)
	}
	return out
}

// Info is a device as tailscale.devices shows it.
type Info struct {
	Name              string   `json:"name"`
	DNSName           string   `json:"dns_name,omitempty"`
	User              string   `json:"user,omitempty"`
	OS                string   `json:"os,omitempty"`
	Online            bool     `json:"online"`
	LastSeen          string   `json:"last_seen,omitempty"`
	TailscaleIP       string   `json:"tailscale_ip,omitempty"`
	Addresses         []string `json:"addresses,omitempty"`
	ClientVersion     string   `json:"client_version,omitempty"`
	UpdateAvailable   bool     `json:"update_available"`
	KeyExpiry         string   `json:"key_expiry,omitempty"`
	KeyExpiryDisabled bool     `json:"key_expiry_disabled,omitempty"`
	KeyExpired        bool     `json:"key_expired,omitempty"`
	Authorized        bool     `json:"authorized"`
	Tags              []string `json:"tags,omitempty"`
	Shared            bool     `json:"shared_in,omitempty"`
}

// online uses connectedToControl; older answers without it count a device
// as online if it was seen in the last five minutes.
func (d Device) online(now time.Time) bool {
	if d.ConnectedToControl != nil {
		return *d.ConnectedToControl
	}
	t, err := time.Parse(time.RFC3339, d.LastSeen)
	return err == nil && now.Sub(t) < 5*time.Minute
}

// deviceInfo lists the devices, online ones first, then by name.
func deviceInfo(devs []Device, now time.Time) []Info {
	out := make([]Info, 0, len(devs))
	for _, d := range devs {
		in := Info{Name: d.host(), DNSName: strings.TrimSuffix(d.Name, "."), User: d.User, OS: d.OS, Online: d.online(now), LastSeen: d.LastSeen,
			TailscaleIP: d.tailscaleIP(), Addresses: d.Addresses, ClientVersion: version(d.ClientVersion), UpdateAvailable: d.UpdateAvailable,
			KeyExpiryDisabled: d.KeyExpiryDisabled, Authorized: d.Authorized, Tags: d.Tags, Shared: d.IsExternal}
		if !d.KeyExpiryDisabled && d.Expires != "" {
			if t, err := time.Parse(time.RFC3339, d.Expires); err == nil && t.Year() > 1 {
				in.KeyExpiry = d.Expires
				in.KeyExpired = !t.After(now)
			}
		}
		out = append(out, in)
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Online != out[b].Online {
			return out[a].Online
		}
		return strings.ToLower(out[a].Name) < strings.ToLower(out[b].Name)
	})
	return out
}
