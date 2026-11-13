// Logic of the Home Assistant plugin, without build tags so it is testable natively.
package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// registryTemplate asks Home Assistant for all devices with an area in one
// REST call (POST /api/template); the device registry is otherwise only
// reachable over the WebSocket API.
const registryTemplate = `{% set ns = namespace(out=[]) %}{% for a in areas() %}{% for d in area_devices(a) %}` +
	`{% set ns.out = ns.out + [{'area': area_name(a), 'id': d, 'name': device_attr(d, 'name_by_user') or device_attr(d, 'name'),` +
	` 'manufacturer': device_attr(d, 'manufacturer'), 'model': device_attr(d, 'model')}] %}{% endfor %}{% endfor %}{{ ns.out | tojson }}`

// Device is one device of the registry with its area.
type Device struct {
	Area         string  `json:"area"`
	ID           string  `json:"id"`
	Name         *string `json:"name"`
	Manufacturer *string `json:"manufacturer"`
	Model        *string `json:"model"`
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slugify makes a Phoedox slug: lowercase, a-z0-9 and dashes.
func slugify(s string) string {
	s = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss").Replace(strings.ToLower(s))
	s = strings.Trim(nonSlug.ReplaceAllString(s, "-"), "-")
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-")
	}
	return s
}

// parseDevices reads the template answer.
func parseDevices(body []byte) ([]Device, error) {
	var list []Device
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("unexpected answer from Home Assistant: %w", err)
	}
	return list, nil
}

// graphOps turns areas and devices into rooms, devices and located_in edges.
// Slugs start with "ha-" so imports do not take over hand-made entries.
func graphOps(devices []Device) pdk.GraphOps {
	var ops pdk.GraphOps
	rooms := map[string]string{}
	for _, d := range devices {
		if d.Area == "" || d.ID == "" {
			continue
		}
		room, ok := rooms[d.Area]
		if !ok {
			room = ops.UpsertNamed("room", "ha-"+slugify(d.Area), d.Area, map[string]any{})
			rooms[d.Area] = room
		}
		name := str(d.Name)
		if name == "" {
			name = d.ID
		}
		dev := ops.UpsertNamed("device", "ha-"+slugify(name)+"-"+strings.ToLower(d.ID[:min(6, len(d.ID))]), name, map[string]any{
			"manufacturer": str(d.Manufacturer), "model": str(d.Model), "ha_device_id": d.ID,
		})
		ops.Link(dev, "located_in", room, nil)
	}
	return ops
}

var entityRe = regexp.MustCompile(`^[a-z_]+\.[a-z0-9_]+$`)

// State is what ha.state returns per entity.
type State struct {
	EntityID   string         `json:"entity_id"`
	State      string         `json:"state"`
	Attributes map[string]any `json:"attributes,omitempty"`
	Changed    string         `json:"last_changed,omitempty"`
	Error      string         `json:"error,omitempty"`
}

func validEntities(list []string) ([]string, error) {
	if len(list) == 0 || len(list) > 20 {
		return nil, fmt.Errorf("give 1 to 20 entity ids")
	}
	for _, e := range list {
		if !entityRe.MatchString(e) {
			return nil, fmt.Errorf("%q is not an entity id like light.flur", e)
		}
	}
	return list, nil
}
