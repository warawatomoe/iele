package jsn

import (
	"fmt"

	e "iele/internal/err"
)

type Cat = e.Cat

const (
	OK    = e.OK
	Call  = e.Call
	Trans = e.Trans
	Prov  = e.Prov
	Cap   = e.Cap
	Bug   = e.Bug
)

type Stage uint8

const (
	StNone Stage = iota
	StTok
	StPar
	StWrt
)

func (s Stage) String() string {
	switch s {
	case StNone:
		return "none"
	case StTok:
		return "tok"
	case StPar:
		return "par"
	case StWrt:
		return "wrt"
	default:
		return "bug"
	}
}

type Loc struct {
	Off uint
	Row uint
	Col uint
}

func fail(cat Cat, stg Stage, loc Loc, cause string, err error) *e.E {
	return &e.E{
		Cat:   cat,
		At:    fmt.Sprintf("jsn:%s:%d:%d:%d", stg, loc.Off, loc.Row, loc.Col),
		Cause: cause,
		Err:   err,
	}
}
