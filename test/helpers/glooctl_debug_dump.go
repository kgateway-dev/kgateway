package helpers

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/go-utils/testutils"
)

func RegisterGlooDebugLogPrintHandlerAndClearLogs() {
	_ = os.Remove(cliutil.GetLogsPath())
	RegisterGlooDebugLogPrintHandler()
}

func RegisterGlooDebugLogPrintHandler() {
	testutils.RegisterPreFailHandler(PrintGlooDebugLogs)
}

func PrintGlooDebugLogs() {
	logs, _ := ioutil.ReadFile(cliutil.GetLogsPath())
	fmt.Println("*** Gloo debug logs ***")
	fmt.Println(string(logs))
	fmt.Println("*** End Gloo debug logs ***")
}
