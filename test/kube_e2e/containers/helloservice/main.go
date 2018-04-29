package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

type handler struct{}

var hello string

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/text")
	if _, err := w.Write([]byte(hello)); err != nil {
		log.Printf("err: %v", err)
	}
}

func serve(port uint) {
	log.Printf("upstream listening HTTP1.1 on %d", port)
	h := handler{}
	server := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: h}
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func main() {
	port := flag.Uint("p", 8080, "listener port")
	flag.StringVar(&hello, "reply", "Hi there\n", "reply for requests")
	flag.Parse()
	serve(*port)
}
