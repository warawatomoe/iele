package arg

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	e "iele/internal/err"
)

type Typ int

const (
	TFlg Typ = iota + 1
	TInt
	TNum
	TStr
)

type Opt struct {
	Key byte
	Typ Typ
	Dst any
	Met string
	Doc string
	Flg uint
}

const (
	Req uint = 1
)

type Pos struct {
	Name string
	Flg  uint
}

type Res struct {
	Pos []string
}

func Parse(name, usage string, opts []Opt) Res {
	var res Res
	stop := false
	args := os.Args[1:]

	for i := 0; i < len(args); i++ {
		cur := args[i]
		if stop || cur == "" || cur[0] != '-' || cur == "-" {
			res.Pos = append(res.Pos, cur)
			continue
		}
		if cur == "--" {
			stop = true
			continue
		}
		if strings.HasPrefix(cur, "--") {
			Help(os.Stderr, name, usage, opts)
			e.Die(e.New(name, e.Call, "arg:parse", "long_flag"))
		}

		for j := 1; j < len(cur); j++ {
			opt := find(opts, cur[j])
			if opt == nil {
				Help(os.Stderr, name, usage, opts)
				e.Die(e.New(name, e.Prov, "arg:parse", "unknown_"+string(cur[j])))
			}
			if opt.Typ == TFlg {
				set(name, opt, "")
				continue
			}

			val := ""
			if j+1 < len(cur) {
				val = cur[j+1:]
			} else {
				i++
				if i >= len(args) {
					Help(os.Stderr, name, usage, opts)
					e.Die(e.New(name, e.Call, "arg:parse", "missing_"+string(opt.Key)))
				}
				val = args[i]
			}
			set(name, opt, val)
			break
		}
	}

	return res
}

func Help(out *os.File, name, usage string, opts []Opt) {
	fmt.Fprintf(out, "usage: %s %s\n", name, usage)
	if len(opts) == 0 {
		return
	}
	fmt.Fprintln(out, "\noptions:")
	for _, opt := range opts {
		if opt.Doc == "" {
			continue
		}
		if opt.Typ == TFlg {
			fmt.Fprintf(out, "  -%c        %s\n", opt.Key, opt.Doc)
			continue
		}
		met := opt.Met
		if met == "" {
			met = "val"
		}
		fmt.Fprintf(out, "  -%c %-8s %s\n", opt.Key, met, opt.Doc)
	}
}

func Usage(opts []Opt, pos []Pos) string {
	var parts []string
	i := 0
	for i < len(opts) {
		opt := opts[i]
		if opt.Typ == TFlg && opt.Flg&Req == 0 {
			j := i
			var keys []byte
			for j < len(opts) && opts[j].Typ == TFlg && opts[j].Flg&Req == 0 {
				keys = append(keys, opts[j].Key)
				j++
			}
			parts = append(parts, "[-"+string(keys)+"]")
			i = j
			continue
		}

		item := "-" + string(opt.Key)
		if opt.Typ != TFlg {
			met := opt.Met
			if met == "" {
				met = "val"
			}
			item += " " + met
		}
		if opt.Flg&Req == 0 {
			item = "[" + item + "]"
		}
		parts = append(parts, item)
		i++
	}

	for _, p := range pos {
		if p.Name == "" {
			continue
		}
		if p.Flg&Req != 0 {
			parts = append(parts, "<"+p.Name+">")
		} else {
			parts = append(parts, "["+p.Name+"]")
		}
	}
	return strings.Join(parts, " ")
}

func find(opts []Opt, key byte) *Opt {
	for i := range opts {
		if opts[i].Key == key {
			return &opts[i]
		}
	}
	return nil
}

func set(name string, opt *Opt, val string) {
	if opt.Dst == nil {
		e.Die(e.New(name, e.Call, "arg:set", "nil_dst"))
	}

	switch opt.Typ {
	case TFlg:
		switch dst := opt.Dst.(type) {
		case *bool:
			*dst = true
		case *int:
			*dst++
		default:
			e.Die(e.New(name, e.Bug, "arg:set", "bad_flg_dst"))
		}
	case TInt:
		n, err := strconv.Atoi(val)
		if err != nil {
			e.Die(e.New(name, e.Prov, "arg:set", "bad_int"))
		}
		switch dst := opt.Dst.(type) {
		case *int:
			*dst = n
		case *int64:
			*dst = int64(n)
		default:
			e.Die(e.New(name, e.Bug, "arg:set", "bad_int_dst"))
		}
	case TNum:
		dst, ok := opt.Dst.(*float64)
		if !ok {
			e.Die(e.New(name, e.Bug, "arg:set", "bad_num_dst"))
		}
		n, err := strconv.ParseFloat(val, 64)
		if err != nil {
			e.Die(e.New(name, e.Prov, "arg:set", "bad_num"))
		}
		*dst = n
	case TStr:
		dst, ok := opt.Dst.(*string)
		if !ok {
			e.Die(e.New(name, e.Bug, "arg:set", "bad_str_dst"))
		}
		*dst = val
	default:
		e.Die(e.New(name, e.Bug, "arg:set", "bad_type"))
	}
}
