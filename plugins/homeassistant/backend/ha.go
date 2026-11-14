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
	` 'manufacturer': device_attr(d, 'manufacturer'), 'model': device_attr(d, 'model'), 'entities': device_entities(d) | list}] %}{% endfor %}{% endfor %}{{ ns.out | tojson }}`

// Device is one device of the registry with its area.
type Device struct {
	Area         string   `json:"area"`
	ID           string   `json:"id"`
	Name         *string  `json:"name"`
	Manufacturer *string  `json:"manufacturer"`
	Model        *string  `json:"model"`
	Entities     []string `json:"entities"`
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
		attrs := map[string]any{"manufacturer": str(d.Manufacturer), "model": str(d.Model), "ha_device_id": d.ID}
		if len(d.Entities) > 0 {
			attrs["ha_entities"] = strings.Join(d.Entities, ", ")
		}
		if e := primaryEntity(d.Entities); e != "" {
			attrs["ha_entity"] = e
		}
		dev := ops.UpsertNamed(kindFor(primaryEntity(d.Entities)), "ha-"+slugify(name)+"-"+strings.ToLower(d.ID[:min(6, len(d.ID))]), name, attrs)
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

// switchable are the domains Phoedox may switch, in order of preference.
var switchable = []string{"switch", "light", "fan", "input_boolean"}

// primaryEntity picks the entity of a device that switches it on and off.
func primaryEntity(ids []string) string {
	for _, domain := range switchable {
		for _, id := range ids {
			if strings.HasPrefix(id, domain+".") && entityRe.MatchString(id) {
				return id
			}
		}
	}
	return ""
}

// Switchable reports whether Phoedox may switch the entity.
func Switchable(id string) bool {
	return entityRe.MatchString(id) && primaryEntity([]string{id}) == id
}

// CallArgs are the arguments of ha.call.
type CallArgs struct {
	EntityID string `json:"entity_id"`
	Action   string `json:"action"` // on, off, toggle
}

// service returns the Home Assistant service for a switch action.
func (c CallArgs) service() (domain, service string, err error) {
	if !Switchable(c.EntityID) {
		return "", "", fmt.Errorf("%q is not a switch, light, fan or input_boolean", c.EntityID)
	}
	svc := map[string]string{"on": "turn_on", "off": "turn_off", "toggle": "toggle"}[c.Action]
	if svc == "" {
		return "", "", fmt.Errorf("action must be on, off or toggle")
	}
	return strings.SplitN(c.EntityID, ".", 2)[0], svc, nil
}

// RestartArgs are the arguments of ha.restart.
type RestartArgs struct {
	EntityID string `json:"entity_id"`
	Seconds  int    `json:"seconds"`
}

// pause is how long the power stays off (default 10 s, 3 to 30).
func (r RestartArgs) pause() int {
	if r.Seconds == 0 {
		return 10
	}
	return min(max(r.Seconds, 3), 30)
}

// Control is one switchable thing shown on an entry.
type Control struct {
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	EntityID string `json:"entity_id"`
	State    string `json:"state,omitempty"`
}

// Controls is what ha.controls returns for an entry: the entry's own switch
// and the switch it is powered by (restart candidate).
type Controls struct {
	Entry string   `json:"entry"` // name of the entry asked about
	Self  *Control `json:"self,omitempty"`
	Power *Control `json:"power,omitempty"`
}

// controlOf reads the switch of an entry from its attributes.
func controlOf(e pdk.Entity) *Control {
	id, _ := e.Attrs["ha_entity"].(string)
	if !Switchable(id) {
		return nil
	}
	return &Control{Slug: e.Slug, Name: e.Name, EntityID: id}
}

// kindFor picks the Phoedox kind from the device's switch: switches are
// smart plugs (so other entries can be powered_by them), lights are lights.
func kindFor(primary string) string {
	switch {
	case strings.HasPrefix(primary, "switch."):
		return "smart_plug"
	case strings.HasPrefix(primary, "light."):
		return "light"
	}
	return "device"
}
