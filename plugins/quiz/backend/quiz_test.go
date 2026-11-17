package main

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	// The core's case: choices with an explanation, then a flashcard.
	z, ps := Parse("title: Netz\n? Router?\n- 1.1\n+ 10.1\n> Seit März.\n\n? WLAN?\n= Gartenhaus")
	if len(ps) > 0 {
		t.Fatal(ps)
	}
	want := []Question{
		{Q: "Router?", Options: []Option{{"1.1", false}, {"10.1", true}}, Explain: "Seit März.", Line: 2},
		{Q: "WLAN?", Answer: "Gartenhaus", Line: 7},
	}
	if z.Title != "Netz" || !reflect.DeepEqual(z.Questions, want) {
		t.Errorf("quiz: %+v", z)
	}
	// German key, id, comments, CRLF, explanations joined, the last answer counts,
	// "title:" after the first question is no key.
	z, ps = Parse("# a comment\r\nTitel: Netz\r\nid: netz-1\r\n?Router?\r\n+10.1\r\n> one\r\n> two\r\n? WLAN\r\n= A\r\n= B")
	if len(ps) > 0 {
		t.Fatal(ps)
	}
	if z.Title != "Netz" || z.ID != "netz-1" || z.Questions[0].Q != "Router?" || z.Questions[0].Options[0].Text != "10.1" ||
		z.Questions[0].Explain != "one two" || z.Questions[1].Answer != "B" {
		t.Errorf("quiz: %+v", z)
	}
	if _, ps = Parse("? A\n+ x\ntitle: late"); len(ps) != 1 || ps[0].Line != 3 {
		t.Errorf("title after a question: %+v", ps)
	}
}

func TestProblems(t *testing.T) {
	for src, want := range map[string]Problem{
		"? A\n- x\n- y":               {1, "question 1 has no right answer (+ …)"},
		"? A":                         {1, "question 1 needs answers (+ right, - wrong) or a solution (= …)"},
		"? A\n+ x\n\n? B\n> why":      {4, "question 2 needs answers"},
		"title: Q\n# c\n":             {1, "a question is missing (? …)"},
		"? A\n+ x\nplain text":        {3, `"plain text": lines start with ?, +, -, = or >`},
		"title: T\n+ early\n? A\n+ x": {2, `"+ early" comes before the first question (? …)`},
		"oops\n? A\n+ x":              {1, `"oops": lines start with`},
	} {
		z, ps := Parse(src)
		if z != nil || len(ps) == 0 {
			t.Errorf("%q: no problem", src)
			continue
		}
		ps = sortedProblems(ps)
		if ps[0].Line != want.Line || !strings.Contains(ps[0].Message, want.Message) {
			t.Errorf("%q: %+v, want %+v", src, ps, want)
		}
	}
}

func TestSVG(t *testing.T) {
	z, _ := Parse("title: Net <b>\n? Router & <i>?\n+ a\n? Wi-Fi\n= b")
	svg := SVG(z, 600)
	for _, want := range []string{`<svg`, `viewBox="0 0 600 90"`, `width="600" height="90"`, "Net &lt;b&gt;", "1. Router &amp; &lt;i&gt;?", `y="74"`, "2. Wi-Fi", "var(--fg)", "var(--font-body"} {
		if !strings.Contains(svg, want) {
			t.Errorf("missing %q in %s", want, svg)
		}
	}
	if strings.Contains(svg, "<b>") || strings.Contains(svg, "<i>") {
		t.Error("text must be escaped")
	}
	if !strings.Contains(SVG(z, 0), `width="720"`) {
		t.Error("default width")
	}
	z.Title = ""
	if svg = SVG(z, 500); !strings.Contains(svg, `height="62"`) || !strings.Contains(svg, `<text x="6" y="22" style="font-size:13px`) {
		t.Errorf("without title: %s", svg)
	}
}

func TestManifestExample(t *testing.T) {
	raw, err := os.ReadFile("../phoedox-plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Contributes struct {
			Blocks []struct {
				Example string
				Board   bool
			} `json:"blocks"`
		} `json:"contributes"`
	}
	if err := json.Unmarshal(raw, &m); err != nil || len(m.Contributes.Blocks) != 1 {
		t.Fatal(err)
	}
	if m.Contributes.Blocks[0].Board {
		t.Error("the quiz is for pages only")
	}
	z, ps := Parse(m.Contributes.Blocks[0].Example)
	if len(ps) > 0 {
		t.Fatal(ps)
	}
	if len(z.Questions) != 2 || z.Questions[1].Answer != "Garden house" || z.Questions[0].Explain == "" {
		t.Errorf("example: %+v", z)
	}
}
