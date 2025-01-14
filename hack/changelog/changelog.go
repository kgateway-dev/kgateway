package changelog

import (
	"fmt"
	"os"

	"github.com/go-git/go-git/v5"
)

func main() {
	if err := generate(); err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func generate() error {
	r, err := git.PlainOpen(".")
	if err != nil {
		return err
	}
}
