package main

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

func getter(v map[string]any) Getter {
	return func(n string) (any, error) {
		if x, ok := v[n]; ok {
			return x, nil
		}
		return nil, ferr("name")
	}
}

func code(err error) string {
	if fe, ok := err.(*FormulaError); ok {
		return fe.Code
	}
	return ""
}

// The cases of the core's web/src/interactive-blocks.test.ts and more.
func TestFormula(t *testing.T) {
	get := getter(map[string]any{"Leistung": 25.0, "Stunden_pro_Tag": 24.0, "Preis": 0.32, "Größe": 3.0})
	for src, want := range map[string]any{
		"Leistung * Stunden_pro_Tag * 365 / 1000 * Preis": 70.08,
		"ROUND(10/3; 1)":                     3.3,
		"19%*100":                            19.0,
		`IF(Leistung > 20, "viel", "wenig")`: "viel",
		`IF(Leistung < 20, "viel")`:          "",
		"2,5 * 2":                            5.0,
		"1 + 2 * 3":                          7.0,
		"(1 + 2) * 3":                        9.0,
		"2 ^ 3 ^ 2":                          64.0, // left to right, as formula.ts
		"-2 ^ 2":                             -4.0, // the sign takes the power
		"10 - 2 - 3":                         5.0,
		"50%":                                0.5,
		"SUM(1, 2; 3)":                       6.0,
		"SUM(1,2)":                           1.2, // a comma between digits is a decimal comma
		`SUM(1, "x", "", "4")`:               5.0,
		"AVERAGE(2, 4)":                      3.0,
		"avg(2, 4, 6)":                       4.0,
		"AVERAGE()":                          0.0,
		"MIN(3, 1, 2)":                       1.0,
		"MAX(3, 1, 2)":                       3.0,
		"MIN()":                              math.Inf(1),
		"ROUND(2.5)":                         3.0,
		"ROUND(-2.5)":                        -2.0,
		"ROUND(1234.5; -2)":                  1200.0,
		"CEIL(1.2) + FLOOR(1.8) + ABS(-2)":   5.0,
		"SQRT(16)":                           4.0,
		`"a" & 1 & "b"`:                      "a1b",
		`0.1 + 0.2 & ""`:                     "0.30000000000000004",
		"1 = 1":                              true,
		"2 <> 2":                             false,
		"2 <= 2":                             true,
		`"10" > 9`:                           true, // a number on one side: compared as numbers
		`"b" > "a"`:                          true,
		`"10" < "9"`:                         true, // text with text: compared as text
		"Größe * 2":                          6.0,
	} {
		got, err := Evaluate(src, get)
		if err != nil {
			t.Errorf("%s: %v", src, err)
			continue
		}
		if f, ok := want.(float64); ok {
			g, ok := got.(float64)
			if !ok || (math.Abs(g-f) > 1e-9 && !(math.IsInf(f, 0) && g == f)) {
				t.Errorf("%s = %v, want %v", src, got, want)
			}
			continue
		}
		if got != want {
			t.Errorf("%s = %#v, want %#v", src, got, want)
		}
	}
	for src, want := range map[string]string{
		"1/0":           "div0",
		`"x"/0`:         "div0", // the divisor is checked first, as in formula.ts
		"Unbekannt + 1": "name",
		`"abc" * 2`:     "value",
		"1 +":           "syntax",
		"(1 + 2":        "syntax",
		"1 + 2)":        "syntax",
		"1 2":           "syntax",
		"1 $ 2":         "syntax",
		`"open`:         "syntax",
		"SUM(1, 2":      "syntax",
		"FOO(1)":        "syntax", // FOO is a name, then a bracket follows
		`"(" 1 ")"`:     "syntax", // text is never an operator
		`IF(1, 2, 1/0)`: "div0",   // all arguments are computed
	} {
		_, err := Evaluate(src, get)
		if code(err) != want {
			t.Errorf("%s: %v, want %s", src, err, want)
		}
	}
	names, err := Names("Größe * Anzahl + SUM(a, b) + a")
	if err != nil || !reflect.DeepEqual(names, []string{"Größe", "Anzahl", "a", "b"}) {
		t.Errorf("names: %v %v", names, err)
	}
	if _, err := ParseFormula("(1 + 2"); err == nil || !strings.Contains(err.Error(), ") is missing") {
		t.Errorf("message: %v", err)
	}
	if v, _ := Evaluate("SQRT(-1)", get); !math.IsNaN(v.(float64)) {
		t.Errorf("sqrt(-1): %v", v)
	}
}

func TestJS(t *testing.T) {
	for in, want := range map[float64]string{1e21: "1e+21", 1e-7: "1e-7", -1.5e-7: "-1.5e-7", 123.25: "123.25", 0: "0"} {
		if got := jsNumString(in); got != want {
			t.Errorf("%v: %s, want %s", in, got, want)
		}
	}
	for in, want := range map[string]float64{" 12 ": 12, "1e3": 1000, "0x10": 16, ".5": 0.5, "Infinity": math.Inf(1)} {
		if got := jsNumberOf(in); got != want {
			t.Errorf("%q: %v, want %v", in, got, want)
		}
	}
	if !math.IsNaN(jsNumberOf("12a")) || !math.Signbit(jsRound(-0.4)) {
		t.Error("NaN / -0")
	}
}

const example = `title: Strom
# a comment
Leistung (W) = 25
Preis (€/kWh) = 0,32
Kosten pro Jahr (€) = Leistung * 24 * 365 / 1000 * Preis
Pro Monat (€) = Kosten_pro_Jahr / 12
NAS (W) = @nas.power_w
Groß = 1 000,5`

func TestParse(t *testing.T) {
	if n, u := CalcName("Kosten pro Jahr (€)"); n != "Kosten_pro_Jahr" || u != "€" {
		t.Errorf("name: %s %s", n, u)
	}
	if n, u := CalcName("  -Größe/Tag- "); n != "Größe_Tag" || u != "" {
		t.Errorf("name: %s %s", n, u)
	}
	c, ps := Parse(example)
	if len(ps) > 0 {
		t.Fatal(ps)
	}
	if c.Title != "Strom" || len(c.Rows) != 6 {
		t.Fatalf("%+v", c)
	}
	nas := c.Rows[4]
	if !nas.Input || nas.Entity == nil || *nas.Entity != (Entity{"nas", "power_w"}) || nas.Line != 7 {
		t.Errorf("entry input: %+v", nas)
	}
	if r := c.Rows[5]; r.Value != 1000.5 || !r.Input {
		t.Errorf("spaces in numbers: %+v", r)
	}
	v := Values(c, nil)
	if math.Abs(v["Kosten_pro_Jahr"].(float64)-70.08) > 1e-9 || v["NAS"] != 0.0 {
		t.Errorf("values: %v", v)
	}
	if m := Values(c, map[string]float64{"Leistung": 50})["Pro_Monat"].(float64); math.Abs(m-11.68) > 1e-9 {
		t.Errorf("changed input: %v", m)
	}
	for src, want := range map[string]Problem{
		"A = B + 1\nB = A * 2":     {1, `"A" depends on itself`},
		"A = 1\nB = C\nC = B + A":  {2, `"B" depends on itself`},
		"A = C + 1":                {1, `"A" uses "C", which does not exist`},
		"nur Text":                 {1, `"nur Text": a line needs label = value or formula`},
		"A = 1\n= 2":               {2, `"= 2": a line needs`},
		"A =":                      {1, `"A =": a line needs`},
		"\n\nA = 1\nA = 2":         {4, `the name "A" is there twice`},
		"A = 1\nB = (A + 2":        {2, `the formula of "B" cannot be read: a ) is missing`},
		"A = 1\nB = A $ 2":         {2, `the formula of "B" cannot be read: unexpected "$"`},
		"title: Leer\n# nothing":   {1, "the calculator is empty"},
		"A = 1\nB = SUM(A, Größe)": {2, `"B" uses "Größe"`},
	} {
		_, ps := Parse(src)
		if len(ps) == 0 || ps[0].Line != want.Line || !strings.HasPrefix(ps[0].Message, want.Message) {
			t.Errorf("%q: %v, want %v", src, ps, want)
		}
	}
	if _, ps := Parse("A = 1\nB = SUM(A, 2)\nC = IF(B > 2, \"ok\", \"no\")"); len(ps) > 0 {
		t.Errorf("valid: %v", ps)
	}
	// Errors that only show when computing stay values, not problems.
	c, ps = Parse("A = 0\nB = 1 / A\nC = B + 1")
	if len(ps) > 0 {
		t.Fatal(ps)
	}
	v = Values(c, nil)
	if code(v["B"].(error)) != "div0" || code(v["C"].(error)) != "div0" {
		t.Errorf("div0: %v", v)
	}
	if got := Values(c, map[string]float64{"A": 4})["C"]; got != 1.25 {
		t.Errorf("C: %v", got)
	}
}

func TestFormatValue(t *testing.T) {
	for _, tc := range []struct {
		v        any
		loc, out string
	}{
		{1234.567, "de", "1.235"},
		{5.678, "de", "5,68"},
		{5.678, "en", "5.68"},
		{1234567.0, "en", "1,234,567"},
		{1234567.0, "de-AT", "1.234.567"},
		{0.125, "en", "0.13"}, // halves away from zero
		{-0.125, "en", "-0.13"},
		{2.675, "en", "2.67"}, // 2.675 is a little less in binary
		{99.995, "en", "100"},
		{99.5, "en", "99.5"},
		{100.5, "en", "101"},
		{-1234.4, "de", "-1.234"},
		{0.001, "en", "0"},
		{-0.001, "en", "-0"},
		{1e21, "en", "1,000,000,000,000,000,000,000"},
		{math.Inf(1), "en", "∞"},
		{math.NaN(), "de", "NaN"},
		{true, "en", "✓"},
		{false, "de", "✗"},
		{"text", "de", "text"},
		{nil, "de", ""},
		{ferr("div0"), "de", "#DIV/0"},
		{ferr("cycle"), "de", "#ZYKLUS"},
		{ferr("cycle"), "en", "#CYCLE"},
		{ferr("value"), "de", "#FEHLER"},
		{ferr("syntax"), "en", "#ERROR"},
	} {
		if got := FormatValue(tc.v, tc.loc); got != tc.out {
			t.Errorf("%v (%s): %q, want %q", tc.v, tc.loc, got, tc.out)
		}
	}
}

func TestSVG(t *testing.T) {
	c, _ := Parse(example + "\nX <script> (\"&\") = Leistung / 0")
	svg := SVG(c, 420, "de")
	for _, want := range []string{"<svg", `viewBox="0 0 420 236"`, ">Strom<", "70,08 €", "5,84 €", "25 W", "– W", "1.001", "#DIV/0 &#34;&amp;&#34;", "X &lt;script&gt;", "font-weight:600", "var(--muted)"} {
		if !strings.Contains(svg, want) {
			t.Errorf("missing %q in %s", want, svg)
		}
	}
	if strings.Contains(svg, "<script") {
		t.Error("no scripts")
	}
	if en := SVG(c, 0, "en"); !strings.Contains(en, "70.08 €") || !strings.Contains(en, `width="480"`) {
		t.Error("english / default width")
	}
	c, _ = Parse("A = 1")
	if svg := SVG(c, 100, "en"); !strings.Contains(svg, `viewBox="0 0 240 40"`) || strings.Contains(svg, "font-size:16px") {
		t.Errorf("no title, minimum width: %s", svg)
	}
}

func TestManifestExample(t *testing.T) {
	raw, err := os.ReadFile("../phoedox-plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Contributes struct {
			Blocks []struct{ Example string } `json:"blocks"`
		} `json:"contributes"`
	}
	if err := json.Unmarshal(raw, &m); err != nil || len(m.Contributes.Blocks) != 1 {
		t.Fatal(err)
	}
	c, ps := Parse(m.Contributes.Blocks[0].Example)
	if len(ps) > 0 {
		t.Fatal(ps)
	}
	if got := FormatValue(Values(c, nil)["Cost_per_month"], "en"); got != "5.84" {
		t.Errorf("cost per month: %s", got)
	}
}
