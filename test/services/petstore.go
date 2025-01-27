package services

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-swagger/go-swagger/examples/2.0/petstore/server/api"
)

func RunPetstore(ctx context.Context, port int) error {
	router, err := api.NewPetstore()
	if err != nil {
		return err
	}
	s := &http.Server{Addr: fmt.Sprintf(":%v", port), Handler: router}
	go func() {
		defer s.Close()
		<-ctx.Done()
	}()
	return s.ListenAndServe()
}
