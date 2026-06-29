package wal

import (
	"encoding/binary"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"

	e "iele/internal/err"
)

const (
	fileMagicN = 8
	fileHdrN   = 16
	recHdrN    = 32
	verV0      = 1
	endianLE   = 1
	maxU32     = 1<<32 - 1
	maxPayload = maxU32 - recHdrN
)

// Magic is an 8-byte file identifier set by the caller.
type Magic [fileMagicN]byte

// Rec is a decoded record header.
type Rec struct {
	Len  uint32
	Type uint16
	Flag uint16
	CRC  uint32
	TS   uint32
	ID   [8]byte
	Meta [8]byte
}

// Append is the input to writing a record.
type Append struct {
	Type    uint16
	Flag    uint16
	TS      uint32
	ID      *[8]byte
	Meta    *[8]byte
	Payload []byte
}

// Idx is a lightweight record index entry.
type Idx struct {
	Off  int64
	Len  uint32
	Type uint16
	Flag uint16
	CRC  uint32
	TS   uint32
	ID   [8]byte
}

// WAL is an open append log.
type WAL struct {
	f     *os.File
	end   int64 // tracked write offset; avoids a seek syscall per Write
	magic Magic
}

// Create creates a new WAL file at path with the given magic.
func Create(path string, magic Magic) (*WAL, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, e.Wrap("", e.Trans, "wal:mkdir", err)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, e.Wrap("", e.Trans, "wal:create", err)
	}
	w := &WAL{f: f, magic: magic, end: 0}
	if err := w.writeHead(); err != nil {
		f.Close()
		os.Remove(path)
		return nil, err
	}
	return w, nil
}

// Open opens an existing WAL file for appending.
func Open(path string, magic Magic) (*WAL, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return nil, e.Wrap("", e.Trans, "wal:open", err)
	}
	w := &WAL{f: f, magic: magic}
	if err := w.readHead(); err != nil {
		f.Close()
		return nil, err
	}
	// Track end so Write never needs to seek.
	end, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		f.Close()
		return nil, e.Wrap("", e.Trans, "wal:seek_end", err)
	}
	w.end = end
	return w, nil
}

// OpenOrCreate opens an existing WAL or creates a new one.
func OpenOrCreate(path string, magic Magic) (*WAL, error) {
	w, err := Open(path, magic)
	if os.IsNotExist(err) {
		return Create(path, magic)
	}
	return w, err
}

// Close closes the underlying file.
func (w *WAL) Close() error {
	return w.f.Close()
}

// Write appends a record to the WAL.
func (w *WAL) Write(a *Append) (Rec, error) {
	if a == nil {
		return Rec{}, e.New("", e.Call, "wal:write", "nil_append")
	}
	if uint64(len(a.Payload)) > maxPayload {
		return Rec{}, e.New("", e.Cap, "wal:write", "payload_u32")
	}

	r := Rec{
		Len:  uint32(recHdrN) + uint32(len(a.Payload)),
		Type: a.Type,
		Flag: a.Flag,
		CRC:  crc32.ChecksumIEEE(a.Payload),
		TS:   a.TS,
	}
	if a.ID != nil {
		r.ID = *a.ID
	}
	if a.Meta != nil {
		r.Meta = *a.Meta
	}

	hdr := encRec(r)
	// Write at tracked offset — no seek needed.
	if _, err := w.f.WriteAt(hdr[:], w.end); err != nil {
		return Rec{}, e.Wrap("", e.Trans, "wal:write_hdr", err)
	}
	w.end += recHdrN
	if len(a.Payload) > 0 {
		if _, err := w.f.WriteAt(a.Payload, w.end); err != nil {
			return Rec{}, e.Wrap("", e.Trans, "wal:write_pay", err)
		}
		w.end += int64(len(a.Payload))
	}
	return r, nil
}

// Sync flushes the WAL to disk.
func (w *WAL) Sync() error {
	if err := w.f.Sync(); err != nil {
		return e.Wrap("", e.Trans, "wal:sync", err)
	}
	return nil
}

// Scan reads all complete records from the WAL calling fn for each.
// EOF before a record header stops the scan; truncated records are corrupt.
// Payload is passed to fn — do not retain it across calls.
func (w *WAL) Scan(fn func(r Rec, payload []byte) error) error {
	if fn == nil {
		return e.New("", e.Call, "wal:scan", "nil_fn")
	}
	if _, err := w.f.Seek(fileHdrN, io.SeekStart); err != nil {
		return e.Wrap("", e.Trans, "wal:seek", err)
	}

	var hdr [recHdrN]byte
	tmp := make([]byte, 4096)

	for {
		_, err := io.ReadFull(w.f, hdr[:])
		if err == io.EOF {
			return nil
		}
		if err == io.ErrUnexpectedEOF {
			return e.New("", e.Prov, "wal:scan", "corrupt_head_trunc")
		}
		if err != nil {
			return e.Wrap("", e.Trans, "wal:read_hdr", err)
		}

		r := decRec(hdr)
		if r.Len < recHdrN {
			return e.New("", e.Prov, "wal:scan", "corrupt_rec_len")
		}
		payN := int64(r.Len) - recHdrN
		if payN > int64(maxInt()) {
			return e.New("", e.Cap, "wal:scan", "payload_cap")
		}

		if cap(tmp) < int(payN) {
			tmp = make([]byte, int(payN))
		}
		pay := tmp[:int(payN)]

		if payN > 0 {
			_, err = io.ReadFull(w.f, pay)
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return e.New("", e.Prov, "wal:scan", "corrupt_payload_trunc")
			}
			if err != nil {
				return e.Wrap("", e.Trans, "wal:read_pay", err)
			}
		}

		if crc32.ChecksumIEEE(pay) != r.CRC {
			return e.New("", e.Prov, "wal:scan", "corrupt_crc")
		}

		if err := fn(r, pay); err != nil {
			return err
		}
	}
}

// Index builds an in-memory index of all records and verifies payload CRCs.
func (w *WAL) Index() ([]Idx, error) {
	if _, err := w.f.Seek(fileHdrN, io.SeekStart); err != nil {
		return nil, e.Wrap("", e.Trans, "wal:seek", err)
	}

	var out []Idx
	off := int64(fileHdrN)
	var hdr [recHdrN]byte

	for {
		_, err := io.ReadFull(w.f, hdr[:])
		if err == io.EOF {
			return out, nil
		}
		if err == io.ErrUnexpectedEOF {
			return nil, e.New("", e.Prov, "wal:index", "corrupt_head_trunc")
		}
		if err != nil {
			return nil, e.Wrap("", e.Trans, "wal:read_hdr", err)
		}

		r := decRec(hdr)
		if r.Len < recHdrN {
			return nil, e.New("", e.Prov, "wal:index", "corrupt_rec_len")
		}

		out = append(out, Idx{
			Off:  off,
			Len:  r.Len,
			Type: r.Type,
			Flag: r.Flag,
			CRC:  r.CRC,
			TS:   r.TS,
			ID:   r.ID,
		})

		if err := w.drainPayload(r, "wal:index"); err != nil {
			return nil, err
		}
		off += int64(r.Len)
	}
}

// ReadAt reads the payload of a record at the given index entry.
func (w *WAL) ReadAt(idx Idx) ([]byte, error) {
	payN := int(idx.Len) - recHdrN
	if payN < 0 {
		return nil, e.New("", e.Prov, "wal:read_at", "corrupt_rec_len")
	}
	if payN == 0 {
		if idx.CRC != crc32.ChecksumIEEE(nil) {
			return nil, e.New("", e.Prov, "wal:read_at", "corrupt_crc")
		}
		return nil, nil
	}
	buf := make([]byte, payN)
	if _, err := w.f.ReadAt(buf, idx.Off+recHdrN); err != nil {
		return nil, e.Wrap("", e.Trans, "wal:read_at", err)
	}
	if crc32.ChecksumIEEE(buf) != idx.CRC {
		return nil, e.New("", e.Prov, "wal:read_at", "corrupt_crc")
	}
	return buf, nil
}

func (w *WAL) writeHead() error {
	var hdr [fileHdrN]byte
	copy(hdr[:fileMagicN], w.magic[:])
	binary.LittleEndian.PutUint16(hdr[8:], verV0)
	hdr[10] = endianLE
	hdr[11] = fileHdrN
	if _, err := w.f.Write(hdr[:]); err != nil {
		return e.Wrap("", e.Trans, "wal:write_head", err)
	}
	w.end = fileHdrN
	return nil
}

func (w *WAL) readHead() error {
	var hdr [fileHdrN]byte
	if _, err := io.ReadFull(w.f, hdr[:]); err != nil {
		return e.Wrap("", e.Trans, "wal:read_head", err)
	}
	if binary.LittleEndian.Uint16(hdr[8:]) != verV0 {
		return e.New("", e.Prov, "wal:head", "unsup_ver")
	}
	if hdr[10] != endianLE {
		return e.New("", e.Prov, "wal:head", "unsup_endian")
	}
	if hdr[11] != fileHdrN {
		return e.New("", e.Prov, "wal:head", "bad_hsz")
	}
	if Magic(hdr[:fileMagicN]) != w.magic {
		return e.New("", e.Prov, "wal:head", "bad_magic")
	}
	return nil
}

func encRec(r Rec) [recHdrN]byte {
	var b [recHdrN]byte
	binary.LittleEndian.PutUint32(b[0:], r.Len)
	binary.LittleEndian.PutUint16(b[4:], r.Type)
	binary.LittleEndian.PutUint16(b[6:], r.Flag)
	binary.LittleEndian.PutUint32(b[8:], r.CRC)
	binary.LittleEndian.PutUint32(b[12:], r.TS)
	copy(b[16:], r.ID[:])
	copy(b[24:], r.Meta[:])
	return b
}

func decRec(b [recHdrN]byte) Rec {
	var r Rec
	r.Len = binary.LittleEndian.Uint32(b[0:])
	r.Type = binary.LittleEndian.Uint16(b[4:])
	r.Flag = binary.LittleEndian.Uint16(b[6:])
	r.CRC = binary.LittleEndian.Uint32(b[8:])
	r.TS = binary.LittleEndian.Uint32(b[12:])
	copy(r.ID[:], b[16:])
	copy(r.Meta[:], b[24:])
	return r
}

func (w *WAL) drainPayload(r Rec, at string) error {
	payN := int64(r.Len) - recHdrN
	var crc uint32
	h := crc32.NewIEEE()
	buf := make([]byte, 4096)

	for payN > 0 {
		want := int64(len(buf))
		if want > payN {
			want = payN
		}
		if _, err := io.ReadFull(w.f, buf[:want]); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return e.New("", e.Prov, at, "corrupt_payload_trunc")
			}
			return e.Wrap("", e.Trans, at, err)
		}
		if _, err := h.Write(buf[:want]); err != nil {
			return e.Wrap("", e.Trans, at, err)
		}
		payN -= want
	}

	crc = h.Sum32()
	if crc != r.CRC {
		return e.New("", e.Prov, at, "corrupt_crc")
	}
	return nil
}

func maxInt() int {
	return int(^uint(0) >> 1)
}
