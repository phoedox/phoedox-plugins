//go:build wasip1

package integ

import (
	"encoding/json"
	"fmt"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// Setting reads a required plugin setting with a hint for the error.
func Setting(name, hint string) (string, error) {
	v, err := pdk.Secret(name)
	if err != nil || v == "" {
		return "", fmt.Errorf("set the setting %s (%s)", name, hint)
	}
	return v, nil
}

// Optional reads a setting that may be missing.
func Optional(name string) string {
	v, _ := pdk.Secret(name)
	return v
}

// GetJSON fetches a URL and decodes a 200 answer into out.
func GetJSON(service, url string, headers map[string]string, out any) error {
	res, err := pdk.Fetch(pdk.FetchRequest{URL: url, Headers: headers})
	if err != nil {
		return err
	}
	if res.Status != 200 {
		return &StatusError{Service: service, Status: res.Status}
	}
	if err := json.Unmarshal([]byte(res.Body), out); err != nil {
		return fmt.Errorf("unexpected answer from %s: %w", service, err)
	}
	return nil
}

// Apply writes graph changes if there are any.
func Apply(ops pdk.GraphOps) error {
	if len(ops.Ops) == 0 {
		return nil
	}
	_, err := pdk.Graph.Apply(&ops)
	return err
}
