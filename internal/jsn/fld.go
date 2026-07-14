package jsn

import (
	"errors"
	"strconv"
	"unicode/utf8"
)

type FTyp int

const (
	FStr FTyp = iota + 1
	FBytes
	FI32
	FU32
	FF64
	FBool
	FRaw
	FOStr
	FOBytes
	FOI32
	FOU32
	FOF64
	FOBool
)

const (
	FReq uint = 1
)

type Fld struct {
	Key string
	Typ FTyp
	Dst any
	Flg uint
}

type OptI32 struct {
	Has bool
	V   int32
}

type OptU32 struct {
	Has bool
	V   uint32
}

type OptF64 struct {
	Has bool
	V   float64
}

type OptBool struct {
	Has bool
	V   bool
}

type OptStr struct {
	Has bool
	V   string
}

type OptBytes struct {
	Has bool
	V   []byte
}

type Arena struct {
	B []byte
}

func (a *Arena) Reset() {
	a.B = a.B[:0]
}

func (a *Arena) put(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	beg := len(a.B)
	a.B = append(a.B, src...)
	return a.B[beg:]
}

func FldObj(def []Fld, b []byte, a *Arena) error {
	if def == nil {
		return fail(Call, StPar, Loc{}, "arg_fld", nil)
	}
	var seen []bool
	if len(def) > 64 {
		seen = make([]bool, len(def))
	}
	st := fldState{def: def, seen: seen, buf: b, arena: a}
	p := bpar{
		fld:    &st,
		maxdep: MaxDep,
		buf:    b,
		row:    1,
		col:    1,
	}
	err := p.run()
	if st.err != nil {
		return st.err
	}
	if err != nil {
		return err
	}
	if !st.root {
		return fail(Prov, StPar, Loc{}, "want_obeg", nil)
	}
	for i := range def {
		if def[i].Flg&FReq != 0 && !st.isSeen(i) {
			return fail(Prov, StPar, Loc{}, "missing_"+def[i].Key, nil)
		}
	}
	return nil
}

func FldArrObj(out [][]byte, b []byte) (int, error) {
	st := fldArrState{buf: b, out: out, obj: true}
	err := ParseBytes(b, st.on, nil)
	if st.err != nil {
		return st.n, st.err
	}
	if err != nil {
		return st.n, err
	}
	if !st.root {
		return st.n, fail(Prov, StPar, Loc{}, "want_abeg", nil)
	}
	return st.n, nil
}

func FldArrScalar(out [][]byte, b []byte, a *Arena) (int, error) {
	st := fldArrState{buf: b, out: out, arena: a}
	err := ParseBytes(b, st.on, nil)
	if st.err != nil {
		return st.n, st.err
	}
	if err != nil {
		return st.n, err
	}
	if !st.root {
		return st.n, fail(Prov, StPar, Loc{}, "want_abeg", nil)
	}
	return st.n, nil
}

var errFldStop = errors.New("fld_stop")

type fldArrState struct {
	buf    []byte
	out    [][]byte
	arena  *Arena
	err    error
	root   bool
	obj    bool
	dep    int
	n      int
	rawCap bool
	rawBeg uint
	rawDep int
	strCap bool
	strRaw []byte
	strTmp []byte
	strLoc Loc
}

func (s *fldArrState) stop(err error) error {
	s.err = err
	return errFldStop
}

func (s *fldArrState) set(cat Cat, loc Loc, cause string) error {
	return s.stop(fail(cat, StPar, loc, cause, nil))
}

func (s *fldArrState) put(v []byte, loc Loc) error {
	if s.n >= len(s.out) {
		return s.set(Cap, loc, "arg_cap")
	}
	s.out[s.n] = v
	s.n++
	return nil
}

func (s *fldArrState) on(ev Event) error {
	if !s.root {
		if ev.Kind != EvABeg {
			return s.set(Prov, ev.Loc, "want_abeg")
		}
		s.root = true
		s.dep = 1
		return nil
	}

	switch ev.Kind {
	case EvOBeg:
		if s.dep == 1 {
			if !s.obj {
				return s.set(Prov, ev.Loc, "want_str")
			}
			s.rawCap = true
			s.rawBeg = ev.Loc.Off
			s.rawDep = 2
		}
		s.dep++
	case EvABeg:
		if s.dep == 1 {
			if s.obj {
				return s.set(Prov, ev.Loc, "want_obj")
			}
			return s.set(Prov, ev.Loc, "want_str")
		}
		s.dep++
	case EvOEnd:
		if s.rawCap && s.dep == s.rawDep {
			if err := s.put(s.buf[s.rawBeg:ev.Loc.Off+1], ev.Loc); err != nil {
				return err
			}
			s.rawCap = false
		}
		s.dep--
	case EvAEnd:
		s.dep--
	case EvSBeg:
		if s.dep == 1 {
			if s.obj {
				return s.set(Prov, ev.Loc, "want_obj")
			}
			s.strCap = true
			s.strRaw = nil
			s.strTmp = s.strTmp[:0]
			s.strLoc = ev.Loc
		}
	case EvSPar:
		if s.strCap {
			s.strRaw = s.addSeg(s.strRaw, ev.Data)
		}
	case EvSEnd:
		if s.strCap {
			dec, err := fldStrBytes(s.strRaw, s.arena)
			if err != nil {
				return s.stop(err)
			}
			if err := s.put(dec, s.strLoc); err != nil {
				return err
			}
			s.strCap = false
		}
	case EvNBeg:
		if s.dep == 1 {
			if s.obj {
				return s.set(Prov, ev.Loc, "want_obj")
			}
			s.rawCap = true
			s.rawBeg = ev.Loc.Off
			s.rawDep = 1
		}
	case EvNEnd:
		if s.rawCap && s.rawDep == 1 {
			if err := s.put(s.buf[s.rawBeg:ev.Loc.Off+1], ev.Loc); err != nil {
				return err
			}
			s.rawCap = false
		}
	case EvTrue:
		if s.dep == 1 {
			if s.obj {
				return s.set(Prov, ev.Loc, "want_obj")
			}
			return s.put(s.buf[ev.Loc.Off:ev.Loc.Off+4], ev.Loc)
		}
	case EvFalse:
		if s.dep == 1 {
			if s.obj {
				return s.set(Prov, ev.Loc, "want_obj")
			}
			return s.put(s.buf[ev.Loc.Off:ev.Loc.Off+5], ev.Loc)
		}
	case EvNull:
		if s.dep == 1 {
			if s.obj {
				return s.set(Prov, ev.Loc, "want_obj")
			}
			return s.put(s.buf[ev.Loc.Off:ev.Loc.Off+4], ev.Loc)
		}
	}
	return nil
}

func (s *fldArrState) addSeg(cur []byte, seg []byte) []byte {
	if len(cur) == 0 && len(s.strTmp) == 0 {
		return seg
	}
	if len(s.strTmp) == 0 {
		s.strTmp = append(s.strTmp, cur...)
	}
	s.strTmp = append(s.strTmp, seg...)
	return s.strTmp
}

type fldState struct {
	def       []Fld
	seen      []bool
	seenBits  uint64
	buf       []byte
	arena     *Arena
	err       error
	root      bool
	dep       int
	keyCap    bool
	keyRaw    []byte
	keyTmp    []byte
	expectVal bool
	cur       int
	strCap    bool
	strBeg    uint
	strRaw    []byte
	strTmp    []byte
	strLoc    Loc
	numCap    bool
	numRaw    []byte
	numLoc    Loc
	rawCap    bool
	rawBeg    uint
	rawDep    int
}

func (s *fldState) markSeen(i int) {
	if len(s.seen) != 0 {
		s.seen[i] = true
		return
	}
	s.seenBits |= 1 << uint(i)
}

func (s *fldState) isSeen(i int) bool {
	if len(s.seen) != 0 {
		return s.seen[i]
	}
	return s.seenBits&(1<<uint(i)) != 0
}

func (s *fldState) stop(err error) error {
	s.err = err
	return errFldStop
}

func (s *fldState) set(cat Cat, loc Loc, cause string) error {
	return s.stop(fail(cat, StPar, loc, cause, nil))
}

func (s *fldState) on(ev Event) error {
	if !s.root {
		if ev.Kind != EvOBeg {
			return s.set(Prov, ev.Loc, "want_obeg")
		}
		s.root = true
		s.dep = 1
		return nil
	}

	switch ev.Kind {
	case EvOBeg:
		if s.expectVal && s.dep == 1 {
			return s.valComposite(ev, '{')
		}
		s.dep++
	case EvABeg:
		if s.expectVal && s.dep == 1 {
			return s.valComposite(ev, '[')
		}
		s.dep++
	case EvOEnd:
		if s.rawCap && s.dep == s.rawDep {
			if err := s.storeRaw(ev.Loc.Off + 1); err != nil {
				return err
			}
		}
		s.dep--
	case EvAEnd:
		if s.rawCap && s.dep == s.rawDep {
			if err := s.storeRaw(ev.Loc.Off + 1); err != nil {
				return err
			}
		}
		s.dep--
	case EvKBeg:
		if s.dep == 1 {
			s.keyCap = true
			s.keyRaw = nil
			s.keyTmp = s.keyTmp[:0]
		}
	case EvKPar:
		if s.keyCap {
			s.keyRaw = s.addSeg(s.keyRaw, &s.keyTmp, ev.Data)
		}
	case EvKEnd:
		if s.keyCap {
			s.cur = s.find(s.keyRaw)
			s.expectVal = true
			s.keyCap = false
		}
	case EvSBeg:
		if s.expectVal && s.dep == 1 {
			return s.valStringBeg(ev)
		}
	case EvSPar:
		if s.strCap {
			s.strRaw = s.addSeg(s.strRaw, &s.strTmp, ev.Data)
		}
	case EvSEnd:
		if s.strCap {
			return s.valStringEnd(ev)
		}
	case EvNBeg:
		if s.expectVal && s.dep == 1 {
			return s.valNumBeg(ev)
		}
	case EvNPar:
		if s.numCap {
			s.numRaw = ev.Data
		}
	case EvNEnd:
		if s.numCap {
			return s.valNumEnd(ev)
		}
	case EvTrue:
		if s.expectVal && s.dep == 1 {
			return s.valBool(ev, true)
		}
	case EvFalse:
		if s.expectVal && s.dep == 1 {
			return s.valBool(ev, false)
		}
	case EvNull:
		if s.expectVal && s.dep == 1 {
			return s.valNull(ev)
		}
	}
	return nil
}

func (s *fldState) addSeg(cur []byte, tmp *[]byte, seg []byte) []byte {
	if len(cur) == 0 && len(*tmp) == 0 {
		return seg
	}
	if len(*tmp) == 0 {
		*tmp = append(*tmp, cur...)
	}
	*tmp = append(*tmp, seg...)
	return *tmp
}

func (s *fldState) find(key []byte) int {
	for i := range s.def {
		if fldKeyEq(key, s.def[i].Key, &s.keyTmp) {
			return i
		}
	}
	return -1
}

func (s *fldState) valComposite(ev Event, open byte) error {
	if s.cur < 0 {
		s.expectVal = false
		s.dep++
		return nil
	}
	f := &s.def[s.cur]
	if f.Typ != FRaw {
		return s.set(Prov, ev.Loc, fldWant(f.Typ))
	}
	s.rawCap = true
	s.rawBeg = ev.Loc.Off
	s.rawDep = s.dep + 1
	s.expectVal = false
	s.dep++
	_ = open
	return nil
}

func (s *fldState) storeRaw(end uint) error {
	f := &s.def[s.cur]
	dst, ok := f.Dst.(*[]byte)
	if !ok || dst == nil {
		return s.set(Bug, Loc{}, "bad_raw_dst")
	}
	*dst = s.buf[s.rawBeg:end]
	s.markSeen(s.cur)
	s.cur = -1
	s.rawCap = false
	return nil
}

func (s *fldState) valStringBeg(ev Event) error {
	if s.cur < 0 {
		s.expectVal = false
		return nil
	}
	f := &s.def[s.cur]
	if f.Typ != FStr && f.Typ != FBytes && f.Typ != FOStr && f.Typ != FOBytes && f.Typ != FRaw {
		return s.set(Prov, ev.Loc, fldWant(f.Typ))
	}
	s.strCap = true
	s.strBeg = ev.Loc.Off
	s.strRaw = nil
	s.strTmp = s.strTmp[:0]
	s.strLoc = ev.Loc
	s.expectVal = false
	return nil
}

func (s *fldState) valStringEnd(ev Event) error {
	f := &s.def[s.cur]
	if f.Typ == FRaw {
		dst, ok := f.Dst.(*[]byte)
		if !ok || dst == nil {
			return s.set(Bug, Loc{}, "bad_raw_dst")
		}
		*dst = s.buf[s.strBeg : ev.Loc.Off+1]
	} else {
		dec, err := fldStrBytes(s.strRaw, s.arena)
		if err != nil {
			return s.stop(err)
		}
		if err := s.storeStr(f, dec, s.strLoc); err != nil {
			return err
		}
	}
	s.markSeen(s.cur)
	s.cur = -1
	s.strCap = false
	return nil
}

func (s *fldState) valNumBeg(ev Event) error {
	if s.cur < 0 {
		s.expectVal = false
		return nil
	}
	f := &s.def[s.cur]
	if f.Typ != FI32 && f.Typ != FU32 && f.Typ != FF64 &&
		f.Typ != FOI32 && f.Typ != FOU32 && f.Typ != FOF64 && f.Typ != FRaw {
		return s.set(Prov, ev.Loc, fldWant(f.Typ))
	}
	s.numCap = true
	s.numRaw = nil
	s.numLoc = ev.Loc
	s.expectVal = false
	return nil
}

func (s *fldState) valNumEnd(ev Event) error {
	f := &s.def[s.cur]
	if f.Typ == FRaw {
		dst, ok := f.Dst.(*[]byte)
		if !ok || dst == nil {
			return s.set(Bug, Loc{}, "bad_raw_dst")
		}
		*dst = s.numRaw
	} else if err := s.storeNum(f, s.numRaw, s.numLoc); err != nil {
		return err
	}
	s.markSeen(s.cur)
	s.cur = -1
	s.numCap = false
	return nil
}

func (s *fldState) valBool(ev Event, v bool) error {
	if s.cur < 0 {
		s.expectVal = false
		return nil
	}
	f := &s.def[s.cur]
	if f.Typ == FRaw {
		dst, ok := f.Dst.(*[]byte)
		if !ok || dst == nil {
			return s.set(Bug, Loc{}, "bad_raw_dst")
		}
		if v {
			*dst = s.buf[ev.Loc.Off : ev.Loc.Off+4]
		} else {
			*dst = s.buf[ev.Loc.Off : ev.Loc.Off+5]
		}
	} else {
		if f.Typ != FBool && f.Typ != FOBool {
			return s.set(Prov, ev.Loc, fldWant(f.Typ))
		}
		if err := s.storeBool(f, v, ev.Loc); err != nil {
			return err
		}
	}
	s.markSeen(s.cur)
	s.cur = -1
	s.expectVal = false
	return nil
}

func (s *fldState) valNull(ev Event) error {
	if s.cur < 0 {
		s.expectVal = false
		return nil
	}
	f := &s.def[s.cur]
	switch f.Typ {
	case FOStr:
		dst, ok := f.Dst.(*OptStr)
		if !ok || dst == nil {
			return s.set(Bug, Loc{}, "bad_ostr_dst")
		}
		dst.Has = true
		dst.V = ""
	case FOBytes:
		dst, ok := f.Dst.(*OptBytes)
		if !ok || dst == nil {
			return s.set(Bug, Loc{}, "bad_obytes_dst")
		}
		dst.Has = true
		dst.V = nil
	case FRaw:
		dst, ok := f.Dst.(*[]byte)
		if !ok || dst == nil {
			return s.set(Bug, Loc{}, "bad_raw_dst")
		}
		*dst = s.buf[ev.Loc.Off : ev.Loc.Off+4]
	default:
		return s.set(Prov, ev.Loc, fldWant(f.Typ))
	}
	s.markSeen(s.cur)
	s.cur = -1
	s.expectVal = false
	return nil
}

func (s *fldState) storeStr(f *Fld, dec []byte, loc Loc) error {
	switch f.Typ {
	case FStr:
		dst, ok := f.Dst.(*string)
		if !ok || dst == nil {
			return s.set(Bug, Loc{}, "bad_str_dst")
		}
		*dst = string(dec)
	case FBytes:
		dst, ok := f.Dst.(*[]byte)
		if !ok || dst == nil {
			return s.set(Bug, Loc{}, "bad_bytes_dst")
		}
		*dst = dec
	case FOStr:
		dst, ok := f.Dst.(*OptStr)
		if !ok || dst == nil {
			return s.set(Bug, Loc{}, "bad_ostr_dst")
		}
		dst.Has = true
		dst.V = string(dec)
	case FOBytes:
		dst, ok := f.Dst.(*OptBytes)
		if !ok || dst == nil {
			return s.set(Bug, Loc{}, "bad_obytes_dst")
		}
		dst.Has = true
		dst.V = dec
	default:
		return s.set(Prov, loc, fldWant(f.Typ))
	}
	return nil
}

func (s *fldState) storeNum(f *Fld, raw []byte, loc Loc) error {
	switch f.Typ {
	case FI32:
		v, ok := fldParseI32(raw)
		if !ok {
			return s.set(Prov, loc, "num_i32")
		}
		dst, ok := f.Dst.(*int32)
		if !ok || dst == nil {
			return s.set(Bug, Loc{}, "bad_i32_dst")
		}
		*dst = v
	case FU32:
		v, ok := fldParseU32(raw)
		if !ok {
			return s.set(Prov, loc, "num_u32")
		}
		dst, ok := f.Dst.(*uint32)
		if !ok || dst == nil {
			return s.set(Bug, Loc{}, "bad_u32_dst")
		}
		*dst = v
	case FF64:
		v, err := strconv.ParseFloat(string(raw), 64)
		if err != nil {
			return s.set(Prov, loc, "num_f64")
		}
		dst, ok := f.Dst.(*float64)
		if !ok || dst == nil {
			return s.set(Bug, Loc{}, "bad_f64_dst")
		}
		*dst = v
	case FOI32:
		v, ok := fldParseI32(raw)
		if !ok {
			return s.set(Prov, loc, "num_oi32")
		}
		dst, ok := f.Dst.(*OptI32)
		if !ok || dst == nil {
			return s.set(Bug, Loc{}, "bad_oi32_dst")
		}
		dst.Has = true
		dst.V = v
	case FOU32:
		v, ok := fldParseU32(raw)
		if !ok {
			return s.set(Prov, loc, "num_ou32")
		}
		dst, ok := f.Dst.(*OptU32)
		if !ok || dst == nil {
			return s.set(Bug, Loc{}, "bad_ou32_dst")
		}
		dst.Has = true
		dst.V = v
	case FOF64:
		v, err := strconv.ParseFloat(string(raw), 64)
		if err != nil {
			return s.set(Prov, loc, "num_of64")
		}
		dst, ok := f.Dst.(*OptF64)
		if !ok || dst == nil {
			return s.set(Bug, Loc{}, "bad_of64_dst")
		}
		dst.Has = true
		dst.V = v
	default:
		return s.set(Prov, loc, fldWant(f.Typ))
	}
	return nil
}

func (s *fldState) storeBool(f *Fld, v bool, loc Loc) error {
	switch f.Typ {
	case FBool:
		dst, ok := f.Dst.(*bool)
		if !ok || dst == nil {
			return s.set(Bug, Loc{}, "bad_bool_dst")
		}
		*dst = v
	case FOBool:
		dst, ok := f.Dst.(*OptBool)
		if !ok || dst == nil {
			return s.set(Bug, Loc{}, "bad_obool_dst")
		}
		dst.Has = true
		dst.V = v
	default:
		return s.set(Prov, loc, fldWant(f.Typ))
	}
	return nil
}

func fldWant(t FTyp) string {
	switch t {
	case FStr, FBytes, FOStr, FOBytes:
		return "want_str"
	case FI32, FU32, FF64, FOI32, FOU32, FOF64:
		return "want_num"
	case FBool, FOBool:
		return "want_bool"
	case FRaw:
		return "want_val"
	default:
		return "fld_typ"
	}
}

func fldKeyEq(raw []byte, key string, tmp *[]byte) bool {
	if !hasEsc(raw) {
		if len(raw) != len(key) {
			return false
		}
		for i, c := range raw {
			if c != key[i] {
				return false
			}
		}
		return true
	}
	*tmp = (*tmp)[:0]
	dec, ok := fldStrDecode(raw, tmp)
	if !ok || len(dec) != len(key) {
		return false
	}
	for i, c := range dec {
		if c != key[i] {
			return false
		}
	}
	return true
}

func fldStrBytes(raw []byte, a *Arena) ([]byte, error) {
	if !hasEsc(raw) {
		return raw, nil
	}
	if a == nil {
		return nil, fail(Call, StPar, Loc{}, "arg_arena", nil)
	}
	tmp := a.B
	_, ok := fldStrDecode(raw, &tmp)
	if !ok {
		return nil, fail(Prov, StPar, Loc{}, "str", nil)
	}
	beg := len(a.B)
	a.B = tmp
	return a.B[beg:len(a.B)], nil
}

func hasEsc(b []byte) bool {
	for _, c := range b {
		if c == '\\' {
			return true
		}
	}
	return false
}

func fldStrDecode(raw []byte, dst *[]byte) ([]byte, bool) {
	beg := len(*dst)
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if ch != '\\' {
			*dst = append(*dst, ch)
			continue
		}
		i++
		if i >= len(raw) {
			return nil, false
		}
		switch raw[i] {
		case '"', '\\', '/':
			*dst = append(*dst, raw[i])
		case 'b':
			*dst = append(*dst, '\b')
		case 'f':
			*dst = append(*dst, '\f')
		case 'n':
			*dst = append(*dst, '\n')
		case 'r':
			*dst = append(*dst, '\r')
		case 't':
			*dst = append(*dst, '\t')
		case 'u':
			r, n, ok := fldEscRune(raw[i+1:])
			if !ok {
				return nil, false
			}
			i += n
			var enc [utf8.UTFMax]byte
			m := utf8.EncodeRune(enc[:], r)
			*dst = append(*dst, enc[:m]...)
		default:
			return nil, false
		}
	}
	return (*dst)[beg:], true
}

func fldEscRune(raw []byte) (rune, int, bool) {
	if len(raw) < 4 {
		return 0, 0, false
	}
	r, ok := fldHex4(raw[:4])
	if !ok {
		return 0, 0, false
	}
	if r >= 0xd800 && r <= 0xdbff {
		if len(raw) < 10 || raw[4] != '\\' || raw[5] != 'u' {
			return 0, 0, false
		}
		lo, ok := fldHex4(raw[6:10])
		if !ok || lo < 0xdc00 || lo > 0xdfff {
			return 0, 0, false
		}
		return 0x10000 + ((r - 0xd800) << 10) + (lo - 0xdc00), 10, true
	}
	if r >= 0xdc00 && r <= 0xdfff {
		return 0, 0, false
	}
	return r, 4, true
}

func fldHex4(b []byte) (rune, bool) {
	var v rune
	for i := 0; i < 4; i++ {
		c := b[i]
		v <<= 4
		switch {
		case c >= '0' && c <= '9':
			v |= rune(c - '0')
		case c >= 'a' && c <= 'f':
			v |= rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v |= rune(c-'A') + 10
		default:
			return 0, false
		}
	}
	return v, true
}

func fldParseI32(raw []byte) (int32, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	neg := raw[0] == '-'
	i := 0
	if neg {
		i = 1
		if i == len(raw) {
			return 0, false
		}
	}
	var n int64
	for ; i < len(raw); i++ {
		c := raw[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int64(c-'0')
		if !neg && n > 2147483647 {
			return 0, false
		}
		if neg && n > 2147483648 {
			return 0, false
		}
	}
	if neg {
		return int32(-n), true
	}
	return int32(n), true
}

func fldParseU32(raw []byte) (uint32, bool) {
	if len(raw) == 0 || raw[0] == '-' {
		return 0, false
	}
	var n uint64
	for _, c := range raw {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + uint64(c-'0')
		if n > 4294967295 {
			return 0, false
		}
	}
	return uint32(n), true
}
