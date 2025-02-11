//go:build ignore

package helpers

import (
	"fmt"
	"os"

	"github.com/solo-io/go-utils/testutils"

	"github.com/kgateway-dev/kgateway/v2/pkg/cliutil"
)

func RegisterGlooDebugLogPrintHandlerAndClearLogs() {
	_ = os.Remove(cliutil.GetLogsPath())
	RegisterGlooDebugLogPrintHandler()
}

func RegisterGlooDebugLogPrintHandler() {
	testutils.RegisterPreFailHandler(PrintGlooDebugLogs)
}

func PrintGlooDebugLogs() {
	logs, _ := os.ReadFile(cliutil.GetLogsPath())
	fmt.Println("*** Gloo debug logs ***")
	fmt.Println(string(logs))
	fmt.Println("*** End Gloo debug logs ***")
}
