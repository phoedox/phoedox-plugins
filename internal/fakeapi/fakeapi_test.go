package fakeapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServe(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"a":1}`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "routes.json"), []byte(`{"routes":[
		{"path":"/x/*","file":"a.json"},
		{"method":"POST","path":"/login","status":204,"headers":{"Set-Cookie":"sid=demo"}},
		{"path":"/rpc","query":"m=status","body":"ok"},
		{"path":"/inline","json":{"b":2}},
		{"method":"POST","path":"/chat","seq":["t1.sse","t2.sse"]},
		{"method":"POST","path":"/rpc","bodyContains":"\"host.get\"","body":"hosts"},
		{"method":"POST","path":"/rpc","body":"other"}]}`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "t1.sse"), []byte("data: one\n\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "t2.sse"), []byte("data: two\n\n"), 0o644)
	api, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(api)
	defer srv.Close()
	get := func(method, p string) (int, string, http.Header) {
		req, _ := http.NewRequest(method, srv.URL+p, nil)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b), res.Header
	}
	if c, b, h := get("GET", "/x/1"); c != 200 || b != `{"a":1}` || h.Get("Content-Type") != "application/json" {
		t.Fatalf("file: %d %s", c, b)
	}
	if c, _, h := get("POST", "/login"); c != 204 || h.Get("Set-Cookie") != "sid=demo" {
		t.Fatalf("status/headers: %d", c)
	}
	if c, b, _ := get("GET", "/rpc?m=status&x=1"); c != 200 || b != "ok" {
		t.Fatalf("query: %d %s", c, b)
	}
	if c, _, _ := get("GET", "/rpc?m=other"); c != 404 {
		t.Fatalf("query mismatch: %d", c)
	}
	if c, _, _ := get("GET", "/x/1/2"); c != 404 {
		t.Fatalf("* is one segment: %d", c)
	}
	if b, err := Body(dir, "GET", "/inline"); err != nil || string(b) != `{"b":2}` {
		t.Fatalf("body: %v %s", err, b)
	}
	post := func(body string) string {
		res, err := http.Post(srv.URL+"/rpc", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return string(b)
	}
	if post(`{"method":"host.get"}`) != "hosts" || post(`{"method":"problem.get"}`) != "other" {
		t.Fatal("bodyContains")
	}
	if b, _ := BodyFor(dir, "POST", "/rpc", `{"method":"host.get"}`); string(b) != "hosts" {
		t.Fatalf("BodyFor: %s", b)
	}
	for _, want := range []string{"one", "two", "two"} {
		if c, b, h := get("POST", "/chat"); c != 200 || b != "data: "+want+"\n\n" || h.Get("Content-Type") != "text/event-stream" {
			t.Fatalf("seq: %d %q", c, b)
		}
	}
}
