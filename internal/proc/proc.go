package proc

import (
	"bytes"
	"errors"
	"io"
	"os/exec"

	e "iele/internal/err"
)

const MaxCap = 4 * 1024 * 1024

type Res struct {
	Exit int
	Out  []byte
	Err  []byte
}

// Non-zero exit is not an error; stderr is captured in Res.Err.
func Run(path string, args []string, env []string) (Res, error) {
	return RunWith(path, args, env, MaxCap)
}

func RunWith(path string, args []string, env []string, cap int) (Res, error) {
	if path == "" || cap < 0 {
		return Res{}, e.New("", e.Call, "proc:run", "bad_arg")
	}

	cmd := exec.Command(path, args...)
	if env != nil {
		cmd.Env = env
	}

	var out, er bytes.Buffer
	cmd.Stdout = &limitWriter{w: &out, cap: cap}
	cmd.Stderr = &limitWriter{w: &er, cap: cap}

	err := cmd.Run()
	res := Res{
		Exit: -1,
		Out:  out.Bytes(),
		Err:  er.Bytes(),
	}
	if cmd.ProcessState != nil {
		res.Exit = cmd.ProcessState.ExitCode()
	}
	if err != nil {
		if errors.Is(err, errCap) {
			return res, e.New("", e.Cap, "proc:run", "output_cap")
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return res, nil
		}
		return res, e.Wrap("", e.Trans, "proc:run", err)
	}
	return res, nil
}

func RunPath(name string, args []string, env []string) (Res, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return Res{}, e.Wrap("", e.Trans, "proc:path", err)
	}
	return Run(path, args, env)
}

func RunPathWith(name string, args []string, env []string, cap int) (Res, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return Res{}, e.Wrap("", e.Trans, "proc:path", err)
	}
	return RunWith(path, args, env, cap)
}

var errCap = errors.New("output cap")

type limitWriter struct {
	w   io.Writer
	cap int
	n   int
}

func (w *limitWriter) Write(p []byte) (int, error) {
	if w.cap == 0 && len(p) > 0 {
		return 0, errCap
	}
	if w.n+len(p) > w.cap {
		rem := w.cap - w.n
		if rem > 0 {
			_, _ = w.w.Write(p[:rem])
		}
		return rem, errCap
	}
	n, err := w.w.Write(p)
	w.n += n
	return n, err
}
