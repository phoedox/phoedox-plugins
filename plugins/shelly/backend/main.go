//go:build wasip1

// Backend of the Shelly plugin: imports the configured Shelly devices every
// six hours (and on shelly.sync); live power, energy, temperature and Wi-Fi
// values for AI clients. Read-only: only GET requests, nothing is switched.
package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
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
	case "shelly.sync":
		return sync(45 * time.Second)
	case "shelly.status":
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
		fn(d, err)
	}
	if n == 0 {
		return errors.New("set the setting url (e.g. http://192.168.1.50) and device2_url … for further devices")
	}
	return nil
}

type client struct {
	base, user, pass string
	gen              int
}

func read(setting, raw, user, pass string) (Device, error) {
	d := Device{Setting: setting}
	base, host, err := deviceBase(setting, raw)
	if err != nil {
		return d, err
	}
	d.Host = host
	c := &client{base: base, user: user, pass: pass}
	body, err := c.get("/shelly")
	if err != nil {
		return d, err
	}
	if d.Info, err = parseInfo(body); err != nil {
		return d, err
	}
	c.gen = d.Info.Gen
	if c.gen >= 2 {
		body, err = c.get("/rpc/Shelly.GetStatus")
		if err != nil {
			return d, err
		}
		d.Status, err = parseGen2Status(body)
		return d, err
	}
	body, err = c.get("/status")
	if err != nil {
		return d, err
	}
	if d.Status, err = parseGen1Status(body); err != nil {
		return d, err
	}
	if b, err := c.get("/settings"); err == nil {
		s := parseSettings1(b)
		if s.Name != nil {
			d.Name = *s.Name
		}
		d.Hostname = s.Device.Hostname
	}
	return d, nil
}

// get fetches a path; on 401 it logs in (digest for Gen2+, basic for Gen1)
// and tries once more. Credentials are only sent when the device asks.
func (c *client) get(path string) ([]byte, error) {
	res, err := pdk.Fetch(pdk.FetchRequest{URL: c.base + path, Headers: map[string]string{"Accept": "application/json"}})
	if err != nil {
		return nil, err
	}
	if res.Status == 401 {
		if c.pass == "" {
			return nil, errors.New("the Shelly asks for a password: set the setting password")
		}
		h := map[string]string{"Accept": "application/json"}
		if c.gen >= 2 {
			auth, err := digestAuth(res.Headers["www-authenticate"], "GET", path, "admin", c.pass, cnonce())
			if err != nil {
				return nil, err
			}
			h["Authorization"] = auth
		} else {
			h["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(c.user+":"+c.pass))
		}
		if res, err = pdk.Fetch(pdk.FetchRequest{URL: c.base + path, Headers: h}); err != nil {
			return nil, err
		}
		if res.Status == 401 {
			return nil, errors.New("the Shelly rejected the password (setting password; Gen1 also username)")
		}
	}
	if res.Status != 200 {
		return nil, &integ.StatusError{Service: "Shelly", Status: res.Status}
	}
	return []byte(res.Body), nil
}

func cnonce() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
