package cfg

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	e "iele/internal/err"
)

const (
	MaxIn  = 16 * 1024 * 1024
	MaxLn  = 65536
	MaxSec = 128
	MaxKey = 128
	MaxVal = 4096

	Req uint = 1
)

type Typ int

const (
	TStr Typ = iota
	TBool
	TInt
	TUint
	TNum
	TEnum
)

type Opt struct {
	MaxIn  int
	MaxLn  int
	MaxSec int
	MaxKey int
	MaxVal int
}

type Entry struct {
	Section string
	Key     string
	Val     string
}

// Sections optional. Comments: # or ;.
// Empty lines skipped. Keys without = rejected.
func Parse(r io.Reader, fn func(Entry) error) error {
	return ParseWith(r, nil, fn)
}

func ParseWith(r io.Reader, opt *Opt, fn func(Entry) error) error {
	if r == nil || fn == nil {
		return e.New("", e.Call, "cfg:parse", "bad_arg")
	}

	o := normOpt(opt)
	data, err := io.ReadAll(io.LimitReader(r, int64(o.MaxIn)+1))
	if err != nil {
		return e.Wrap("", e.Trans, "cfg:read", err)
	}
	if len(data) > o.MaxIn {
		return e.New("", e.Cap, "cfg:parse:1", "max_in")
	}

	sec := ""
	row := 1
	pos := 0

	for pos < len(data) {
		beg := pos
		for pos < len(data) && data[pos] != '\n' {
			pos++
		}

		line := string(data[beg:pos])
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if len(line) > o.MaxLn {
			return e.New("", e.Cap, at(row), "max_ln")
		}
		if pos < len(data) && data[pos] == '\n' {
			pos++
		}

		trimmed := strings.TrimSpace(line)

		if trimmed == "" || trimmed[0] == '#' || trimmed[0] == ';' {
			row++
			continue
		}

		if trimmed[0] == '[' {
			end := strings.IndexByte(trimmed, ']')
			if end < 0 {
				return e.New("", e.Prov, at(row), "sec_end")
			}
			trail := strings.TrimSpace(trimmed[end+1:])
			if trail != "" && trail[0] != '#' && trail[0] != ';' {
				return e.New("", e.Prov, at(row), "sec_trail")
			}
			sec = strings.TrimSpace(trimmed[1:end])
			if sec == "" {
				return e.New("", e.Prov, at(row), "sec_empty")
			}
			if len(sec) > o.MaxSec {
				return e.New("", e.Cap, at(row), "max_sec")
			}
			row++
			continue
		}

		eq := strings.IndexByte(trimmed, '=')
		if eq < 0 {
			return e.New("", e.Prov, at(row), "ent_eq")
		}
		key := strings.TrimSpace(trimmed[:eq])
		val := strings.TrimSpace(trimmed[eq+1:])
		if key == "" {
			return e.New("", e.Prov, at(row), "key_empty")
		}
		if len(key) > o.MaxKey {
			return e.New("", e.Cap, at(row), "max_key")
		}
		if len(val) > o.MaxVal {
			return e.New("", e.Cap, at(row), "max_val")
		}

		if err := fn(Entry{Section: sec, Key: key, Val: val}); err != nil {
			return err
		}
		row++
	}

	return nil
}

// Returns nil if file does not exist.
func Load(path string, fn func(Entry) error) error {
	return LoadWith(path, nil, fn)
}

func LoadWith(path string, opt *Opt, fn func(Entry) error) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return e.Wrap("", e.Trans, "cfg:open", err)
	}
	defer f.Close()
	return ParseWith(f, opt, fn)
}

type Enum struct {
	Text string
	Val  int
}

type Bind struct {
	Section string
	Key     string
	Typ     Typ
	Dst     any
	Enum    []Enum
	Flg     uint
}

func Apply(path string, binds []Bind) error {
	return ApplyWith(path, nil, binds)
}

func ApplyWith(path string, opt *Opt, binds []Bind) error {
	hit := make([]bool, len(binds))

	err := LoadWith(path, opt, func(ent Entry) error {
		for i := range binds {
			if binds[i].Section == ent.Section && binds[i].Key == ent.Key {
				hit[i] = true
				return set(&binds[i], ent)
			}
		}
		return e.New("", e.Prov, atEntry(ent), "unk_key")
	})
	if err != nil {
		return err
	}

	for i := range binds {
		if binds[i].Flg&Req != 0 && !hit[i] {
			return e.New("", e.Prov, "cfg:bind", "req_key")
		}
	}
	return nil
}

// Creates dir if needed. Mode 0700.
func Dir(tool string) (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", e.Wrap("", e.Trans, "cfg:dir", err)
	}
	dir := filepath.Join(base, tool)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", e.Wrap("", e.Trans, "cfg:dir", err)
	}
	return dir, nil
}

func Path(tool string) (string, error) {
	dir, err := Dir(tool)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.ini"), nil
}

func normOpt(opt *Opt) Opt {
	if opt == nil {
		return Opt{
			MaxIn:  MaxIn,
			MaxLn:  MaxLn,
			MaxSec: MaxSec,
			MaxKey: MaxKey,
			MaxVal: MaxVal,
		}
	}
	o := *opt
	if o.MaxIn == 0 {
		o.MaxIn = MaxIn
	}
	if o.MaxLn == 0 {
		o.MaxLn = MaxLn
	}
	if o.MaxSec == 0 {
		o.MaxSec = MaxSec
	}
	if o.MaxKey == 0 {
		o.MaxKey = MaxKey
	}
	if o.MaxVal == 0 {
		o.MaxVal = MaxVal
	}
	return o
}

func set(b *Bind, ent Entry) error {
	if b.Dst == nil {
		return e.New("", e.Call, atEntry(ent), "nil_dst")
	}

	switch b.Typ {
	case TStr:
		dst, ok := b.Dst.(*string)
		if !ok {
			return e.New("", e.Call, atEntry(ent), "dst_str")
		}
		*dst = ent.Val
		return nil
	case TBool:
		dst, ok := b.Dst.(*bool)
		if !ok {
			return e.New("", e.Call, atEntry(ent), "dst_bool")
		}
		val, ok := parseBool(ent.Val)
		if !ok {
			return e.New("", e.Prov, atEntry(ent), "type_bool")
		}
		*dst = val
		return nil
	case TInt:
		dst, ok := b.Dst.(*int64)
		if !ok {
			return e.New("", e.Call, atEntry(ent), "dst_int")
		}
		val, err := strconv.ParseInt(ent.Val, 10, 64)
		if err != nil {
			return e.New("", e.Prov, atEntry(ent), "type_int")
		}
		*dst = val
		return nil
	case TUint:
		dst, ok := b.Dst.(*uint64)
		if !ok {
			return e.New("", e.Call, atEntry(ent), "dst_uint")
		}
		val, err := strconv.ParseUint(ent.Val, 10, 64)
		if err != nil {
			return e.New("", e.Prov, atEntry(ent), "type_uint")
		}
		*dst = val
		return nil
	case TNum:
		dst, ok := b.Dst.(*float64)
		if !ok {
			return e.New("", e.Call, atEntry(ent), "dst_num")
		}
		val, err := strconv.ParseFloat(ent.Val, 64)
		if err != nil {
			return e.New("", e.Prov, atEntry(ent), "type_num")
		}
		*dst = val
		return nil
	case TEnum:
		dst, ok := b.Dst.(*int)
		if !ok {
			return e.New("", e.Call, atEntry(ent), "dst_enum")
		}
		for _, m := range b.Enum {
			if ent.Val == m.Text {
				*dst = m.Val
				return nil
			}
		}
		return e.New("", e.Prov, atEntry(ent), "type_enum")
	default:
		return e.New("", e.Call, atEntry(ent), "type_slot")
	}
}

func parseBool(s string) (bool, bool) {
	switch s {
	case "1", "true", "on", "yes":
		return true, true
	case "0", "false", "off", "no":
		return false, true
	default:
		return false, false
	}
}

func at(row int) string {
	return fmt.Sprintf("cfg:parse:%d", row)
}

func atEntry(ent Entry) string {
	return "cfg:bind:" + ent.Section + "." + ent.Key
}
