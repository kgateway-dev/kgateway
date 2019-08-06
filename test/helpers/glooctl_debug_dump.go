package helpers

import (
	"fmt"
	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/go-utils/testutils"
	"io/ioutil"
	"os"
)

func RegisterGlooDebugLogPrintHandlerAndClearLogs() {
	_ = os.Remove(cliutil.GetLogsPath())
	RegisterGlooDebugLogPrintHandler()
}

func RegisterGlooDebugLogPrintHandler() {
	testutils.RegisterPreFailHandler(printGlooDebugLogs)
}

func printGlooDebugLogs() {
	logs, _ := ioutil.ReadFile(cliutil.GetLogsPath())
	fmt.Println("*** Gloo debug logs ***")
	fmt.Println(string(logs))
	fmt.Println("*** End Gloo debug logs ***")
}
