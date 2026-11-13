// Package manifestcheck validates every plugin manifest in plugins/*.
// Full schema validation uses phoedox-sdk once it is published; until then
// this checks the required fields and that referenced files exist.
package manifestcheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

type manifest struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	HostAPI string `json:"hostApi"`
	Engines struct {
		Phoedox string `json:"phoedox"`
	} `json:"engines"`
	Contributes struct {
		Kinds []struct {
			Schema string `json:"schema"`
		} `json:"kinds"`
	} `json:"contributes"`
}

var (
	idRe     = regexp.MustCompile(`^[a-z0-9]+(\.[a-z0-9-]+)+$`)
	semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)
)

func TestManifests(t *testing.T) {
	files, _ := filepath.Glob("../../plugins/*/phoedox-plugin.json")
	if len(files) == 0 {
		t.Fatal("no plugin manifests found")
	}
	for _, f := range files {
		t.Run(filepath.Base(filepath.Dir(f)), func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var m manifest
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatal(err)
			}
			if !idRe.MatchString(m.ID) || m.Name == "" || !semverRe.MatchString(m.Version) || m.HostAPI == "" || m.Engines.Phoedox == "" {
				t.Fatalf("invalid manifest fields: %+v", m)
			}
			for _, k := range m.Contributes.Kinds {
				if k.Schema == "" {
					continue
				}
				if _, err := os.Stat(filepath.Join(filepath.Dir(f), k.Schema)); err != nil {
					t.Errorf("kind schema %s missing", k.Schema)
				}
			}
		})
	}
}
