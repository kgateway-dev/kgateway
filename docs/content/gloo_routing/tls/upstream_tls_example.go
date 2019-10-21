package main

import (
	"io"
	"log"
	"net/http"
)

func helloHandler(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "Hello, world!\n")
}

func main() {
	// Set up a /hello resource handler
	http.HandleFunc("/hello", helloHandler)

	log.Println("started")
	// Listen to port 8080 and wait
	log.Fatal(http.ListenAndServeTLS(":8080", "/cert.pem", "/key.pem", nil))
}
