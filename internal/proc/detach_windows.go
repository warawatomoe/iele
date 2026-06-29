//go:build windows

package proc

import (
	"os/exec"
	"syscall"

	e "iele/internal/err"
)

func Detach(path string, args []string, env []string) (int, error) {
	cmd := exec.Command(path, args...)
	if env != nil {
		cmd.Env = env
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | syscall.DETACHED_PROCESS,
	}

	if err := cmd.Start(); err != nil {
		return 0, e.Wrap("", e.Trans, "proc:detach", err)
	}

	pid := cmd.Process.Pid
	go cmd.Wait()
	return pid, nil
}
