// Package fakeapi serves recorded answers of a third-party API from a
// plugin's testdata/fake/routes.json, with fictional data only (the demo
// household of the concept "Plugin logos and screenshots"). The same files
// feed the plugin's Go tests (Body) and the screenshot runner
// (tools/fakeapi, scripts/shots.mjs), so test data and screenshots match.
//
// routes.json:
//
//	{"routes": [
//	  {"method": "GET", "path": "/api2/json/cluster/resources", "file": "resources.json"},
//	  {"path": "/api/v1/hosts/*", "json": {"ok": true}},
//	  {"method": "POST", "path": "/login", "status": 204, "headers": {"Set-Cookie": "sid=demo"}},
//	  {"path": "/rpc", "query": "method=status", "body": "plain text"},
//	  {"method": "POST", "path": "/v1/chat/completions", "seq": ["turn1.json", "turn2.json"]}
//	]}
//
// method defaults to GET, status to 200. path is matched with path.Match
// (* stands for one path segment); query, when set, must be contained in
// the raw query. The first matching route wins. file is relative to the
// folder of routes.json; json is an inline answer; body a plain one; seq
// lists files answered one after the other (the last one repeats), e.g. the
// turns of a recorded AI conversation. A file ending in .sse is sent as
// text/event-stream.
package fakeapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

// Route is one recorded answer.
type Route struct {
	Method  string            `json:"method,omitempty"`
	Path    string            `json:"path"`
	Query   string            `json:"query,omitempty"`
	Status  int               `json:"status,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	File    string            `json:"file,omitempty"`
	JSON    json.RawMessage   `json:"json,omitempty"`
	Body    string            `json:"body,omitempty"`
	Seq     []string          `json:"seq,omitempty"`
}

// API is a loaded routes.json.
type API struct {
	Routes []Route `json:"routes"`
	dir    string
	mu     sync.Mutex
	calls  map[int]int
}

// Load reads dir/routes.json.
func Load(dir string) (*API, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "routes.json"))
	if err != nil {
		return nil, err
	}
	a := &API{dir: dir}
	if err := json.Unmarshal(raw, a); err != nil {
		return nil, fmt.Errorf("%s/routes.json: %w", dir, err)
	}
	for i, r := range a.Routes {
		if r.Method == "" {
			a.Routes[i].Method = http.MethodGet
		}
		if r.Status == 0 {
			a.Routes[i].Status = http.StatusOK
		}
		if _, err := path.Match(r.Path, "/"); err != nil || !strings.HasPrefix(r.Path, "/") {
			return nil, fmt.Errorf("%s/routes.json: bad path %q", dir, r.Path)
		}
	}
	return a, nil
}

// Find returns the route for a request.
func (a *API) Find(method, p, rawQuery string) (Route, bool) {
	r, _, ok := a.find(method, p, rawQuery)
	return r, ok
}

func (a *API) find(method, p, rawQuery string) (Route, int, bool) {
	for i, r := range a.Routes {
		if r.Method != method {
			continue
		}
		if ok, _ := path.Match(r.Path, p); !ok {
			continue
		}
		if r.Query != "" && !strings.Contains(rawQuery, r.Query) {
			continue
		}
		return r, i, true
	}
	return Route{}, -1, false
}

// next picks the file of a seq route for this call.
func (a *API) next(i int, r Route) Route {
	if len(r.Seq) == 0 {
		return r
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.calls == nil {
		a.calls = map[int]int{}
	}
	n := min(a.calls[i], len(r.Seq)-1)
	a.calls[i]++
	r.File = r.Seq[n]
	return r
}

// Content is the body of a route.
func (a *API) Content(r Route) ([]byte, error) {
	switch {
	case r.File != "":
		return os.ReadFile(filepath.Join(a.dir, filepath.FromSlash(r.File)))
	case len(r.JSON) > 0:
		return r.JSON, nil
	default:
		return []byte(r.Body), nil
	}
}

// ServeHTTP answers with the matching route, 404 otherwise.
func (a *API) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r, i, ok := a.find(req.Method, req.URL.Path, req.URL.RawQuery)
	r = a.next(i, r)
	if !ok {
		http.Error(w, "fakeapi: no route for "+req.Method+" "+req.URL.Path, http.StatusNotFound)
		return
	}
	b, err := a.Content(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	switch {
	case r.File != "" && strings.HasSuffix(r.File, ".sse"):
		w.Header().Set("Content-Type", "text/event-stream")
	case r.File != "" && strings.HasSuffix(r.File, ".json") || len(r.JSON) > 0:
		w.Header().Set("Content-Type", "application/json")
	}
	for k, v := range r.Headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(r.Status)
	_, _ = w.Write(b)
}

// Body returns the recorded answer for method and path (tests).
func Body(dir, method, p string) ([]byte, error) {
	a, err := Load(dir)
	if err != nil {
		return nil, err
	}
	u := strings.SplitN(p, "?", 2)
	q := ""
	if len(u) == 2 {
		q = u[1]
	}
	r, i, ok := a.find(method, u[0], q)
	if !ok {
		return nil, fmt.Errorf("fakeapi: no route for %s %s in %s", method, p, dir)
	}
	return a.Content(a.next(i, r))
}
