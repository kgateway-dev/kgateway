package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

type handler struct{}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logRequest(r)
	w.Header().Set("Content-Type", "application/text")
	if _, err := w.Write([]byte("accepted event")); err != nil {
		log.Printf("err: %v", err)
	}
}

func logRequest(r *http.Request) {
	log.Printf("Got request with headers: ")
	for k, v := range r.Header {
		log.Printf("%v: %v", k, v)
	}
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("error reading body: %v", err)
		return
	}
	defer r.Body.Close()
	log.Printf("received event: %s", data)
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
	port := flag.Uint("port", 8080, "listener port")
	flag.Parse()
	serve(*port)
}
