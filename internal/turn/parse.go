package turn

import (
	"fmt"
	"io"
	"strings"

	e "iele/internal/err"
)

const (
	MaxIn  = 16 * 1024 * 1024
	MaxLn  = 65536
	MaxKey = 128
	MaxVal = 4096
)

type Param struct {
	Key string
	Val string
}

type Payload struct {
	Params []Param
	Msgs   []Msg
}

// Parse reads a payload file in the YAML subset format:
//
//	key: value
//
//	messages:
//	- role: user
//	  content: |-
//	    text here
func Parse(r io.Reader) (Payload, error) {
	return ParseWith(r, nil)
}

type Opt struct {
	MaxIn  int
	MaxLn  int
	MaxKey int
	MaxVal int
}

func ParseWith(r io.Reader, opt *Opt) (Payload, error) {
	var p Payload
	if r == nil {
		return Payload{}, e.New("", e.Call, "turn:parse", "bad_arg")
	}

	o := normOpt(opt)
	data, err := io.ReadAll(io.LimitReader(r, int64(o.MaxIn)+1))
	if err != nil {
		return Payload{}, e.Wrap("", e.Trans, "turn:read", err)
	}
	if len(data) > o.MaxIn {
		return Payload{}, e.New("", e.Cap, at(1), "max_in")
	}

	st := 0
	row := 1
	pos := 0
	var role Role
	var lines []string
	contentIndent := ""
	sawMessages := false

	flush := func() {
		p.Msgs = append(p.Msgs, Msg{Role: role, Text: strings.Join(lines, "\n")})
		role = ""
		lines = nil
		contentIndent = ""
		st = 1
	}

	for pos < len(data) {
		line, next := nextLine(data, pos)
		if len(line) > o.MaxLn {
			return Payload{}, e.New("", e.Cap, at(row), "max_ln")
		}
		pos = next

		if st == 3 {
			if line == "" {
				lines = append(lines, "")
				row++
				continue
			}
			if strings.HasPrefix(line, contentIndent) {
				lines = append(lines, line[len(contentIndent):])
				row++
				continue
			}
			if strings.TrimSpace(line) == "" {
				return Payload{}, e.New("", e.Prov, at(row), "msg_ind")
			}
			flush()
		}

		if hasTabIndent(line) {
			return Payload{}, e.New("", e.Prov, at(row), "tab_indent")
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			row++
			continue
		}

		switch st {
		case 0:
			if trimmed == "messages:" {
				sawMessages = true
				st = 1
				row++
				continue
			}
			key, val, ok := strings.Cut(trimmed, ":")
			if !ok {
				return Payload{}, e.New("", e.Prov, at(row), "param_colon")
			}
			key = strings.TrimSpace(key)
			val = strings.TrimSpace(val)
			if key == "" {
				return Payload{}, e.New("", e.Prov, at(row), "param_key")
			}
			if len(key) > o.MaxKey {
				return Payload{}, e.New("", e.Cap, at(row), "max_key")
			}
			if len(val) > o.MaxVal {
				return Payload{}, e.New("", e.Cap, at(row), "max_val")
			}
			p.Params = append(p.Params, Param{Key: key, Val: val})
		case 1:
			nextRole, ok := parseRoleLine(trimmed)
			if !ok {
				return Payload{}, e.New("", e.Prov, at(row), "msg_role")
			}
			role = nextRole
			st = 2
		case 2:
			if trimmed != "content: |-" {
				return Payload{}, e.New("", e.Prov, at(row), "msg_cont")
			}
			indent := leadingSpaces(line)
			contentIndent = indent + "  "
			lines = nil
			st = 3
		default:
			return Payload{}, e.New("", e.Bug, at(row), "state")
		}
		row++
	}

	if !sawMessages {
		return Payload{}, e.New("", e.Prov, at(row), "msg_head")
	}
	if st == 2 {
		return Payload{}, e.New("", e.Prov, at(row), "msg_eof")
	}
	if st == 3 {
		flush()
	}
	if len(p.Msgs) == 0 {
		return Payload{}, e.New("", e.Prov, at(row), "msg_empty")
	}

	return p, nil
}

func (p *Payload) Param(key string) string {
	for _, kv := range p.Params {
		if kv.Key == key {
			return kv.Val
		}
	}
	return ""
}

func normOpt(opt *Opt) Opt {
	if opt == nil {
		return Opt{
			MaxIn:  MaxIn,
			MaxLn:  MaxLn,
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
	if o.MaxKey == 0 {
		o.MaxKey = MaxKey
	}
	if o.MaxVal == 0 {
		o.MaxVal = MaxVal
	}
	return o
}

func nextLine(data []byte, pos int) (string, int) {
	beg := pos
	for pos < len(data) && data[pos] != '\n' {
		pos++
	}
	line := string(data[beg:pos])
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	if pos < len(data) && data[pos] == '\n' {
		pos++
	}
	return line, pos
}

func parseRoleLine(trimmed string) (Role, bool) {
	raw, ok := strings.CutPrefix(trimmed, "- role:")
	if !ok {
		return "", false
	}
	role := Role(strings.TrimSpace(raw))
	switch role {
	case RoleSys, RoleDev, RoleUser, RoleAsst:
		return role, true
	default:
		return "", false
	}
}

func leadingSpaces(s string) string {
	i := 0
	for i < len(s) && s[i] == ' ' {
		i++
	}
	return s[:i]
}

func hasTabIndent(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ':
			continue
		case '\t':
			return true
		default:
			return false
		}
	}
	return false
}

func at(row int) string {
	return fmt.Sprintf("turn:parse:%d", row)
}
