//go:build wasip1

// Backend of the Tasmota plugin: imports the configured Tasmota devices
// every six hours (and on tasmota.sync); relay states and energy values for
// AI clients. Read-only: only the commands "Status 0" and "Module" are sent,
// nothing is switched.
package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

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
		// Scheduled runs have the host's default limit of 10 s.
		_, err := sync(7 * time.Second)
		return err
	})
}

//go:wasmexport tool
func tool() int32 { return pdk.HandleTool(runTool) }

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "tasmota.sync":
		return sync(45 * time.Second)
	case "tasmota.status":
		var rows []Row
		err := eachDevice(45*time.Second, func(d Device, err error) {
			if err != nil {
				rows = append(rows, Row{Setting: d.Setting, Error: err.Error()})
				return
			}
			rows = append(rows, row(d))
		})
		return rows, err
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}

// sync imports device by device, so a slow or offline device does not undo
// the others.
func sync(budget time.Duration) (map[string]int, error) {
	out := map[string]int{"imported": 0, "failed": 0}
	var firstErr error
	err := eachDevice(budget, func(d Device, err error) {
		if err == nil {
			err = integ.Apply(graphOps(d))
		}
		if err != nil {
			out["failed"]++
			pdk.Warn(d.Setting + ": " + err.Error())
			if firstErr == nil {
				firstErr = err
			}
			return
		}
		out["imported"]++
	})
	if err != nil {
		return nil, err
	}
	if out["imported"] == 0 && firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

// eachDevice reads every configured device. It stops starting new devices
// when the time budget is used up and reports the rest as skipped.
func eachDevice(budget time.Duration, fn func(Device, error)) error {
	pass := integ.Optional("password")
	user := integ.Optional("username")
	if user == "" {
		user = "admin"
	}
	start := time.Now()
	n := 0
	for _, s := range slots() {
		raw := strings.TrimSpace(integ.Optional(s))
		if raw == "" {
			continue
		}
		n++
		if time.Since(start) > budget {
			fn(Device{Setting: s}, errors.New("skipped: time limit reached"))
			continue
		}
		d, err := read(s, raw, user, pass)
		if err != nil {
			err = errors.New(redact(err.Error(), pass))
		}
		fn(d, err)
	}
	if n == 0 {
		return errors.New("set the setting url (e.g. http://192.168.1.60) and device2_url … for further devices")
	}
	return nil
}

func read(setting, raw, user, pass string) (Device, error) {
	d := Device{Setting: setting}
	base, err := deviceBase(setting, raw)
	if err != nil {
		return d, err
	}
	body, err := command(base, "Status 0", user, pass)
	if err != nil {
		return d, err
	}
	if d.Status, err = parseStatus0(body); err != nil {
		return d, err
	}
	if b, err := command(base, "Module", user, pass); err == nil {
		d.Module = parseModule(b)
	}
	return d, nil
}

func command(base, cmnd, user, pass string) ([]byte, error) {
	res, err := pdk.Fetch(pdk.FetchRequest{URL: commandURL(base, cmnd, user, pass), Headers: map[string]string{"Accept": "application/json"}})
	if err != nil {
		return nil, err
	}
	if res.Status == 401 {
		return nil, errAuth
	}
	if res.Status != 200 {
		return nil, &integ.StatusError{Service: "Tasmota", Status: res.Status}
	}
	return []byte(res.Body), nil
}
