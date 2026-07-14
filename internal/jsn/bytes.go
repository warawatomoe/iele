package jsn

type bpar struct {
	on     EventFunc
	fld    *fldState
	flg    uint
	maxdep uint
	buf    []byte
	pos    uint
	off    uint
	row    uint
	col    uint
}

func (p *bpar) now() Loc {
	return Loc{Off: p.off, Row: p.row, Col: p.col}
}

func (p *bpar) set(cat Cat, stg Stage, loc Loc, cause string, err error) error {
	return fail(cat, stg, loc, cause, err)
}

func (p *bpar) run() error {
	if err := p.bom(); err != nil {
		return err
	}
	if err := p.val(0); err != nil {
		return err
	}
	p.skip()
	if p.pos < uint(len(p.buf)) {
		return p.set(Prov, StPar, p.now(), "trail", nil)
	}
	return nil
}

func (p *bpar) has() bool {
	return p.pos < uint(len(p.buf))
}

func (p *bpar) peek() (byte, Loc, bool) {
	if !p.has() {
		return 0, Loc{}, false
	}
	return p.buf[p.pos], p.now(), true
}

func (p *bpar) step(ch byte) {
	p.pos++
	p.off++
	if ch == '\n' {
		p.row++
		p.col = 1
	} else {
		p.col++
	}
}

func (p *bpar) stepRun(n uint) {
	p.pos += n
	p.off += n
	p.col += n
}

func (p *bpar) take() (byte, Loc, bool) {
	if !p.has() {
		return 0, Loc{}, false
	}
	ch := p.buf[p.pos]
	loc := p.now()
	p.step(ch)
	return ch, loc, true
}

func (p *bpar) emit(k Kind, data []byte, loc Loc) error {
	if p.fld != nil {
		return p.fld.on(Event{Kind: k, Data: data, Loc: loc})
	}
	if p.on == nil {
		return nil
	}
	if err := p.on(Event{Kind: k, Data: data, Loc: loc}); err != nil {
		return p.set(Call, StPar, loc, "cb_fail", err)
	}
	return nil
}

func (p *bpar) skip() {
	for p.pos < uint(len(p.buf)) {
		switch p.buf[p.pos] {
		case ' ', '\t', '\r':
			p.pos++
			p.off++
			p.col++
		case '\n':
			p.pos++
			p.off++
			p.row++
			p.col = 1
		default:
			return
		}
	}
}

func (p *bpar) scanDigits() Loc {
	n := uint(0)
	for p.pos+n < uint(len(p.buf)) && isdg(p.buf[p.pos+n]) {
		n++
	}
	loc := Loc{Off: p.off + n - 1, Row: p.row, Col: p.col + n - 1}
	p.stepRun(n)
	return loc
}

func (p *bpar) ctok(min byte, max byte) error {
	ch, loc, ok := p.peek()
	if !ok {
		return p.set(Prov, StTok, p.now(), "utf8", nil)
	}
	if ch < min || ch > max {
		return p.set(Prov, StTok, loc, "utf8", nil)
	}
	p.step(ch)
	return nil
}

func (p *bpar) u8() error {
	b0, loc, ok := p.peek()
	if !ok {
		return p.set(Prov, StTok, p.now(), "utf8", nil)
	}
	if b0 < 0x80 {
		return p.set(Bug, StTok, loc, "u8_call", nil)
	}
	if b0 >= 0xc2 && b0 <= 0xdf {
		p.step(b0)
		return p.ctok(0x80, 0xbf)
	}
	if b0 == 0xe0 {
		p.step(b0)
		if err := p.ctok(0xa0, 0xbf); err != nil {
			return err
		}
		return p.ctok(0x80, 0xbf)
	}
	if b0 >= 0xe1 && b0 <= 0xec {
		p.step(b0)
		if err := p.ctok(0x80, 0xbf); err != nil {
			return err
		}
		return p.ctok(0x80, 0xbf)
	}
	if b0 == 0xed {
		p.step(b0)
		if err := p.ctok(0x80, 0x9f); err != nil {
			return err
		}
		return p.ctok(0x80, 0xbf)
	}
	if b0 >= 0xee && b0 <= 0xef {
		p.step(b0)
		if err := p.ctok(0x80, 0xbf); err != nil {
			return err
		}
		return p.ctok(0x80, 0xbf)
	}
	if b0 == 0xf0 {
		p.step(b0)
		if err := p.ctok(0x90, 0xbf); err != nil {
			return err
		}
		if err := p.ctok(0x80, 0xbf); err != nil {
			return err
		}
		return p.ctok(0x80, 0xbf)
	}
	if b0 >= 0xf1 && b0 <= 0xf3 {
		p.step(b0)
		if err := p.ctok(0x80, 0xbf); err != nil {
			return err
		}
		if err := p.ctok(0x80, 0xbf); err != nil {
			return err
		}
		return p.ctok(0x80, 0xbf)
	}
	if b0 == 0xf4 {
		p.step(b0)
		if err := p.ctok(0x80, 0x8f); err != nil {
			return err
		}
		if err := p.ctok(0x80, 0xbf); err != nil {
			return err
		}
		return p.ctok(0x80, 0xbf)
	}
	return p.set(Prov, StTok, loc, "utf8", nil)
}

func (p *bpar) esc() error {
	ch, loc, ok := p.peek()
	if !ok {
		return p.set(Prov, StTok, p.now(), "esc_eof", nil)
	}
	if ch == '"' || ch == '\\' || ch == '/' || ch == 'b' ||
		ch == 'f' || ch == 'n' || ch == 'r' || ch == 't' {
		p.step(ch)
		return nil
	}
	if ch != 'u' {
		return p.set(Prov, StTok, loc, "bad_esc", nil)
	}
	p.step(ch)
	for i := 0; i < 4; i++ {
		ch, loc, ok = p.peek()
		if !ok {
			return p.set(Prov, StTok, p.now(), "u4_len", nil)
		}
		if !ishex(ch) {
			return p.set(Prov, StTok, loc, "u4_hex", nil)
		}
		p.step(ch)
	}
	return nil
}

func (p *bpar) str(key bool) error {
	evb, evp, eve := EvSBeg, EvSPar, EvSEnd
	if key {
		evb, evp, eve = EvKBeg, EvKPar, EvKEnd
	}
	ch, qloc, ok := p.take()
	if !ok {
		return p.set(Prov, StTok, p.now(), "tok_eof", nil)
	}
	if ch != '"' {
		return p.set(Prov, StTok, qloc, "bad_tok", nil)
	}
	if err := p.emit(evb, nil, qloc); err != nil {
		return err
	}
	start := p.pos
	sloc := p.now()
	for {
		if !p.has() {
			return p.set(Prov, StTok, p.now(), "str_eof", nil)
		}
		ch := p.buf[p.pos]
		if ch >= 0x20 && ch != '"' && ch != '\\' && ch < 0x80 {
			n := uint(1)
			for p.pos+n < uint(len(p.buf)) {
				c := p.buf[p.pos+n]
				if c < 0x20 || c == '"' || c == '\\' || c >= 0x80 {
					break
				}
				n++
			}
			p.stepRun(n)
			continue
		}
		loc := p.now()
		if ch == '"' {
			if p.pos > start {
				if err := p.emit(evp, p.buf[start:p.pos], sloc); err != nil {
					return err
				}
			}
			p.step(ch)
			return p.emit(eve, nil, loc)
		}
		if ch < 0x20 {
			return p.set(Prov, StTok, loc, "ctl_str", nil)
		}
		if ch == '\\' {
			p.step(ch)
			if err := p.esc(); err != nil {
				return err
			}
			continue
		}
		if ch < 0x80 {
			p.step(ch)
			continue
		}
		if err := p.u8(); err != nil {
			return err
		}
	}
}

func (p *bpar) num() error {
	ch, loc, ok := p.peek()
	if !ok {
		return p.set(Prov, StTok, p.now(), "bad_num", nil)
	}
	start := p.pos
	sloc := loc
	nloc := loc
	if err := p.emit(EvNBeg, nil, loc); err != nil {
		return err
	}
	if ch == '-' {
		p.step(ch)
		nloc = loc
		ch, loc, ok = p.peek()
		if !ok {
			return p.set(Prov, StTok, p.now(), "bad_num", nil)
		}
	}
	if ch == '0' {
		p.step(ch)
		nloc = loc
		ch, loc, ok = p.peek()
	} else {
		if ch < '1' || ch > '9' {
			return p.set(Prov, StTok, loc, "bad_num", nil)
		}
		nloc = p.scanDigits()
		ch, loc, ok = p.peek()
	}
	if ok && ch == '.' {
		p.step(ch)
		nloc = loc
		ch, loc, ok = p.peek()
		if !ok || !isdg(ch) {
			return p.set(Prov, StTok, loc, "bad_num", nil)
		}
		nloc = p.scanDigits()
		ch, loc, ok = p.peek()
	}
	if ok && (ch == 'e' || ch == 'E') {
		p.step(ch)
		nloc = loc
		ch, loc, ok = p.peek()
		if !ok {
			return p.set(Prov, StTok, p.now(), "bad_num", nil)
		}
		if ch == '+' || ch == '-' {
			p.step(ch)
			nloc = loc
			ch, loc, ok = p.peek()
			if !ok {
				return p.set(Prov, StTok, p.now(), "bad_num", nil)
			}
		}
		if !isdg(ch) {
			return p.set(Prov, StTok, loc, "bad_num", nil)
		}
		nloc = p.scanDigits()
		ch, loc, ok = p.peek()
	}
	if err := p.emit(EvNPar, p.buf[start:p.pos], sloc); err != nil {
		return err
	}
	if ok && !isend(ch) {
		return p.set(Prov, StTok, loc, "bad_num", nil)
	}
	return p.emit(EvNEnd, nil, nloc)
}

func (p *bpar) lit(txt string, k Kind) error {
	_, sloc, ok := p.peek()
	if !ok {
		return p.set(Prov, StTok, p.now(), "bad_tok", nil)
	}
	for i := 0; i < len(txt); i++ {
		ch, loc, ok := p.peek()
		if !ok || ch != txt[i] {
			return p.set(Prov, StTok, loc, "bad_tok", nil)
		}
		p.step(ch)
	}
	ch, loc, ok := p.peek()
	if ok && !isend(ch) {
		return p.set(Prov, StTok, loc, "bad_tok", nil)
	}
	return p.emit(k, nil, sloc)
}

func (p *bpar) obj(dep uint) error {
	p.skip()
	ch, loc, ok := p.peek()
	if !ok {
		return p.set(Prov, StPar, p.now(), "want_key", nil)
	}
	if ch == '}' {
		p.step(ch)
		return p.emit(EvOEnd, nil, loc)
	}
	for {
		if ch != '"' {
			return p.set(Prov, StPar, loc, "want_key", nil)
		}
		if err := p.str(true); err != nil {
			return err
		}
		p.skip()
		ch, loc, ok = p.take()
		if !ok {
			return p.set(Prov, StTok, p.now(), "tok_eof", nil)
		}
		if ch != ':' {
			return p.set(Prov, StPar, loc, "want_col", nil)
		}
		if err := p.val(dep); err != nil {
			return err
		}
		p.skip()
		ch, loc, ok = p.peek()
		if !ok {
			return p.set(Prov, StPar, p.now(), "want_oend", nil)
		}
		if ch == ',' {
			p.step(ch)
			p.skip()
			ch, loc, ok = p.peek()
			if !ok {
				return p.set(Prov, StPar, p.now(), "want_key", nil)
			}
			continue
		}
		if ch == '}' {
			p.step(ch)
			return p.emit(EvOEnd, nil, loc)
		}
		return p.set(Prov, StPar, loc, "want_oend", nil)
	}
}

func (p *bpar) arr(dep uint) error {
	p.skip()
	ch, loc, ok := p.peek()
	if !ok {
		return p.set(Prov, StPar, p.now(), "want_aend", nil)
	}
	if ch == ']' {
		p.step(ch)
		return p.emit(EvAEnd, nil, loc)
	}
	for {
		if err := p.val(dep); err != nil {
			return err
		}
		p.skip()
		ch, loc, ok = p.peek()
		if !ok {
			return p.set(Prov, StPar, p.now(), "want_aend", nil)
		}
		if ch == ',' {
			p.step(ch)
			continue
		}
		if ch == ']' {
			p.step(ch)
			return p.emit(EvAEnd, nil, loc)
		}
		return p.set(Prov, StPar, loc, "want_aend", nil)
	}
}

func (p *bpar) val(dep uint) error {
	p.skip()
	ch, loc, ok := p.peek()
	if !ok {
		return p.set(Prov, StPar, p.now(), "tok_eof", nil)
	}
	if ch == '{' {
		if dep+1 > p.maxdep {
			return p.set(Cap, StPar, loc, "max_dep", nil)
		}
		p.step(ch)
		if err := p.emit(EvOBeg, nil, loc); err != nil {
			return err
		}
		return p.obj(dep + 1)
	}
	if ch == '[' {
		if dep+1 > p.maxdep {
			return p.set(Cap, StPar, loc, "max_dep", nil)
		}
		p.step(ch)
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

func (p *bpar) bom() error {
	if p.flg&PFBOM == 0 {
		return nil
	}
	if !p.has() || p.buf[p.pos] != 0xef {
		return nil
	}
	loc0 := p.now()
	p.step(p.buf[p.pos])
	if !p.has() || p.buf[p.pos] != 0xbb {
		return p.set(Prov, StTok, loc0, "bad_tok", nil)
	}
	p.step(p.buf[p.pos])
	if !p.has() || p.buf[p.pos] != 0xbf {
		return p.set(Prov, StTok, loc0, "bad_tok", nil)
	}
	p.step(p.buf[p.pos])
	return nil
}
