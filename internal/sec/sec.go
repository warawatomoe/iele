package sec

import (
	"bufio"
	"os"
	"strings"

	e "iele/internal/err"
)

type Token = string

// File must be owned by current user, mode 0600.
// One token per line, no empty lines.
func Load(path string) ([]Token, error) {
	if err := check(path); err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, e.Wrap("", e.Trans, "sec:open", err)
	}
	defer f.Close()

	var tokens []Token
	seen := make(map[Token]bool)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" {
			return nil, e.New("", e.Prov, "sec:load", "empty_line")
		}
		if seen[line] {
			continue
		}
		seen[line] = true
		tokens = append(tokens, line)
	}
	if err := sc.Err(); err != nil {
		return nil, e.Wrap("", e.Trans, "sec:read", err)
	}
	if len(tokens) == 0 {
		return nil, e.New("", e.Prov, "sec:load", "empty_file")
	}

	return tokens, nil
}

func Hash(data []byte) uint32 {
	h := uint32(2166136261)
	for _, b := range data {
		h ^= uint32(b)
		h *= 16777619
	}
	return h
}

func HashSalt(salt, data []byte) uint32 {
	h := uint32(2166136261)
	for _, b := range salt {
		h ^= uint32(b)
		h *= 16777619
	}
	for _, b := range data {
		h ^= uint32(b)
		h *= 16777619
	}
	return h
}

func Pick(tokens []Token, data []byte) (Token, error) {
	if len(tokens) == 0 {
		return "", e.New("", e.Call, "sec:pick", "empty_tokens")
	}
	h := Hash(data)
	return tokens[h%uint32(len(tokens))], nil
}

func PickSalt(tokens []Token, salt, data []byte) (Token, error) {
	if len(tokens) == 0 {
		return "", e.New("", e.Call, "sec:pick", "empty_tokens")
	}
	h := HashSalt(salt, data)
	return tokens[h%uint32(len(tokens))], nil
}
