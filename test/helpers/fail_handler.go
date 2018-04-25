package helpers

import (
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func waitOnFail() {

	if os.Getenv("WAIT_ON_FAIL") == "1" {
		// wait for sig usr1
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGUSR1)
		defer signal.Reset(syscall.SIGUSR1)
		fmt.Println("We are here:")
		debug.PrintStack()
		fmt.Printf("Waiting for human intervention. to continue, run 'kill -SIGUSR1 %d'\n", os.Getpid())
		<-c
	}
}

var preFails []func()

func RegisterPreFailHandler(prefail func()) {
	preFails = append(preFails, prefail)
}

func RegisterCommonFailHandlers() {
	RegisterPreFailHandler(waitOnFail)
	RegisterFailHandler(failHandler)
}

func failHandler(message string, callerSkip ...int) {
	fmt.Println("Fail handler msg", message)

	for _, prefail := range preFails {
		prefail()
	}
	Fail(message, callerSkip...)

}
