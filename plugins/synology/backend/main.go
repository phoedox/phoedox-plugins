//go:build wasip1

// Backend of the Synology plugin: imports the NAS every six hours (and on
// synology.sync) through the DSM Web API; storage for AI clients. Read-only:
// it signs in, reads and signs out. Requests are POSTed as forms so that the
// password and the session ID never appear in a URL.
package main

import (
	"encoding/json"
	"fmt"

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

type nas struct {
	base, sid string
}

func (n *nas) post(form string) ([]byte, error) {
	res, err := pdk.Fetch(pdk.FetchRequest{Method: "POST", URL: n.base + "/webapi/entry.cgi", Body: form,
		Headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Accept": "application/json"}})
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return nil, &integ.StatusError{Service: "Synology DSM", Status: res.Status}
	}
	return []byte(res.Body), nil
}

// login signs in; callers must defer logout.
func login() (*nas, error) {
	raw, err := integ.Setting("url", "e.g. https://nas.example.org:5001")
	if err != nil {
		return nil, err
	}
	base, err := integ.BaseURL(raw)
	if err != nil {
		return nil, err
	}
	user, err := integ.Setting("username", "a DSM account in the administrators group")
	if err != nil {
		return nil, err
	}
	pass, err := integ.Setting("password", "the password of that account")
	if err != nil {
		return nil, err
	}
	n := &nas{base: base}
	body, err := n.post(loginForm(user, pass))
	if err != nil {
		return nil, err
	}
	data, err := unwrap(body, true)
	if err != nil {
		return nil, err
	}
	sid, err := parseSID(data)
	if err != nil {
		return nil, err
	}
	n.sid = sid
	return n, nil
}

func (n *nas) logout() { _, _ = n.post(logoutForm(n.sid)) }

func (n *nas) call(api string, version int, method string) (json.RawMessage, error) {
	body, err := n.post(callForm(api, version, method, n.sid))
	if err != nil {
		return nil, err
	}
	return unwrap(body, false)
}

func sync() (map[string]int, error) {
	n, err := login()
	if err != nil {
		return nil, err
	}
	defer n.logout()
	var info NAS
	if data, err := n.call("SYNO.Core.System", 1, "info"); err == nil {
		info = parseSystem(data)
	} else {
		data, err2 := n.call("SYNO.DSM.Info", 2, "getinfo")
		if err2 != nil {
			return nil, err
		}
		info = parseDSMInfo(data)
	}
	if data, err := n.call("SYNO.Core.Network", 1, "get"); err == nil {
		info.Hostname = parseHostname(data)
	}
	return map[string]int{"servers": 1}, integ.Apply(graphOps(info, n.base))
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "synology.sync":
		return sync()
	case "synology.storage":
		n, err := login()
		if err != nil {
			return nil, err
		}
		defer n.logout()
		data, err := n.call("SYNO.Storage.CGI.Storage", 1, "load_info")
		if err != nil {
			return nil, err
		}
		return parseStorage(data)
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
