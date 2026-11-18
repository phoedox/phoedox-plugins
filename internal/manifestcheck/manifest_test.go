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
	tiles := map[string]string{}
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
			// Tiles and logos (ADR-0090, docs/branding).
			if m.Icon == "" || m.Category == "" || m.Category == "other" {
				t.Errorf("official plugins need an icon and a category other than other")
			}
			tile := m.Category + "/" + m.Icon
			if other, dup := tiles[tile]; dup {
				t.Errorf("same icon and category as %s: %s", other, tile)
			}
			tiles[tile] = filepath.Base(dir)
			// Screenshots (concept "Plugin logos and screenshots"): at least
			// one, made by scripts/shots.mjs, with a caption, at most 2 MB.
			if len(m.Screenshots) == 0 {
				t.Errorf("official plugins need at least one screenshot (scripts/shots.mjs)")
			}
			for _, sc := range m.Screenshots {
				info, err := os.Stat(filepath.Join(dir, sc.File))
				switch {
				case err != nil:
					t.Errorf("screenshot %s missing", sc.File)
				case info.Size() > 2<<20:
					t.Errorf("screenshot %s is larger than 2 MB", sc.File)
				}
				if strings.TrimSpace(sc.Caption) == "" {
					t.Errorf("screenshot %s needs a caption", sc.File)
				}
			}
			for _, l := range []string{m.Logo, m.LogoDark} {
				if l == "" {
					continue
				}
				b, err := os.ReadFile(filepath.Join(dir, l))
				if err != nil {
					t.Errorf("logo %s missing", l)
				} else if err := pdk.CheckSVG(b); err != nil {
					t.Errorf("%s: %v", l, err)
				}
			}
		})
	}
}
