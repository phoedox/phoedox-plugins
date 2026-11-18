package main

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"
)

// TestDemoData parses the blocks the screenshots show (shots.json), so the
// store pictures never show a block with problems.
func TestDemoData(t *testing.T) {
	srcs := demoBlocks(t)
	if len(srcs) != 2 {
		t.Fatalf("%d poll blocks in shots.json", len(srcs))
	}
	for _, src := range srcs {
		p, problems := Parse(src)
		if len(problems) > 0 || len(p.Options) < 3 {
			t.Fatalf("%v %v\n%s", problems, p, src)
		}
	}
}

// demoBlocks returns the sources of the poll blocks on the pages and
// whiteboards that shots.json creates for the screenshots.
func demoBlocks(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("../shots.json")
	if err != nil {
		t.Fatal(err)
	}
	var plan struct {
		Setup []struct {
			Body struct {
				Markdown   string `json:"markdown"`
				Whiteboard *struct {
					Elements []struct {
						Type   string `json:"type"`
						Kind   string `json:"kind"`
						Source string `json:"source"`
					} `json:"elements"`
				} `json:"whiteboard"`
			} `json:"body"`
		} `json:"setup"`
	}
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	fence := regexp.MustCompile("(?s)```poll\n(.*?)\n```")
	var out []string
	for _, s := range plan.Setup {
		for _, m := range fence.FindAllStringSubmatch(s.Body.Markdown, -1) {
			out = append(out, m[1])
		}
		if wb := s.Body.Whiteboard; wb != nil {
			for _, e := range wb.Elements {
				if e.Type == "block" && e.Kind == "poll" {
					out = append(out, e.Source)
				}
			}
		}
	}
	return out
}
