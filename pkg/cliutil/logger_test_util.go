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
	logsFile := GetLogsPath()
	logs, _ := ioutil.ReadFile(logsFile)
	fmt.Println("*** Gloo debug logs ***")
	fmt.Println(string(logs))
	fmt.Println("*** End Gloo debug logs ***")
}
