package services

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

type Runner struct {
	Sourcepath    string
	ComponentName string
}

func (r *Runner) waitForExternalProcess() error {
	for {
		procs, err := process.Processes()
		if err != nil {
			panic(err)
		}
		for _, proc := range procs {
			str, err := proc.Exe()
			if err != nil {
				continue
			}
			if strings.Contains(str, r.Sourcepath) {
				return nil
			}

		}
		time.Sleep(time.Second)
	}
}

func (r *Runner) Run(c *exec.Cmd) (*exec.Cmd, error) {
	if os.Getenv("USE_DEBUGGER_"+r.ComponentName) == "1" {
		fmt.Printf("Please Run the following command in your debugger:\n"+
			"%v %v \n"+
			"CWD %v \n"+
			"looking for processes started from %v", c.Path, c.Args, c.Dir, r.Sourcepath)

		return nil, r.waitForExternalProcess()
	}
	return c, c.Start()
}
