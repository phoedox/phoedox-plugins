// Logic of the Philips Hue plugin, without build tags so it is testable natively.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// apiBase builds http://<bridge>/api/<username>. The bridge's HTTPS
// certificate is signed by Signify's own CA, which the host does not trust,
// so the plugin uses API v1 over plain HTTP.
func apiBase(rawURL, username string) (string, error) {
	base, err := integ.BaseURL(rawURL)
	if err != nil {
		return "", errors.New("the setting url must be the bridge address like http://192.168.1.20")
	}
	u, _ := url.Parse(base)
	if u.Scheme != "http" {
		return "", errors.New("the setting url must start with http:// (the bridge's HTTPS certificate is not trusted by Phoedox)")
	}
	if !userRe.MatchString(username) {
		return "", errors.New("the setting username must be the application key of the bridge (letters, digits, - and _)")
	}
	return base + "/api/" + username, nil
}

var userRe = regexp.MustCompile(`^[A-Za-z0-9_-]{10,80}$`)

// redact removes the application key from a message (the host's fetch
// errors may contain the request URL).
func redact(msg, username string) string {
	if username == "" {
		return msg
	}
	return strings.ReplaceAll(msg, username, "***")
}

// apiError reads the error list the bridge answers with HTTP 200, e.g.
// [{"error":{"type":1,"address":"/lights","description":"unauthorized user"}}].
func apiError(body []byte) error {
	var list []struct {
		Error *struct {
			Type        int    `json:"type"`
			Description string `json:"description"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &list) != nil || len(list) == 0 || list[0].Error == nil {
		return nil
	}
	if list[0].Error.Type == 1 {
		return errors.New("the bridge does not know the application key: set the setting username to a key created with the link button")
	}
	return fmt.Errorf("the Hue bridge answered: %s", list[0].Error.Description)
}

func decode(body []byte, out any) error {
	if err := apiError(body); err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("unexpected answer from the Hue bridge: %w", err)
	}
	return nil
}

// Config is GET /config.
type Config struct {
	Name       string `json:"name"`
	BridgeID   string `json:"bridgeid"`
	MAC        string `json:"mac"`
	IP         string `json:"ipaddress"`
	ModelID    string `json:"modelid"`
	SWVersion  string `json:"swversion"`
	APIVersion string `json:"apiversion"`
	ZigbeeChan int    `json:"zigbeechannel"`
}

// Light is one entry of GET /lights.
type Light struct {
	ID    string `json:"-"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	State struct {
		On        *bool `json:"on"`
		Bri       *int  `json:"bri"`
		Reachable *bool `json:"reachable"`
	} `json:"state"`
	ModelID      string `json:"modelid"`
	Manufacturer string `json:"manufacturername"`
	Product      string `json:"productname"`
	UniqueID     string `json:"uniqueid"`
	SWVersion    string `json:"swversion"`
}

// Group is one entry of GET /groups; rooms have type "Room".
type Group struct {
	ID     string   `json:"-"`
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Class  string   `json:"class"`
	Lights []string `json:"lights"`
}

func parseConfig(body []byte) (Config, error) {
	var c Config
	if err := decode(body, &c); err != nil {
		return c, err
	}
	if c.BridgeID == "" && c.MAC == "" {
		return c, errors.New("the address does not answer like a Hue bridge")
	}
	return c, nil
}

func parseLights(body []byte) ([]Light, error) {
	var m map[string]Light
	if err := decode(body, &m); err != nil {
		return nil, err
	}
	out := make([]Light, 0, len(m))
	for id, l := range m {
		l.ID = id
		out = append(out, l)
	}
	slices.SortFunc(out, func(a, b Light) int { return byNumber(a.ID, b.ID) })
	return out, nil
}

// parseRooms returns the groups of type Room.
func parseRooms(body []byte) ([]Group, error) {
	var m map[string]Group
	if err := decode(body, &m); err != nil {
		return nil, err
	}
	var out []Group
	for id, g := range m {
		if g.Type == "Room" {
			g.ID = id
			out = append(out, g)
		}
	}
	slices.SortFunc(out, func(a, b Group) int { return byNumber(a.ID, b.ID) })
	return out, nil
}

func byNumber(a, b string) int {
	x, _ := strconv.Atoi(a)
	y, _ := strconv.Atoi(b)
	if x != y {
		return x - y
	}
	return strings.Compare(a, b)
}

// roomIndex maps lower-case names of existing rooms to their slugs, so a
// Hue room joins a room someone already created (by hand or by another
// plugin) instead of adding a second one.
func roomIndex(rooms []pdk.Entity) map[string]string {
	out := map[string]string{}
	for _, r := range rooms {
		k := normName(r.Name)
		if _, ok := out[k]; !ok && k != "" && r.Slug != "" {
			out[k] = r.Slug
		}
	}
	return out
}

func normName(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }

// graphOps: the bridge as device, rooms as room (an existing room with the
// same name is reused, otherwise "hue-room-<group id>"), lights as light
// "located in" their room and "related to" the bridge.
func graphOps(c Config, lights []Light, rooms []Group, existing map[string]string) pdk.GraphOps {
	var ops pdk.GraphOps
	bid := strings.ToLower(c.BridgeID)
	if bid == "" {
		bid = strings.ToLower(strings.ReplaceAll(c.MAC, ":", ""))
	}
	name := c.Name
	if name == "" {
		name = "Hue Bridge"
	}
	battrs := map[string]any{"manufacturer": "Signify"}
	set := func(m map[string]any, k, v string) {
		if v != "" {
			m[k] = v
		}
	}
	set(battrs, "model", c.ModelID)
	set(battrs, "serial", c.BridgeID)
	if _, err := netip.ParseAddr(c.IP); err == nil {
		battrs["ip"] = c.IP
	}
	set(battrs, "mac", strings.ToUpper(c.MAC))
	set(battrs, "firmware", c.SWVersion)
	set(battrs, "hue_api_version", c.APIVersion)
	bridge := ops.UpsertNamed("device", "hue-bridge-"+integ.Slugify(bid), name, battrs)
	roomOf := map[string]string{}
	for _, r := range rooms {
		ref, ok := existing[normName(r.Name)]
		if !ok {
			ref = ops.UpsertNamed("room", "hue-room-"+integ.Slugify(r.ID), r.Name, map[string]any{})
		}
		for _, l := range r.Lights {
			roomOf[l] = ref
		}
	}
	for _, l := range lights {
		attrs := map[string]any{}
		set(attrs, "manufacturer", l.Manufacturer)
		set(attrs, "model", l.ModelID)
		set(attrs, "product", l.Product)
		set(attrs, "type", l.Type)
		set(attrs, "uniqueid", l.UniqueID)
		set(attrs, "firmware", l.SWVersion)
		if l.State.Reachable != nil {
			attrs["reachable"] = *l.State.Reachable
		}
		id := "hue-light-" + integ.Slugify(l.UniqueID)
		if l.UniqueID == "" {
			id = "hue-light-" + integ.Slugify(bid+"-"+l.ID)
		}
		lname := l.Name
		if lname == "" {
			lname = "Hue light " + l.ID
		}
		ref := ops.UpsertNamed("light", id, lname, attrs)
		if room, ok := roomOf[l.ID]; ok {
			ops.Link(ref, "located_in", room, nil)
		}
		ops.Link(ref, "related_to", bridge, nil)
	}
	return ops
}

// LightRow is what hue.lights returns per light.
type LightRow struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Room       string `json:"room,omitempty"`
	On         *bool  `json:"on,omitempty"`
	Brightness *int   `json:"brightness_pct,omitempty"`
	Reachable  *bool  `json:"reachable,omitempty"`
	Type       string `json:"type,omitempty"`
	Model      string `json:"model,omitempty"`
	Product    string `json:"product,omitempty"`
}

func lightRows(lights []Light, rooms []Group) []LightRow {
	roomName := map[string]string{}
	for _, r := range rooms {
		for _, l := range r.Lights {
			roomName[l] = r.Name
		}
	}
	out := make([]LightRow, 0, len(lights))
	for _, l := range lights {
		r := LightRow{ID: l.ID, Name: l.Name, Room: roomName[l.ID], On: l.State.On, Reachable: l.State.Reachable, Type: l.Type, Model: l.ModelID, Product: l.Product}
		if l.State.Bri != nil {
			pct := int(math.Round(float64(*l.State.Bri) / 254 * 100))
			r.Brightness = &pct
		}
		out = append(out, r)
	}
	return out
}
