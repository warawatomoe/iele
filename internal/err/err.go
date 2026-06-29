package err

import (
	"fmt"
	"os"
)

type Cat int

const (
	OK    Cat = iota // 0 - no error
	Call             // 1 - caller misuse
	Trans            // 2 - transport/io
	Prov             // 3 - provider/input
	Cap              // 4 - capacity exceeded
	Bug              // 5 - internal invariant breach
)

func Exit(c Cat) int {
	switch c {
	case OK:
		return 0
	case Call:
		return 2
	case Trans:
		return 3
	case Prov:
		return 4
	case Cap:
		return 5
	case Bug:
		return 6
	default:
		return 6
	}
}

type E struct {
	Lib   string
	Cat   Cat
	At    string
	Cause string
	Err   error
}

func (e *E) Error() string {
	return fmt.Sprintf("ERR lib=%s exit=%d code=%s at=%s cause=%s",
		e.Lib, Exit(e.Cat), code(e.Cat), e.At, e.Cause)
}

func New(lib string, cat Cat, at, cause string) *E {
	return &E{Lib: lib, Cat: cat, At: at, Cause: cause}
}

// Prints to stderr and exits.
func Die(e *E) {
	fmt.Fprintln(os.Stderr, e.Error())
	os.Exit(Exit(e.Cat))
}

func Wrap(lib string, cat Cat, at string, cause error) *E {
	return &E{Lib: lib, Cat: cat, At: at, Cause: cause.Error(), Err: cause}
}

func (e *E) Unwrap() error {
	return e.Err
}

func code(c Cat) string {
	switch c {
	case OK:
		return "ok"
	case Call:
		return "call"
	case Trans:
		return "trans"
	case Prov:
		return "prov"
	case Cap:
		return "cap"
	case Bug:
		return "bug"
	default:
		return "bug"
	}
}
