package v1

import (
	"fmt"
	"log"
	"net/http"
)

type HttpPassthroughService struct{}

const ServerPort = 9001

func (h *HttpPassthroughService) StartServer() {
	fmt.Printf("Listening on port %d for auth requests\n", ServerPort)
	handler := func(rw http.ResponseWriter, r *http.Request) {
		fmt.Printf("received request with url: %s, with headers %+v\n", r.URL.String(), r.Header)
		switch r.URL.Path {
		case "/auth":
			if r.Header.Get("authorization") == "authorize me" {
				rw.WriteHeader(200)
			} else {
				rw.WriteHeader(403)
				rw.Write([]byte("Please also set the `authorization: authorize me` header."))
			}
		default:
			fmt.Println("Did not reach the right path, use /auth to authenticate requests! Make sure to include the header 'authorization: authorize me' too.")
			rw.WriteHeader(403)
		}
	}
	address := fmt.Sprintf(":%d", ServerPort)
	err := http.ListenAndServe(address, http.HandlerFunc(handler))
	if err != nil {
		log.Fatal(err.Error())
	}
}
