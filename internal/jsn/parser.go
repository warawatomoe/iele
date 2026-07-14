package jsn

const (
	MaxDep = 256

	PFBOM uint = 1 << iota
	PFDup
)

type Kind uint8

const (
	EvOBeg Kind = iota + 1
	EvOEnd
	EvABeg
	EvAEnd
	EvKBeg
	EvKPar
	EvKEnd
	EvSBeg
	EvSPar
	EvSEnd
	EvNBeg
	EvNPar
	EvNEnd
	EvTrue
	EvFalse
	EvNull
)

type Event struct {
	Kind Kind
	Data []byte
	Loc  Loc
}

type Pull func() ([]byte, bool, error)
type EventFunc func(Event) error

type Opt struct {
	Flg    uint
	MaxDep uint
}

func Parse(pull Pull, on EventFunc, opt *Opt) error {
	if pull == nil {
		return fail(Call, StPar, Loc{}, "arg_pull", nil)
	}

	maxdep := uint(MaxDep)
	flg := uint(0)
	if opt != nil {
		flg = opt.Flg
		if opt.MaxDep != 0 {
			maxdep = opt.MaxDep
		}
	}
	if flg&PFDup != 0 {
		return fail(Call, StPar, Loc{}, "dup_chk", nil)
	}
	if maxdep == 0 || maxdep > MaxDep {
		return fail(Call, StPar, Loc{}, "arg_dep", nil)
	}

	p := par{
		pull:   pull,
		on:     on,
		flg:    flg,
		maxdep: maxdep,
		row:    1,
		col:    1,
	}
	return p.run()
}

func ParseBytes(b []byte, on EventFunc, opt *Opt) error {
	maxdep := uint(MaxDep)
	flg := uint(0)
	if opt != nil {
		flg = opt.Flg
		if opt.MaxDep != 0 {
			maxdep = opt.MaxDep
		}
	}
	if flg&PFDup != 0 {
		return fail(Call, StPar, Loc{}, "dup_chk", nil)
	}
	if maxdep == 0 || maxdep > MaxDep {
		return fail(Call, StPar, Loc{}, "arg_dep", nil)
	}

	p := bpar{
		on:     on,
		flg:    flg,
		maxdep: maxdep,
		buf:    b,
		row:    1,
		col:    1,
	}
	return p.run()
}

func (p *par) run() error {
	if err := p.bom(); err != nil {
		return err
	}
	if err := p.val(0); err != nil {
		return err
	}
	if err := p.skip(); err != nil {
		return err
	}
	has, _, loc, err := p.peek()
	if err != nil {
		return err
	}
	if has {
		return p.set(Prov, StPar, loc, "trail", nil)
	}
	return nil
}

type par struct {
	pull   Pull
	on     EventFunc
	flg    uint
	maxdep uint
	buf    []byte
	pos    uint
	off    uint
	row    uint
	col    uint
	eof    bool
}

type seg struct {
	buf []byte
	pos uint
	loc Loc
}

func (p *par) set(cat Cat, stg Stage, loc Loc, cause string, err error) error {
	return fail(cat, stg, loc, cause, err)
}

func (p *par) now() Loc {
	return Loc{Off: p.off, Row: p.row, Col: p.col}
}

func isws(ch byte) bool {
	return ch == 0x20 || ch == 0x09 || ch == 0x0a || ch == 0x0d
}

func isdg(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isend(ch byte) bool {
	return isws(ch) || ch == ',' || ch == ']' || ch == '}' || ch == ':'
}

func ishex(ch byte) bool {
	return (ch >= '0' && ch <= '9') ||
		(ch >= 'a' && ch <= 'f') ||
		(ch >= 'A' && ch <= 'F')
}

func (p *par) step(ch byte) {
	p.pos++
	p.off++
	if ch == '\n' {
		p.row++
		p.col = 1
	} else {
		p.col++
	}
}

func (p *par) refil() error {
	if p.pos < uint(len(p.buf)) || p.eof {
		return nil
	}
	buf, ok, err := p.pull()
	if err != nil {
		return p.set(Trans, StTok, p.now(), "pull", err)
	}
	if !ok {
		p.eof = true
		return nil
	}
	if len(buf) == 0 {
		return p.set(Prov, StTok, p.now(), "pull_buf", nil)
	}
	p.buf = buf
	p.pos = 0
	return nil
}

func (p *par) peek() (bool, byte, Loc, error) {
	if err := p.refil(); err != nil {
		return false, 0, Loc{}, err
	}
	if p.pos >= uint(len(p.buf)) {
		return false, 0, Loc{}, nil
	}
	return true, p.buf[p.pos], p.now(), nil
}

func (p *par) take1() (byte, Loc) {
	ch := p.buf[p.pos]
	loc := p.now()
	p.step(ch)
	return ch, loc
}

func (p *par) take() (byte, Loc, error) {
	has, _, _, err := p.peek()
	if err != nil {
		return 0, Loc{}, err
	}
	if !has {
		return 0, Loc{}, p.set(Prov, StTok, p.now(), "tok_eof", nil)
	}
	ch, loc := p.take1()
	return ch, loc, nil
}

func (p *par) emit(k Kind, data []byte, loc Loc) error {
	if p.on == nil {
		return nil
	}
	if err := p.on(Event{Kind: k, Data: data, Loc: loc}); err != nil {
		return p.set(Call, StPar, loc, "cb_fail", err)
	}
	return nil
}

func (p *par) skip() error {
	for {
		has, ch, _, err := p.peek()
		if err != nil {
			return err
		}
		if !has || !isws(ch) {
			return nil
		}
		p.take1()
	}
}

func (p *par) sini(s *seg) {
	s.buf = p.buf
	s.pos = p.pos
	s.loc = p.now()
}

func (p *par) sput(s *seg, k Kind) error {
	if s.buf == nil {
		return nil
	}
	if len(s.buf) > 0 && &s.buf[0] != &p.buf[0] {
		return p.set(Bug, StTok, p.now(), "seg_buf", nil)
	}
	if p.pos < s.pos {
		return p.set(Bug, StTok, p.now(), "seg_pos", nil)
	}
	if p.pos == s.pos {
		return nil
	}
	if err := p.emit(k, s.buf[s.pos:p.pos], s.loc); err != nil {
		return err
	}
	s.pos = p.pos
	s.loc = p.now()
	return nil
}

func (p *par) speek(s *seg, k Kind) (bool, byte, Loc, error) {
	for p.pos >= uint(len(p.buf)) {
		if err := p.sput(s, k); err != nil {
			return false, 0, Loc{}, err
		}
		if err := p.refil(); err != nil {
			return false, 0, Loc{}, err
		}
		if p.pos >= uint(len(p.buf)) {
			return false, 0, Loc{}, nil
		}
		p.sini(s)
	}
	return true, p.buf[p.pos], p.now(), nil
}

func (p *par) ctok(s *seg, k Kind, min byte, max byte) error {
	has, ch, loc, err := p.speek(s, k)
	if err != nil {
		return err
	}
	if !has {
		return p.set(Prov, StTok, p.now(), "utf8", nil)
	}
	if ch < min || ch > max {
		return p.set(Prov, StTok, loc, "utf8", nil)
	}
	p.take1()
	return nil
}

func (p *par) u8(s *seg, k Kind) error {
	has, b0, loc, err := p.speek(s, k)
	if err != nil {
		return err
	}
	if !has {
		return p.set(Prov, StTok, p.now(), "utf8", nil)
	}
	if b0 < 0x80 {
		return p.set(Bug, StTok, loc, "u8_call", nil)
	}
	if b0 >= 0xc2 && b0 <= 0xdf {
		p.take1()
		return p.ctok(s, k, 0x80, 0xbf)
	}
	if b0 == 0xe0 {
		p.take1()
		if err := p.ctok(s, k, 0xa0, 0xbf); err != nil {
			return err
		}
		return p.ctok(s, k, 0x80, 0xbf)
	}
	if b0 >= 0xe1 && b0 <= 0xec {
		p.take1()
		if err := p.ctok(s, k, 0x80, 0xbf); err != nil {
			return err
		}
		return p.ctok(s, k, 0x80, 0xbf)
	}
	if b0 == 0xed {
		p.take1()
		if err := p.ctok(s, k, 0x80, 0x9f); err != nil {
			return err
		}
		return p.ctok(s, k, 0x80, 0xbf)
	}
	if b0 >= 0xee && b0 <= 0xef {
		p.take1()
		if err := p.ctok(s, k, 0x80, 0xbf); err != nil {
			return err
		}
		return p.ctok(s, k, 0x80, 0xbf)
	}
	if b0 == 0xf0 {
		p.take1()
		if err := p.ctok(s, k, 0x90, 0xbf); err != nil {
			return err
		}
		if err := p.ctok(s, k, 0x80, 0xbf); err != nil {
			return err
		}
		return p.ctok(s, k, 0x80, 0xbf)
	}
	if b0 >= 0xf1 && b0 <= 0xf3 {
		p.take1()
		if err := p.ctok(s, k, 0x80, 0xbf); err != nil {
			return err
		}
		if err := p.ctok(s, k, 0x80, 0xbf); err != nil {
			return err
		}
		return p.ctok(s, k, 0x80, 0xbf)
	}
	if b0 == 0xf4 {
		p.take1()
		if err := p.ctok(s, k, 0x80, 0x8f); err != nil {
			return err
		}
		if err := p.ctok(s, k, 0x80, 0xbf); err != nil {
			return err
		}
		return p.ctok(s, k, 0x80, 0xbf)
	}
	return p.set(Prov, StTok, loc, "utf8", nil)
}

func (p *par) esc(s *seg, k Kind) error {
	has, ch, loc, err := p.speek(s, k)
	if err != nil {
		return err
	}
	if !has {
		return p.set(Prov, StTok, p.now(), "esc_eof", nil)
	}
	if ch == '"' || ch == '\\' || ch == '/' || ch == 'b' ||
		ch == 'f' || ch == 'n' || ch == 'r' || ch == 't' {
		p.take1()
		return nil
	}
	if ch != 'u' {
		return p.set(Prov, StTok, loc, "bad_esc", nil)
	}
	p.take1()
	for i := 0; i < 4; i++ {
		has, ch, loc, err = p.speek(s, k)
		if err != nil {
			return err
		}
		if !has {
			return p.set(Prov, StTok, p.now(), "u4_len", nil)
		}
		if !ishex(ch) {
			return p.set(Prov, StTok, loc, "u4_hex", nil)
		}
		p.take1()
	}
	return nil
}

func (p *par) str(key bool) error {
	evb, evp, eve := EvSBeg, EvSPar, EvSEnd
	if key {
		evb, evp, eve = EvKBeg, EvKPar, EvKEnd
	}
	ch, qloc, err := p.take()
	if err != nil {
		return err
	}
	if ch != '"' {
		return p.set(Prov, StTok, qloc, "bad_tok", nil)
	}
	if err := p.emit(evb, nil, qloc); err != nil {
		return err
	}
	var s seg
	p.sini(&s)
	for {
		has, ch, loc, err := p.speek(&s, evp)
		if err != nil {
			return err
		}
		if !has {
			return p.set(Prov, StTok, p.now(), "str_eof", nil)
		}
		if ch == '"' {
			if err := p.sput(&s, evp); err != nil {
				return err
			}
			p.take1()
			return p.emit(eve, nil, loc)
		}
		if ch < 0x20 {
			return p.set(Prov, StTok, loc, "ctl_str", nil)
		}
		if ch == '\\' {
			p.take1()
			if err := p.esc(&s, evp); err != nil {
				return err
			}
			continue
		}
		if ch < 0x80 {
			p.take1()
			continue
		}
		if err := p.u8(&s, evp); err != nil {
			return err
		}
	}
}

func (p *par) num() error {
	var s seg
	p.sini(&s)
	has, ch, loc, err := p.speek(&s, EvNPar)
	if err != nil {
		return err
	}
	if !has {
		return p.set(Prov, StTok, p.now(), "bad_num", nil)
	}
	nloc := loc
	if err := p.emit(EvNBeg, nil, loc); err != nil {
		return err
	}
	if ch == '-' {
		p.take1()
		nloc = loc
		has, ch, loc, err = p.speek(&s, EvNPar)
		if err != nil {
			return err
		}
		if !has {
			return p.set(Prov, StTok, p.now(), "bad_num", nil)
		}
	}
	if ch == '0' {
		p.take1()
		nloc = loc
		has, ch, loc, err = p.speek(&s, EvNPar)
		if err != nil {
			return err
		}
	} else {
		if ch < '1' || ch > '9' {
			return p.set(Prov, StTok, loc, "bad_num", nil)
		}
		for has && isdg(ch) {
			p.take1()
			nloc = loc
			has, ch, loc, err = p.speek(&s, EvNPar)
			if err != nil {
				return err
			}
		}
	}
	if has && ch == '.' {
		p.take1()
		nloc = loc
		has, ch, loc, err = p.speek(&s, EvNPar)
		if err != nil {
			return err
		}
		if !has || !isdg(ch) {
			return p.set(Prov, StTok, loc, "bad_num", nil)
		}
		for has && isdg(ch) {
			p.take1()
			nloc = loc
			has, ch, loc, err = p.speek(&s, EvNPar)
			if err != nil {
				return err
			}
		}
	}
	if has && (ch == 'e' || ch == 'E') {
		p.take1()
		nloc = loc
		has, ch, loc, err = p.speek(&s, EvNPar)
		if err != nil {
			return err
		}
		if !has {
			return p.set(Prov, StTok, p.now(), "bad_num", nil)
		}
		if ch == '+' || ch == '-' {
			p.take1()
			nloc = loc
			has, ch, loc, err = p.speek(&s, EvNPar)
			if err != nil {
				return err
			}
			if !has {
				return p.set(Prov, StTok, p.now(), "bad_num", nil)
			}
		}
		if !isdg(ch) {
			return p.set(Prov, StTok, loc, "bad_num", nil)
		}
		for has && isdg(ch) {
			p.take1()
			nloc = loc
			has, ch, loc, err = p.speek(&s, EvNPar)
			if err != nil {
				return err
			}
		}
	}
	if err := p.sput(&s, EvNPar); err != nil {
		return err
	}
	if has && !isend(ch) {
		return p.set(Prov, StTok, loc, "bad_num", nil)
	}
	return p.emit(EvNEnd, nil, nloc)
}

func (p *par) lit(txt string, k Kind) error {
	has, _, sloc, err := p.peek()
	if err != nil {
		return err
	}
	if !has {
		return p.set(Prov, StTok, p.now(), "bad_tok", nil)
	}
	for i := 0; i < len(txt); i++ {
		has, ch, loc, err := p.peek()
		if err != nil {
			return err
		}
		if !has || ch != txt[i] {
			return p.set(Prov, StTok, loc, "bad_tok", nil)
		}
		p.take1()
	}
	has, ch, loc, err := p.peek()
	if err != nil {
		return err
	}
	if has && !isend(ch) {
		return p.set(Prov, StTok, loc, "bad_tok", nil)
	}
	return p.emit(k, nil, sloc)
}

func (p *par) obj(dep uint) error {
	if err := p.skip(); err != nil {
		return err
	}
	has, ch, loc, err := p.peek()
	if err != nil {
		return err
	}
	if !has {
		return p.set(Prov, StPar, p.now(), "want_key", nil)
	}
	if ch == '}' {
		p.take1()
		return p.emit(EvOEnd, nil, loc)
	}
	for {
		if ch != '"' {
			return p.set(Prov, StPar, loc, "want_key", nil)
		}
		if err := p.str(true); err != nil {
			return err
		}
		if err := p.skip(); err != nil {
			return err
		}
		ch, loc, err = p.take()
		if err != nil {
			return err
		}
		if ch != ':' {
			return p.set(Prov, StPar, loc, "want_col", nil)
		}
		if err := p.val(dep); err != nil {
			return err
		}
		if err := p.skip(); err != nil {
			return err
		}
		has, ch, loc, err = p.peek()
		if err != nil {
			return err
		}
		if !has {
			return p.set(Prov, StPar, p.now(), "want_oend", nil)
		}
		if ch == ',' {
			p.take1()
			if err := p.skip(); err != nil {
				return err
			}
			has, ch, loc, err = p.peek()
			if err != nil {
				return err
			}
			if !has {
				return p.set(Prov, StPar, p.now(), "want_key", nil)
			}
			continue
		}
		if ch == '}' {
			p.take1()
			return p.emit(EvOEnd, nil, loc)
		}
		return p.set(Prov, StPar, loc, "want_oend", nil)
	}
}

func (p *par) arr(dep uint) error {
	if err := p.skip(); err != nil {
		return err
	}
	has, ch, loc, err := p.peek()
	if err != nil {
		return err
	}
	if !has {
		return p.set(Prov, StPar, p.now(), "want_aend", nil)
	}
	if ch == ']' {
		p.take1()
		return p.emit(EvAEnd, nil, loc)
	}
	for {
		if err := p.val(dep); err != nil {
			return err
		}
		if err := p.skip(); err != nil {
			return err
		}
		has, ch, loc, err = p.peek()
		if err != nil {
			return err
		}
		if !has {
			return p.set(Prov, StPar, p.now(), "want_aend", nil)
		}
		if ch == ',' {
			p.take1()
			continue
		}
		if ch == ']' {
			p.take1()
			return p.emit(EvAEnd, nil, loc)
		}
		return p.set(Prov, StPar, loc, "want_aend", nil)
	}
}

func (p *par) val(dep uint) error {
	if err := p.skip(); err != nil {
		return err
	}
	has, ch, loc, err := p.peek()
	if err != nil {
		return err
	}
	if !has {
		return p.set(Prov, StPar, p.now(), "tok_eof", nil)
	}
	if ch == '{' {
		if dep+1 > p.maxdep {
			return p.set(Cap, StPar, loc, "max_dep", nil)
		}
		p.take1()
		if err := p.emit(EvOBeg, nil, loc); err != nil {
			return err
		}
		return p.obj(dep + 1)
	}
	if ch == '[' {
		if dep+1 > p.maxdep {
			return p.set(Cap, StPar, loc, "max_dep", nil)
		}
		p.take1()
		if err := p.emit(EvABeg, nil, loc); err != nil {
			return err
		}
		return p.arr(dep + 1)
	}
	if ch == '"' {
		return p.str(false)
	}
	if ch == '-' || isdg(ch) {
		return p.num()
	}
	if ch == 't' {
		return p.lit("true", EvTrue)
	}
	if ch == 'f' {
		return p.lit("false", EvFalse)
	}
	if ch == 'n' {
		return p.lit("null", EvNull)
	}
	return p.set(Prov, StPar, loc, "want_val", nil)
}

func (p *par) bom() error {
	if p.flg&PFBOM == 0 {
		return nil
	}
	has, ch, loc0, err := p.peek()
	if err != nil {
		return err
	}
	if !has || ch != 0xef {
		return nil
	}
	p.take1()
	has, ch, _, err = p.peek()
	if err != nil {
		return err
	}
	if !has || ch != 0xbb {
		return p.set(Prov, StTok, loc0, "bad_tok", nil)
	}
	p.take1()
	has, ch, _, err = p.peek()
	if err != nil {
		return err
	}
	if !has || ch != 0xbf {
		return p.set(Prov, StTok, loc0, "bad_tok", nil)
	}
	p.take1()
	return nil
}
