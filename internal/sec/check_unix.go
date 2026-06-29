//go:build !windows

package sec

import (
	"fmt"
	"os"
	"syscall"

	e "iele/internal/err"
)

func check(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return e.Wrap("", e.Trans, "sec:stat", err)
	}
	if !info.Mode().IsRegular() {
		return e.New("", e.Prov, "sec:perm", "not_regular")
	}
	perm := info.Mode().Perm()
	if perm != 0600 && perm != 0400 {
		return e.New("", e.Prov, "sec:perm", fmt.Sprintf("mode_%04o", info.Mode().Perm()))
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return e.New("", e.Trans, "sec:perm", "stat_unavail")
	}
	if st.Uid != uint32(os.Getuid()) {
		return e.New("", e.Prov, "sec:perm", "not_owner")
	}
	return nil
}
