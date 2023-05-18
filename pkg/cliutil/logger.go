package cliutil

import (
	"io"
	"os"
	"path/filepath"
	"sync"

	"k8s.io/client-go/util/homedir"
)

const (
	glooDir  = ".gloo"
	glooLogs = "debug.log"
)

var (
	glooPath     string
	glooLogsPath string
	logger       io.Writer
	mutex        sync.Once
)

func init() {
	home := homedir.HomeDir()
	glooPath = filepath.Join(home, glooDir)
	glooLogsPath = filepath.Join(glooPath, glooLogs)
}

func GetLogsPath() string {
	return glooLogsPath
}

func GetLogger() io.Writer {
	Initialize()
	return logger
}

func Initialize() error {
	var initError error
	mutex.Do(func() {
		if _, err := os.ReadDir(glooPath); err != nil {
			if !os.IsNotExist(err) {
				initError = err
				return
			}
			err = os.Mkdir(glooPath, 0755)
			if err != nil {
				initError = err
				return
			}
		}
		file, err := os.OpenFile(glooLogsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			initError = err
			return
		}
		logger = file
	})
	if initError != nil {
		logger = os.Stdout
	}
	return initError
}
