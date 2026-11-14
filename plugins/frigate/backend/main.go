//go:build wasip1

// Backend of the Frigate plugin: imports Frigate and its cameras every six
// hours (and on frigate.sync); recent events and statistics for AI clients.
// Read-only: only GET requests (plus POST /api/login when a user is set).
package main

import (
	"encoding/json"
	"errors"
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

type frigate struct {
	base string
	h    map[string]string
}

// connect reads the settings and logs in: with username and password via
// POST /api/login (the session token is used as bearer token), else with a
// fixed token, else without authentication (port 5000 or auth disabled).
func connect() (*frigate, error) {
	raw, err := integ.Setting("url", "e.g. https://frigate.example.org or http://192.168.1.30:5000")
	if err != nil {
		return nil, err
	}
	base, err := integ.BaseURL(raw)
	if err != nil {
		return nil, err
	}
	f := &frigate{base: base, h: map[string]string{"Accept": "application/json"}}
	user, pass := integ.Optional("username"), integ.Optional("password")
	if user != "" && pass != "" {
		res, err := pdk.Fetch(pdk.FetchRequest{Method: "POST", URL: base + "/api/login", Body: loginBody(user, pass),
			Headers: map[string]string{"Content-Type": "application/json"}})
		if err != nil {
			return nil, err
		}
		switch res.Status {
		case 200:
			tok := cookieToken(res.Headers["set-cookie"])
			if tok == "" {
				return nil, errors.New("Frigate accepted the login but sent no session token")
			}
			f.h["Authorization"] = "Bearer " + tok
		case 404: // authentication is disabled
		case 401, 403:
			return nil, errors.New("Frigate rejected the login (settings username and password)")
		default:
			return nil, &integ.StatusError{Service: "Frigate login", Status: res.Status}
		}
	} else if tok := integ.Optional("token"); tok != "" {
		f.h["Authorization"] = "Bearer " + tok
	}
	return f, nil
}

func (f *frigate) get(path string) ([]byte, error) {
	res, err := pdk.Fetch(pdk.FetchRequest{URL: f.base + path, Headers: f.h})
	if err != nil {
		return nil, err
	}
	switch res.Status {
	case 200:
		return []byte(res.Body), nil
	case 401, 403:
		return nil, errors.New("Frigate asks for a login: set username and password (or token), or use the unauthenticated port 5000")
	}
	return nil, &integ.StatusError{Service: "Frigate", Status: res.Status}
}

func (f *frigate) stats() (Stats, error) {
	b, err := f.get("/api/stats")
	if err != nil {
		return Stats{}, err
	}
	return parseStats(b)
}

func sync() (map[string]int, error) {
	f, err := connect()
	if err != nil {
		return nil, err
	}
	b, err := f.get("/api/config")
	if err != nil {
		return nil, err
	}
	cfg, err := parseConfig(b)
	if err != nil {
		return nil, err
	}
	st, err := f.stats()
	if err != nil {
		return nil, err
	}
	name := integ.Optional("name")
	if name == "" {
		name = "Frigate"
	}
	if err := integ.Apply(graphOps(name, f.base, cfg, st)); err != nil {
		return nil, err
	}
	return map[string]int{"cameras": len(cfg.Cameras)}, nil
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "frigate.sync":
		return sync()
	case "frigate.events":
		var args EventArgs
		_ = json.Unmarshal(tc.Args, &args)
		f, err := connect()
		if err != nil {
			return nil, err
		}
		b, err := f.get(eventsQuery(args))
		if err != nil {
			return nil, err
		}
		return parseEvents(b)
	case "frigate.stats":
		f, err := connect()
		if err != nil {
			return nil, err
		}
		st, err := f.stats()
		if err != nil {
			return nil, err
		}
		return statsView(st), nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
