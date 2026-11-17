package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	// The core's cases (web/src/interactive-blocks.test.ts).
	p, ps := Parse("question: Wohin?\n- Zoo\n- Museum\nmultiple: ja\nid: ausflug")
	want := &Poll{Question: "Wohin?", Options: []string{"Zoo", "Museum"}, Multiple: true, Results: "after", ID: "ausflug"}
	if len(ps) > 0 || !reflect.DeepEqual(p, want) {
		t.Errorf("poll: %+v %v", p, ps)
	}
	p, ps = Parse("multiple\nWas nehmen wir mit?\n- Picknick\n- Ball\nclosed")
	if len(ps) > 0 || p.Question != "Was nehmen wir mit?" || !p.Multiple || !p.Closed {
		t.Errorf("flags on their own line: %+v %v", p, ps)
	}
	// German key, comments, other list marks, results, flags with values.
	p, ps = Parse("# a comment\r\nFrage: Wann?\r\n* Morgen\r\n+  Übermorgen  \r\nresults: immer\r\nclosed: nein\r\nMULTIPLE: off")
	if len(ps) > 0 || p.Question != "Wann?" || !reflect.DeepEqual(p.Options, []string{"Morgen", "Übermorgen"}) || p.Results != "always" || p.Closed || p.Multiple {
		t.Errorf("german: %+v %v", p, ps)
	}
	p, _ = Parse("Q\n- a\n- b\nresults: after")
	if p.Results != "after" {
		t.Errorf("results after: %q", p.Results)
	}
	p, _ = Parse("Q\n- a\n- b\nresults: never")
	if p.Results != "after" {
		t.Errorf("unknown results: %q", p.Results)
	}
	// A line that is neither key, flag nor answer is the question only once.
	p, _ = Parse("First\nSecond: line\n- a\n- b")
	if p.Question != "First" {
		t.Errorf("question: %q", p.Question)
	}
	p, _ = Parse("question: Q\n- a\n- b\nquestion: Later")
	if p.Question != "Later" {
		t.Errorf("the key wins: %q", p.Question)
	}
}

func TestProblems(t *testing.T) {
	many := "Q"
	for i := 1; i <= 31; i++ {
		many += fmt.Sprintf("\n- %d", i)
	}
	for src, want := range map[string]Problem{
		"- a\n- b":                   {1, "the question is missing (question: …)"},
		"Wohin?\n- Zoo":              {1, "a poll needs at least two answers (- …)"},
		"Wohin?\n- Zoo\n\n- Zoo":     {4, `the answer "Zoo" is there twice`},
		"Wohin?\n- a\n- b\n- a\n- b": {4, `the answer "a" is there twice`},
		many:                         {32, "at most 30 answers"},
		"":                           {1, "the question is missing"},
	} {
		p, ps := Parse(src)
		if p != nil || len(ps) == 0 {
			t.Errorf("%q: no problem", src)
			continue
		}
		ps = sortedProblems(ps)
		found := false
		for _, x := range ps {
			if x.Line == want.Line && strings.Contains(x.Message, want.Message) {
				found = true
			}
		}
		if !found {
			t.Errorf("%q: %+v, want %+v", src, ps, want)
		}
	}
	if _, ps := Parse(""); len(ps) != 2 {
		t.Errorf("empty: %v", ps)
	}
}

func TestSVG(t *testing.T) {
	p, _ := Parse("question: Where <to>?\n- Zoo & more\n- \"Museum\"\n- Pool")
	svg := SVG(p, 400)
	for _, want := range []string{`<svg`, `viewBox="0 0 400 152"`, "Where &lt;to&gt;?", "Zoo &amp; more", "&#34;Museum&#34;", "var(--surface2)", "var(--fg)", "var(--font-body", `width="388"`} {
		if !strings.Contains(svg, want) {
			t.Errorf("missing %q in %s", want, svg)
		}
	}
	if strings.Contains(svg, "<to>") {
		t.Error("text must be escaped")
	}
	if strings.Count(svg, "<rect") != 3 {
		t.Error("one bar per answer")
	}
	if strings.Contains(svg, "var(--tb-blue)") {
		t.Error("no votes in the picture")
	}
	if !strings.Contains(SVG(p, 0), `width="420"`) {
		t.Error("default width")
	}
	if !strings.HasSuffix(svg, "</svg>") {
		t.Error("closed svg")
	}
}
