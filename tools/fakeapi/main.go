// Command fakeapi serves a plugin's recorded API answers
// (testdata/fake/routes.json, see internal/fakeapi) for screenshots and
// manual tests:
//
//	go run ./tools/fakeapi -addr 127.0.0.1:18191 plugins/proxmox/testdata/fake
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/phoedox/phoedox-plugins/internal/fakeapi"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:0", "listen address")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: fakeapi [-addr host:port] plugins/<name>/testdata/fake")
		os.Exit(2)
	}
	api, err := fakeapi.Load(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	// The runner reads this line to learn the address.
	fmt.Printf("fakeapi listening on http://%s\n", ln.Addr())
	log.Fatal(http.Serve(ln, logged(api)))
}

func logged(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.RequestURI())
		h.ServeHTTP(w, r)
	})
}
