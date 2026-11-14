//go:build wasip1

// Backend of the Google Workspace plugin: signs in as a service account
// with domain-wide delegation, imports the ChromeOS and mobile devices every
// six hours (and on gworkspace.sync) and lists them for AI clients.
// Read-only.
package main

import (
	"encoding/json"
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
		_, err := sync()
		return err
	})
}

//go:wasmexport tool
func tool() int32 { return pdk.HandleTool(runTool) }

// maxPages bounds the pages per listing.
const maxPages = 50

type admin struct{ h map[string]string }

func connect() (*admin, error) {
	raw, err := integ.Setting("service_account_json", "the JSON key file of a service account with domain-wide delegation")
	if err != nil {
		return nil, err
	}
	sa, err := parseServiceAccount(raw)
	if err != nil {
		return nil, err
	}
	subject, err := integ.Setting("admin_email", "the e-mail address of a Google Workspace administrator")
	if err != nil {
		return nil, err
	}
	subject = strings.TrimSpace(subject)
	if err := validAdmin(subject); err != nil {
		return nil, err
	}
	jwt, err := sa.assertion(subject, scopes, time.Now())
	if err != nil {
		return nil, err
	}
	res, err := pdk.Fetch(pdk.FetchRequest{
		Method:  "POST",
		URL:     tokenURL,
		Headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Accept": "application/json"},
		Body:    tokenBody(jwt),
	})
	if err != nil {
		return nil, err
	}
	token, err := parseToken(res.Status, []byte(res.Body))
	if err != nil {
		return nil, err
	}
	return &admin{h: map[string]string{"Authorization": "Bearer " + token, "Accept": "application/json"}}, nil
}

func (a *admin) get(link string) ([]byte, error) {
	res, err := pdk.Fetch(pdk.FetchRequest{URL: link, Headers: a.h})
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return nil, apiError(res.Status, []byte(res.Body))
	}
	return []byte(res.Body), nil
}

// list reads both listings, page by page.
func (a *admin) list() ([]Device, error) {
	var chrome []ChromeOS
	next := ""
	for page := 0; page < maxPages; page++ {
		body, err := a.get(chromeURL(next))
		if err != nil {
			return nil, err
		}
		l, n, err := parseChrome(body)
		if err != nil {
			return nil, err
		}
		chrome = append(chrome, l...)
		if next = n; next == "" {
			break
		}
	}
	var mobile []Mobile
	next = ""
	for page := 0; page < maxPages; page++ {
		body, err := a.get(mobileURL(next))
		if err != nil {
			return nil, err
		}
		l, n, err := parseMobile(body)
		if err != nil {
			return nil, err
		}
		mobile = append(mobile, l...)
		if next = n; next == "" {
			break
		}
	}
	return append(fromChrome(chrome), fromMobile(mobile)...), nil
}

func sync() (map[string]int, error) {
	a, err := connect()
	if err != nil {
		return nil, err
	}
	list, err := a.list()
	if err != nil {
		return nil, err
	}
	return map[string]int{"devices": len(list)}, integ.Apply(graphOps(list))
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "gworkspace.sync":
		return sync()
	case "gworkspace.devices":
		var in struct {
			Q string `json:"q"`
		}
		if len(tc.Args) > 0 {
			if err := json.Unmarshal(tc.Args, &in); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
		}
		a, err := connect()
		if err != nil {
			return nil, err
		}
		list, err := a.list()
		if err != nil {
			return nil, err
		}
		return filter(list, in.Q), nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
