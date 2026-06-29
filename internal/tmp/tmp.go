package tmp

import (
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	e "iele/internal/err"
)

var (
	watching atomic.Bool
	stopped  atomic.Bool
	quit     chan os.Signal
)

func Make(dir, pattern string) (*os.File, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return nil, e.Wrap("", e.Trans, "tmp:make", err)
	}
	return f, nil
}

func Dir(base, pattern string) (string, error) {
	path, err := os.MkdirTemp(base, pattern)
	if err != nil {
		return "", e.Wrap("", e.Trans, "tmp:dir", err)
	}
	return path, nil
}

func Remove(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return e.Wrap("", e.Trans, "tmp:remove", err)
	}
	return nil
}

func RemoveAll(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return e.Wrap("", e.Trans, "tmp:remove_all", err)
	}
	return nil
}

// Watch latches SIGINT/SIGTERM for cooperative shutdown.
// Safe to call multiple times.
func Watch() {
	if !watching.CompareAndSwap(false, true) {
		return
	}
	quit = make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		stopped.Store(true)
	}()
}

func Stop() {
	if quit != nil {
		signal.Stop(quit)
	}
	stopped.Store(false)
	watching.Store(false)
}

// Useful for cooperative cancellation in long loops.
func Stopped() bool {
	return stopped.Load()
}
