// Package manifestcheck validates every plugin manifest in plugins/* with
// the SDK's rules and checks that referenced files exist.
package manifestcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

func TestManifests(t *testing.T) {
	files, _ := filepath.Glob("../../plugins/*/phoedox-plugin.json")
	if len(files) == 0 {
		t.Fatal("no plugin manifests found")
	}
	for _, f := range files {
		dir := filepath.Dir(f)
		t.Run(filepath.Base(dir), func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			m, err := pdk.ParseManifest(data)
			if err != nil {
				t.Fatal(err)
			}
			exists := func(what, p string) {
				if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
					t.Errorf("%s %s missing", what, p)
				}
			}
			for _, k := range m.Contributes.Kinds {
				if k.Schema != "" {
					exists("kind schema", k.Schema)
				}
			}
			for _, tool := range m.Contributes.MCPTools {
				if tool.Input != "" {
					exists("tool input", tool.Input)
				}
			}
			if m.Frontend != nil {
				exists("frontend source", strings.TrimSuffix(m.Frontend.Module, ".js")+".ts")
			}
			if m.Backend != nil {
				exists("backend", "backend")
			}
			if !strings.HasPrefix(m.ID, "com.phoedox.") && m.ID != "de.example.ups" {
				t.Errorf("official plugins use com.phoedox.<name>")
			}
		})
	}
}
