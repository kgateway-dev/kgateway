package cliutil

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"k8s.io/client-go/util/homedir"
)

const (
	glooDir  = ".gloo"
	glooLogs = "debug.log"
)

var (
	glooPath     string
	glooLogsPath string
	Logger       io.Writer
)

func init() {
	home := homedir.HomeDir()
	glooPath = filepath.Join(home, glooDir)
	glooLogsPath = filepath.Join(glooPath, glooLogs)
}

func GetLogsPath() string {
	return glooLogsPath
}

func Initialize() error {
	if Logger == nil {
		if _, err := ioutil.ReadDir(glooPath); err != nil {
			if !os.IsNotExist(err) {
				return err
			}
			err := os.Mkdir(glooPath, 0755)
			if err != nil {
				return err
			}
		}
		file, err := os.OpenFile(glooLogsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		Logger = file
	}
	return nil
}
