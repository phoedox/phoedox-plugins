// Package main is the quiz block (ADR-0087): questions with right and wrong
// answers ("? question", "+ right", "- wrong") or flashcards ("= answer"),
// with an explanation after answering ("> …"). block_render checks the source
// and draws the questions as SVG; the frontend makes the quiz playable. The
// logic has no build tag so it is tested natively; it reads the source like
// the frontend does.
package main

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Option is an answer to choose; Right marks the right ones.
type Option struct {
	Text  string
	Right bool
}

// Question is one question: options to choose from, or a flashcard answer to
// reveal (Answer). Explain is shown after answering. Line counts from 1.
type Question struct {
	Q       string
	Options []Option
	Answer  string
	Explain string
	Line    int
}

// Quiz is a parsed quiz; ID is the source's own "id:" (the key of the
// readers' answers when it is valid).
type Quiz struct {
	Title     string
	ID        string
	Questions []Question
}

// Problem is a finding in the source; Line counts from 1.
type Problem struct {
	Line    int
	Message string
}

var (
	keyRe  = regexp.MustCompile(`^([A-Za-z]+)\s*:\s*(.*)$`)
	markRe = regexp.MustCompile(`^([?+\-=>])\s*(.*)$`)
	keys   = map[string]bool{"title": true, "titel": true, "id": true}
)

// Parse reads a quiz; problems name the line they are on. With problems the
// quiz is nil.
func Parse(src string) (*Quiz, []Problem) {
	z := &Quiz{}
	var ps []Problem
	var cur *Question
	for i, raw := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		n := i + 1
		l := strings.TrimSpace(raw)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		// Keys only before the first question.
		if cur == nil {
			if kv := keyRe.FindStringSubmatch(l); kv != nil && keys[strings.ToLower(kv[1])] {
				if v := strings.TrimSpace(kv[2]); strings.ToLower(kv[1]) == "id" {
					z.ID = v
				} else {
					z.Title = v
				}
				continue
			}
		}
		m := markRe.FindStringSubmatch(l)
		if m == nil {
			ps = append(ps, Problem{n, fmt.Sprintf("%q: lines start with ?, +, -, = or >", l)})
			continue
		}
		mark, text := m[1], m[2]
		if mark == "?" {
			z.Questions = append(z.Questions, Question{Q: text, Line: n})
			cur = &z.Questions[len(z.Questions)-1]
			continue
		}
		if cur == nil {
			ps = append(ps, Problem{n, fmt.Sprintf("%q comes before the first question (? …)", l)})
			continue
		}
		switch mark {
		case "+", "-":
			cur.Options = append(cur.Options, Option{Text: text, Right: mark == "+"})
		case "=":
			cur.Answer = text
		default:
			if cur.Explain != "" {
				cur.Explain += " " + text
			} else {
				cur.Explain = text
			}
		}
	}
	if len(z.Questions) == 0 {
		ps = append(ps, Problem{1, "a question is missing (? …)"})
	}
	for i, q := range z.Questions {
		switch {
		case len(q.Options) == 0 && q.Answer == "":
			ps = append(ps, Problem{q.Line, fmt.Sprintf("question %d needs answers (+ right, - wrong) or a solution (= …)", i+1)})
		case len(q.Options) > 0 && !q.hasRight():
			ps = append(ps, Problem{q.Line, fmt.Sprintf("question %d has no right answer (+ …)", i+1)})
		}
	}
	if len(ps) > 0 {
		return nil, ps
	}
	return z, nil
}

func (q Question) hasRight() bool {
	for _, o := range q.Options {
		if o.Right {
			return true
		}
	}
	return false
}

// SVG lists the questions (the picture outside the web app); width 0 means 720.
func SVG(z *Quiz, width int) string {
	if width <= 0 {
		width = 720
	}
	esc := html.EscapeString
	top := 6
	if z.Title != "" {
		top = 34
	}
	h := top + len(z.Questions)*24 + 8
	w, hs := strconv.Itoa(width), strconv.Itoa(h)
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %s %s" width="%s" height="%s" style="font-family:var(--font-body,system-ui,sans-serif)">`, w, hs, w, hs)
	if z.Title != "" {
		fmt.Fprintf(&sb, `<text x="6" y="22" style="font-size:16px;font-weight:600;fill:var(--fg)">%s</text>`, esc(z.Title))
	}
	for i, q := range z.Questions {
		fmt.Fprintf(&sb, `<text x="6" y="%d" style="font-size:13px;fill:var(--fg)">%d. %s</text>`, top+i*24+16, i+1, esc(q.Q))
	}
	sb.WriteString(`</svg>`)
	return sb.String()
}

// sortedProblems puts problems in line order.
func sortedProblems(ps []Problem) []Problem {
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].Line < ps[j].Line })
	return ps
}
