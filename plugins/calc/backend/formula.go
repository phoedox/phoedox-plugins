package main

// The formula engine of the calculator, ported from the core's
// web/src/formula.ts: numbers, text in quotes, names of other values,
// + - * / ^ & %, comparisons and a few functions – without eval. Names are
// letters, digits and _ (any script). A formula is parsed into a tree first
// (syntax errors are found before anything is computed), then evaluated with
// the same rules as JavaScript: values are float64, string, bool or nil (an
// IF without its "then" branch).

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// FormulaError is a failed formula; Code is syntax, name, value, div0 or cycle.
type FormulaError struct {
	Code   string
	Detail string
}

func (e *FormulaError) Error() string {
	if e.Detail != "" {
		return e.Detail
	}
	return e.Code
}

func ferr(code string) *FormulaError { return &FormulaError{Code: code} }

func syntaxErr(f string, a ...any) *FormulaError {
	return &FormulaError{Code: "syntax", Detail: fmt.Sprintf(f, a...)}
}

type tokKind int

const (
	tNum tokKind = iota
	tStr
	tName
	tFn
	tOp
)

type token struct {
	kind tokKind
	num  float64
	text string // the name, function (upper case), operator or string content
	src  string // as written, for messages
}

// funcs are the formula functions; args are evaluated values.
var funcs = map[string]func(a []any) (any, error){
	"SUM": func(a []any) (any, error) {
		s := 0.0
		for _, n := range nums(a) {
			s += n
		}
		return s, nil
	},
	"AVERAGE": average,
	"AVG":     average,
	"MIN": func(a []any) (any, error) {
		m := math.Inf(1)
		for _, n := range nums(a) {
			m = math.Min(m, n)
		}
		return m, nil
	},
	"MAX": func(a []any) (any, error) {
		m := math.Inf(-1)
		for _, n := range nums(a) {
			m = math.Max(m, n)
		}
		return m, nil
	},
	"ROUND": func(a []any) (any, error) {
		d := jsNumberOf(arg(a, 1))
		if math.IsNaN(d) || d == 0 {
			d = 0
		}
		f := jsPow(10, d)
		x, err := num(arg(a, 0))
		if err != nil {
			return nil, err
		}
		return jsRound(x*f) / f, nil
	},
	"CEIL":  unary(math.Ceil),
	"FLOOR": unary(math.Floor),
	"ABS":   unary(math.Abs),
	"SQRT":  unary(math.Sqrt),
	"IF": func(a []any) (any, error) {
		if truthy(arg(a, 0)) {
			return arg(a, 1), nil
		}
		if b := arg(a, 2); b != nil {
			return b, nil
		}
		return "", nil
	},
}

func average(a []any) (any, error) {
	n := nums(a)
	if len(n) == 0 {
		return 0.0, nil
	}
	s := 0.0
	for _, x := range n {
		s += x
	}
	return s / float64(len(n)), nil
}

func unary(f func(float64) float64) func([]any) (any, error) {
	return func(a []any) (any, error) {
		x, err := num(arg(a, 0))
		if err != nil {
			return nil, err
		}
		return f(x), nil
	}
}

func arg(a []any, i int) any {
	if i < len(a) {
		return a[i]
	}
	return nil
}

// num converts a value to a number like formula.ts: "" and nil are 0, text
// that is no number is a value error.
func num(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case nil:
		return 0, nil
	case string:
		if x == "" {
			return 0, nil
		}
	}
	n := jsNumberOf(v)
	if math.IsNaN(n) {
		return 0, ferr("value")
	}
	return n, nil
}

// nums are the numbers among function arguments (empty and non-numeric values are left out).
func nums(a []any) []float64 {
	var out []float64
	for _, v := range a {
		if v == nil || v == "" {
			continue
		}
		if n := jsNumberOf(v); !math.IsNaN(n) {
			out = append(out, n)
		}
	}
	return out
}

func truthy(v any) bool {
	switch x := v.(type) {
	case string:
		return x != "" && strings.ToUpper(x) != "FALSE"
	case float64:
		return x != 0 && !math.IsNaN(x)
	case bool:
		return x
	}
	return false
}

var (
	jsNumRe  = regexp.MustCompile(`^[+-]?(?:[0-9]+\.?[0-9]*|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)
	jsIntRe  = regexp.MustCompile(`^0([xXoObB])([0-9a-fA-F]+)$`)
	numTokRe = regexp.MustCompile(`^[0-9]+(?:[.,][0-9]+)?`)
)

// jsNumberOf is JavaScript's Number(v) for the values formulas use.
func jsNumberOf(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case bool:
		if x {
			return 1
		}
		return 0
	case string:
		s := strings.TrimFunc(x, isSpace)
		switch {
		case s == "":
			return 0
		case s == "Infinity" || s == "+Infinity":
			return math.Inf(1)
		case s == "-Infinity":
			return math.Inf(-1)
		case jsNumRe.MatchString(s):
			n, err := strconv.ParseFloat(s, 64)
			if err != nil && !errors.Is(err, strconv.ErrRange) {
				return math.NaN()
			}
			return n
		}
		if m := jsIntRe.FindStringSubmatch(s); m != nil {
			base := map[byte]int{'x': 16, 'o': 8, 'b': 2}[strings.ToLower(m[1])[0]]
			n, err := strconv.ParseUint(m[2], base, 64)
			if err != nil {
				return math.NaN()
			}
			return float64(n)
		}
	}
	return math.NaN()
}

// jsString is JavaScript's String(v), used for & and for comparing text.
func jsString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return jsNumString(x)
	case bool:
		return strconv.FormatBool(x)
	}
	return "undefined"
}

func jsNumString(x float64) string {
	switch {
	case math.IsNaN(x):
		return "NaN"
	case math.IsInf(x, 1):
		return "Infinity"
	case math.IsInf(x, -1):
		return "-Infinity"
	case x == 0:
		return "0"
	}
	if a := math.Abs(x); a >= 1e21 || a < 1e-6 {
		s := strconv.FormatFloat(x, 'e', -1, 64)
		mant, exp, _ := strings.Cut(s, "e")
		sign, digits := exp[:1], strings.TrimLeft(exp[1:], "0")
		return mant + "e" + sign + digits
	}
	return strconv.FormatFloat(x, 'f', -1, 64)
}

// jsRound is Math.round: halves go up (towards +∞), -0.5 ≤ x < 0 gives -0.
func jsRound(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	if x < 0 && x >= -0.5 {
		return math.Copysign(0, -1)
	}
	f := math.Floor(x)
	if x-f >= 0.5 {
		f++
	}
	return f
}

// jsPow is the ** operator (differs from math.Pow for NaN exponents and ±1 ** ±∞).
func jsPow(x, y float64) float64 {
	if math.IsNaN(y) || (math.Abs(x) == 1 && math.IsInf(y, 0)) {
		return math.NaN()
	}
	return math.Pow(x, y)
}

// isSpace is JavaScript's \s.
func isSpace(r rune) bool {
	return (unicode.IsSpace(r) && r != 0x85) || r == 0xFEFF
}

func isNameStart(r rune) bool { return r == '_' || unicode.IsLetter(r) }
func isNamePart(r rune) bool  { return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r) }

// Tokens splits a formula. A decimal comma belongs to the number ("2,5"),
// so arguments are separated by ", " or ";".
func Tokens(src string) ([]token, error) {
	var out []token
	for i := 0; i < len(src); {
		rest := src[i:]
		r, size := utf8.DecodeRuneInString(rest)
		if isSpace(r) {
			i += size
			continue
		}
		if m := numTokRe.FindString(rest); m != "" {
			n, _ := strconv.ParseFloat(strings.Replace(m, ",", ".", 1), 64)
			out = append(out, token{kind: tNum, num: n, src: m})
			i += len(m)
			continue
		}
		if r == '"' {
			end := strings.IndexByte(rest[1:], '"')
			if end < 0 {
				return nil, syntaxErr("the closing \" is missing")
			}
			out = append(out, token{kind: tStr, text: rest[1 : 1+end], src: rest[:end+2]})
			i += end + 2
			continue
		}
		if isNameStart(r) {
			j := size
			for j < len(rest) {
				r2, s2 := utf8.DecodeRuneInString(rest[j:])
				if !isNamePart(r2) {
					break
				}
				j += s2
			}
			name := rest[:j]
			up := strings.ToUpper(name)
			if _, ok := funcs[up]; ok && strings.HasPrefix(strings.TrimLeftFunc(rest[j:], isSpace), "(") {
				out = append(out, token{kind: tFn, text: up, src: name})
			} else {
				out = append(out, token{kind: tName, text: name, src: name})
			}
			i += j
			continue
		}
		op := ""
		if len(rest) >= 2 && (rest[:2] == "<=" || rest[:2] == ">=" || rest[:2] == "<>") {
			op = rest[:2]
		} else if strings.ContainsRune("-+*/^&(),;=<>%", r) {
			op = rest[:1]
		}
		if op == "" {
			return nil, syntaxErr("unexpected %q", string(r))
		}
		t := token{kind: tOp, text: op, src: op}
		if op == ";" {
			t.text = ","
		}
		out = append(out, t)
		i += len(op)
	}
	return out, nil
}

// Names returns the names a formula uses, each once, in order.
func Names(src string) ([]string, error) {
	ts, err := Tokens(src)
	if err != nil {
		return nil, err
	}
	var out []string
	seen := map[string]bool{}
	for _, t := range ts {
		if t.kind == tName && !seen[t.text] {
			seen[t.text] = true
			out = append(out, t.text)
		}
	}
	return out, nil
}

// Node is a parsed formula.
type Node struct {
	Op   string // val, name, fn, neg, pos, pct or a binary operator
	Val  any
	Name string
	Args []*Node // fn: arguments; unary: one; binary: left and right
}

// ParseFormula reads a formula into a tree (the precedence of formula.ts:
// comparisons, then + - &, then * /, then ^ (left to right), then %, then
// unary signs, brackets and function calls).
func ParseFormula(src string) (*Node, error) {
	ts, err := Tokens(src)
	if err != nil {
		return nil, err
	}
	p := &parser{ts: ts}
	n, err := p.compare()
	if err != nil {
		return nil, err
	}
	if p.i < len(ts) {
		if ts[p.i].kind == tOp && ts[p.i].text == ")" {
			return nil, syntaxErr("there is a ) without (")
		}
		return nil, syntaxErr("unexpected %q", ts[p.i].src)
	}
	return n, nil
}

type parser struct {
	ts []token
	i  int
}

// isOp reports whether the next token is one of the operators.
func (p *parser) isOp(ops ...string) bool {
	if p.i >= len(p.ts) || p.ts[p.i].kind != tOp {
		return false
	}
	for _, o := range ops {
		if p.ts[p.i].text == o {
			return true
		}
	}
	return false
}

func (p *parser) expectClose() error {
	if p.i >= len(p.ts) {
		return syntaxErr("a ) is missing")
	}
	if !p.isOp(")") {
		return syntaxErr("unexpected %q", p.ts[p.i].src)
	}
	p.i++
	return nil
}

func (p *parser) binary(next func() (*Node, error), ops ...string) (*Node, error) {
	l, err := next()
	if err != nil {
		return nil, err
	}
	for p.isOp(ops...) {
		op := p.ts[p.i].text
		p.i++
		r, err := next()
		if err != nil {
			return nil, err
		}
		l = &Node{Op: op, Args: []*Node{l, r}}
	}
	return l, nil
}

func (p *parser) compare() (*Node, error) {
	return p.binary(p.sum, "=", "<>", "<", ">", "<=", ">=")
}
func (p *parser) sum() (*Node, error)   { return p.binary(p.term, "+", "-", "&") }
func (p *parser) term() (*Node, error)  { return p.binary(p.power, "*", "/") }
func (p *parser) power() (*Node, error) { return p.binary(p.percent, "^") }

func (p *parser) percent() (*Node, error) {
	n, err := p.primary()
	if err != nil {
		return nil, err
	}
	for p.isOp("%") {
		p.i++
		n = &Node{Op: "pct", Args: []*Node{n}}
	}
	return n, nil
}

func (p *parser) primary() (*Node, error) {
	if p.i >= len(p.ts) {
		return nil, syntaxErr("the formula ends too early")
	}
	t := p.ts[p.i]
	p.i++
	switch t.kind {
	case tNum:
		return &Node{Op: "val", Val: t.num}, nil
	case tStr:
		return &Node{Op: "val", Val: t.text}, nil
	case tName:
		return &Node{Op: "name", Name: t.text}, nil
	case tFn:
		if !p.isOp("(") {
			return nil, syntaxErr("%s needs (", t.text)
		}
		p.i++
		n := &Node{Op: "fn", Name: t.text}
		if !p.isOp(")") {
			for {
				a, err := p.compare()
				if err != nil {
					return nil, err
				}
				n.Args = append(n.Args, a)
				if !p.isOp(",") {
					break
				}
				p.i++
			}
		}
		return n, p.expectClose()
	}
	switch t.text {
	case "(":
		n, err := p.compare()
		if err != nil {
			return nil, err
		}
		return n, p.expectClose()
	case "-", "+":
		x, err := p.power()
		if err != nil {
			return nil, err
		}
		op := "neg"
		if t.text == "+" {
			op = "pos"
		}
		return &Node{Op: op, Args: []*Node{x}}, nil
	}
	return nil, syntaxErr("unexpected %q", t.src)
}

// Getter gives the value of another name; unknown names are FormulaError "name".
type Getter func(name string) (any, error)

// Eval computes a parsed formula.
func (n *Node) Eval(get Getter) (any, error) {
	switch n.Op {
	case "val":
		return n.Val, nil
	case "name":
		return get(n.Name)
	case "fn":
		args := make([]any, len(n.Args))
		for i, a := range n.Args {
			v, err := a.Eval(get)
			if err != nil {
				return nil, err
			}
			args[i] = v
		}
		return funcs[n.Name](args)
	case "neg", "pos", "pct":
		v, err := n.Args[0].Eval(get)
		if err != nil {
			return nil, err
		}
		x, err := num(v)
		if err != nil {
			return nil, err
		}
		switch n.Op {
		case "neg":
			return -x, nil
		case "pct":
			return x / 100, nil
		}
		return x, nil
	}
	l, err := n.Args[0].Eval(get)
	if err != nil {
		return nil, err
	}
	switch n.Op {
	case "^":
		a, err := num(l)
		if err != nil {
			return nil, err
		}
		r, err := n.Args[1].Eval(get)
		if err != nil {
			return nil, err
		}
		b, err := num(r)
		if err != nil {
			return nil, err
		}
		return jsPow(a, b), nil
	case "*", "/":
		r, err := n.Args[1].Eval(get)
		if err != nil {
			return nil, err
		}
		b, err := num(r)
		if err != nil {
			return nil, err
		}
		if n.Op == "/" && b == 0 {
			return nil, ferr("div0")
		}
		a, err := num(l)
		if err != nil {
			return nil, err
		}
		if n.Op == "*" {
			return a * b, nil
		}
		return a / b, nil
	}
	r, err := n.Args[1].Eval(get)
	if err != nil {
		return nil, err
	}
	switch n.Op {
	case "&":
		return jsString(l) + jsString(r), nil
	case "+", "-":
		a, err := num(l)
		if err != nil {
			return nil, err
		}
		b, err := num(r)
		if err != nil {
			return nil, err
		}
		if n.Op == "+" {
			return a + b, nil
		}
		return a - b, nil
	}
	// Comparisons: as numbers when one side is a number, else as text.
	_, ln := l.(float64)
	_, rn := r.(float64)
	if ln || rn {
		a, err := num(l)
		if err != nil {
			return nil, err
		}
		b, err := num(r)
		if err != nil {
			return nil, err
		}
		return cmp(n.Op, a == b, a < b, a > b), nil
	}
	a, b := jsString(l), jsString(r)
	return cmp(n.Op, a == b, a < b, a > b), nil
}

func cmp(op string, eq, lt, gt bool) bool {
	switch op {
	case "=":
		return eq
	case "<>":
		return !eq
	case "<":
		return lt
	case ">":
		return gt
	case "<=":
		return lt || eq
	}
	return gt || eq // >=
}

// Evaluate parses and computes a formula.
func Evaluate(src string, get Getter) (any, error) {
	n, err := ParseFormula(src)
	if err != nil {
		return nil, err
	}
	return n.Eval(get)
}

// names lists the names a parsed formula uses, each once, in order.
func (n *Node) names(seen map[string]bool, out []string) []string {
	if n.Op == "name" && !seen[n.Name] {
		seen[n.Name] = true
		out = append(out, n.Name)
	}
	for _, a := range n.Args {
		out = a.names(seen, out)
	}
	return out
}
