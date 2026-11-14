//go:build wasip1

// Backend of the FRITZ!Box plugin: imports the box every six hours (and on
// fritz.sync) and lists home network devices for AI clients. Read-only.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

func main() {}

//go:wasmexport on_event
func onEvent() int32 {
	return pdk.HandleEvent(func(ev pdk.Event) error {
		if ev.Type != "schedule" {
			return nil
		}
		_, err := sync()
		return err
	})
}

//go:wasmexport tool
func tool() int32 { return pdk.HandleTool(runTool) }

type box struct {
	base, user, pass string
	digest           *Digest
}

func connect() (*box, error) {
	raw, err := integ.Setting("url", "e.g. http://192.168.178.1:49000")
	if err != nil {
		return nil, err
	}
	base, err := integ.BaseURL(raw)
	if err != nil {
		return nil, err
	}
	user, err := integ.Setting("username", "a FRITZ!Box user")
	if err != nil {
		return nil, err
	}
	pass, err := integ.Setting("password", "the password of that user")
	if err != nil {
		return nil, err
	}
	return &box{base: base, user: user, pass: pass}, nil
}

func cnonce() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// call sends a TR-064 action, answering the digest challenge (and a stale nonce) once.
func (b *box) call(control, service, action string) (map[string]string, error) {
	for attempt := 0; attempt < 2; attempt++ {
		h := map[string]string{"Content-Type": `text/xml; charset="utf-8"`, "SOAPAction": service + "#" + action}
		if b.digest != nil {
			h["Authorization"] = b.digest.Authorization(b.user, b.pass, "POST", control, cnonce())
		}
		res, err := pdk.Fetch(pdk.FetchRequest{Method: "POST", URL: b.base + control, Headers: h, Body: envelope(service, action)})
		if err != nil {
			return nil, err
		}
		if res.Status == 401 {
			d, err := parseChallenge(res.Headers["www-authenticate"])
			if err != nil {
				return nil, err
			}
			b.digest = d
			continue
		}
		if res.Status != 200 && res.Status != 500 {
			return nil, &integ.StatusError{Service: "FRITZ!Box", Status: res.Status}
		}
		return soapValues(res.Body)
	}
	return nil, fmt.Errorf("the FRITZ!Box rejected username or password")
}

func (b *box) info() (Info, error) {
	v, err := b.call("/upnp/control/deviceinfo", "urn:dslforum-org:service:DeviceInfo:1", "GetInfo")
	if err != nil {
		return Info{}, err
	}
	model := v["NewModelName"]
	if model == "" {
		model = "FRITZ!Box"
	}
	return Info{Model: model, Firmware: v["NewSoftwareVersion"], Serial: v["NewSerialNumber"]}, nil
}

func (b *box) hosts() ([]Host, error) {
	v, err := b.call("/upnp/control/hosts", "urn:dslforum-org:service:Hosts:1", "X_AVM-DE_GetHostListPath")
	if err != nil {
		return nil, err
	}
	path := v["NewX_AVM-DE_HostListPath"]
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("the FRITZ!Box sent no host list")
	}
	res, err := pdk.Fetch(pdk.FetchRequest{URL: b.base + path})
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return nil, &integ.StatusError{Service: "FRITZ!Box", Status: res.Status}
	}
	return parseHosts(res.Body)
}

func sync() (map[string]int, error) {
	b, err := connect()
	if err != nil {
		return nil, err
	}
	info, err := b.info()
	if err != nil {
		return nil, err
	}
	var hosts []Host
	if integ.Truthy(integ.Optional("import_hosts")) {
		if hosts, err = b.hosts(); err != nil {
			return nil, err
		}
	}
	ops := graphOps(info, hosts)
	return map[string]int{"entries": len(ops.Ops)}, integ.Apply(ops)
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "fritz.sync":
		return sync()
	case "fritz.hosts":
		b, err := connect()
		if err != nil {
			return nil, err
		}
		return b.hosts()
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
