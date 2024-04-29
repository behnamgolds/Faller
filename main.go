package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/quic-go/quic-go/http3"
)

func main() {

	// Load config file
	c := LoadConfig()

	h3addr := c.H3Addr
	h1addr := c.H1Addr
	servername := c.ServerName
	cert := c.CertPath
	key := c.KeyPath
	scheme := c.Scheme

	log.Println("server listening on " + h3addr)

	// Generate TLS config for HTTP/3 server
	tconf := tls.Config{Rand: rand.Reader, ServerName: servername, NextProtos: []string{"h3", "h2", "http/1.1"}}

	// HTTP/3 Server
	// "QuicConfig: nil" refers to the default configuration for QUIC
	// Handler refers to incoming HTTP request handler
	server := http3.Server{
		Addr:       h3addr,
		QUICConfig: nil,
		TLSConfig:  &tconf,
		Handler:    H3Handler(h1addr, h3addr, scheme),
	}

	defer server.Close()

	// Start Listening

	// behnamgolds: this is much cleaner IMO
	log.Fatalln(server.ListenAndServeTLS(cert, key))

}

// Handle HTTP Request
func H3Handler(H1Addr string, H3Addr string, scheme string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		h1Client := &http.Client{Transport: tr}
		h1req := http.Request{Method: r.Method, URL: &url.URL{Scheme: scheme, Host: H1Addr, Path: r.URL.Path}}
		// behnamgolds:  here we add a 500 ms deadline(it is not timeout since it is another function you can check it out in the documentation) (from when the req is recieved)
		// we get ctx context with set deadline and a cancel function, so we can close it manually sooner than the set deadline if we wanted to.
		ctx, cancel := context.WithDeadline(r.Context(), time.Now().Add(500*time.Microsecond))
		// cancels the request whenever we return from the HandleFunc
		defer cancel()

		h1req = *h1req.WithContext(ctx)

		// Set H3 headers to H1 Agent
		h1Header := http.Header{}
		for h1, v1 := range r.Header {
			h1Header.Add(h1, strings.Join(v1, ";"))
		}
		h1req.Header = h1Header
		h1req.Body = r.Body
		response, h1_err := h1Client.Do(&h1req)
		if h1_err != nil {
			log.Println(h1_err.Error())
			w.WriteHeader(500)
			return
		}

		// Set HTTP/3 Response
		h3Headers := w.Header()
		for h, v := range response.Header {
			h3Headers.Add(h, strings.Join(v, ";"))
		}

		defer response.Body.Close()
		_, e := io.Copy(w, response.Body)
		if e != nil {
			log.Println(e.Error())
			return
		}
	})

	return mux
}
