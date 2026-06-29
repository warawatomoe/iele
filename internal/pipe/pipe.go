package pipe

import (
	"bufio"
	"io"
	"os"

	e "iele/internal/err"
)

func IsPipe() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) == 0
}

// Path takes precedence over piped stdin. "-" reads stdin explicitly.
// Returns nil if neither path nor piped stdin is available.
func Reader(path string) (io.ReadCloser, error) {
	if path == "-" {
		return nopReadCloser{os.Stdin}, nil
	}
	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return nil, e.Wrap("", e.Trans, "pipe:open", err)
		}
		return f, nil
	}
	if IsPipe() {
		return os.Stdin, nil
	}
	return nil, nil
}

type nopReadCloser struct {
	io.Reader
}

func (r nopReadCloser) Close() error {
	return nil
}

type nopWriteCloser struct {
	io.Writer
}

func (w nopWriteCloser) Close() error {
	return nil
}

// Empty path or "-" writes to stdout.
func Writer(path string) (io.WriteCloser, error) {
	if path == "" || path == "-" {
		return nopWriteCloser{os.Stdout}, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, e.Wrap("", e.Trans, "pipe:create", err)
	}
	return f, nil
}

// eol is false on a partial final line (no trailing newline at EOF).
func Lines(r io.Reader, fn func(line []byte, eol bool) error) error {
	br := bufio.NewReaderSize(r, 256*1024)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			eol := len(line) > 0 && line[len(line)-1] == '\n'
			if eol {
				line = line[:len(line)-1]
			}
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			if fe := fn(line, eol); fe != nil {
				return fe
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return e.Wrap("", e.Trans, "pipe:read", err)
		}
	}
}
