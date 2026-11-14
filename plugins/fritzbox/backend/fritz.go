// Logic of the FRITZ!Box plugin, without build tags so it is testable
// natively: TR-064 SOAP envelopes, HTTP digest authentication and the host
// list.
package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// envelope builds a TR-064 SOAP request without arguments.
func envelope(service, action string) string {
	return `<?xml version="1.0" encoding="utf-8"?>` +
		`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">` +
		`<s:Body><u:` + action + ` xmlns:u="` + service + `"/></s:Body></s:Envelope>`
}

// soapValues reads the elements of an action response (names without namespace).
func soapValues(body string) (map[string]string, error) {
	d := xml.NewDecoder(strings.NewReader(body))
	out := map[string]string{}
	var name string
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name = t.Name.Local
		case xml.CharData:
			if name != "" && (strings.HasPrefix(name, "New") || name == "errorDescription") {
				out[name] = strings.TrimSpace(string(t))
			}
		case xml.EndElement:
			name = ""
		}
	}
	if fault := out["errorDescription"]; fault != "" {
		return nil, fmt.Errorf("FRITZ!Box: %s", fault)
	}
	return out, nil
}

var digestParam = regexp.MustCompile(`(\w+)="?([^",]*)"?`)

// Digest holds a server challenge for HTTP digest authentication (RFC 2617, MD5, qop=auth).
type Digest struct {
	Realm, Nonce, Qop, Opaque string
	nc                        int
}

// parseChallenge reads a WWW-Authenticate: Digest header.
func parseChallenge(h string) (*Digest, error) {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(h)), "digest ") {
		return nil, fmt.Errorf("FRITZ!Box did not ask for digest authentication")
	}
	d := &Digest{}
	for _, m := range digestParam.FindAllStringSubmatch(h[7:], -1) {
		switch strings.ToLower(m[1]) {
		case "realm":
			d.Realm = m[2]
		case "nonce":
			d.Nonce = m[2]
		case "qop":
			d.Qop = m[2]
		case "opaque":
			d.Opaque = m[2]
		}
	}
	if d.Nonce == "" {
		return nil, fmt.Errorf("FRITZ!Box sent no nonce")
	}
	return d, nil
}

func md5hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

// Authorization answers the challenge for one request.
func (d *Digest) Authorization(user, pass, method, uri, cnonce string) string {
	d.nc++
	nc := fmt.Sprintf("%08x", d.nc)
	ha1 := md5hex(user + ":" + d.Realm + ":" + pass)
	ha2 := md5hex(method + ":" + uri)
	var resp string
	if d.Qop != "" {
		resp = md5hex(ha1 + ":" + d.Nonce + ":" + nc + ":" + cnonce + ":auth:" + ha2)
	} else {
		resp = md5hex(ha1 + ":" + d.Nonce + ":" + ha2)
	}
	h := fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s", algorithm=MD5`, user, d.Realm, d.Nonce, uri, resp)
	if d.Qop != "" {
		h += fmt.Sprintf(`, qop=auth, nc=%s, cnonce="%s"`, nc, cnonce)
	}
	if d.Opaque != "" {
		h += fmt.Sprintf(`, opaque="%s"`, d.Opaque)
	}
	return h
}

// Host is an entry of the host list (X_AVM-DE_GetHostListPath).
type Host struct {
	IP        string `xml:"IPAddress" json:"ip,omitempty"`
	MAC       string `xml:"MACAddress" json:"mac"`
	Name      string `xml:"HostName" json:"name"`
	Active    int    `xml:"Active" json:"-"`
	Interface string `xml:"InterfaceType" json:"connection"`
	Online    bool   `xml:"-" json:"online"`
}

func parseHosts(body string) ([]Host, error) {
	var l struct {
		Items []Host `xml:"Item"`
	}
	if err := xml.Unmarshal([]byte(body), &l); err != nil {
		return nil, fmt.Errorf("unexpected host list from the FRITZ!Box: %w", err)
	}
	for i := range l.Items {
		h := &l.Items[i]
		h.Online = h.Active == 1
		switch h.Interface {
		case "Ethernet":
			h.Interface = "lan"
		case "802.11":
			h.Interface = "wifi"
		case "":
			h.Interface = "unknown"
		}
	}
	return l.Items, nil
}

// Info is the device information of the box.
type Info struct {
	Model, Firmware, Serial string
}

// graphOps: the box as router; with hosts, active devices wired to it (LAN)
// or with an uplink to it (Wi-Fi).
func graphOps(info Info, hosts []Host) pdk.GraphOps {
	var ops pdk.GraphOps
	attrs := map[string]any{"model": info.Model, "firmware": info.Firmware}
	if info.Serial != "" {
		attrs["serial"] = info.Serial
	}
	box := ops.UpsertNamed("router", "fritzbox-"+integ.Slugify(info.Model), info.Model, attrs)
	for _, h := range hosts {
		if !h.Online || h.MAC == "" {
			continue
		}
		name := h.Name
		if name == "" {
			name = h.MAC
		}
		attrs := map[string]any{"mac": strings.ToLower(h.MAC)}
		if h.IP != "" {
			attrs["ip"] = h.IP
		}
		ref := ops.UpsertNamed("device", "fritz-"+integ.Slugify(strings.ReplaceAll(h.MAC, ":", "")), name, attrs)
		switch h.Interface {
		case "lan":
			ops.Link(ref, "wired_to", box, nil)
		case "wifi":
			ops.Link(ref, "uplink", box, nil)
		}
	}
	return ops
}
