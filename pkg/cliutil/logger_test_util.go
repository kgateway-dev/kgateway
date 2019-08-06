package cliutil

import (
	"fmt"
	"github.com/solo-io/solo-kit/test/helpers"
	"io/ioutil"
	"os"
)

func RegisterGlooDebugLogPrintHandlerAndClearLogs() {
	_ = os.Remove(GetLogsPath())
	RegisterGlooDebugLogPrintHandler()
}

func RegisterGlooDebugLogPrintHandler() {
	helpers.RegisterPreFailHandler(printGlooDebugLogs)
}

func printGlooDebugLogs() {
	logs, _ := ioutil.ReadFile(GetLogsPath())
	fmt.Println("*** Gloo debug logs ***")
	fmt.Println(string(logs))
	fmt.Println("*** End Gloo debug logs ***")
}
