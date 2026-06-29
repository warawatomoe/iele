package turn

import (
	"time"

	e "iele/internal/err"
	"iele/internal/wal"
)

const (
	TxBegin    uint16 = 1
	TypeSys    uint16 = 2
	TypeUser   uint16 = 3
	TypeAsst   uint16 = 4
	TypeTCCall uint16 = 5
	TypeTCRes  uint16 = 6
	TxCommit   uint16 = 7
	TypeDev    uint16 = 8
)

type Role string

const (
	RoleSys  Role = "system"
	RoleDev  Role = "developer"
	RoleUser Role = "user"
	RoleAsst Role = "assistant"
)

type Msg struct {
	Role Role
	Text string
}

func Append(w *wal.WAL, typ uint16, id *[8]byte, text string) error {
	_, err := w.Write(&wal.Append{
		Type:    typ,
		TS:      uint32(time.Now().Unix()),
		ID:      id,
		Payload: []byte(text),
	})
	if err != nil {
		return e.Wrap("", e.Trans, "turn:append", err)
	}
	return nil
}

// Project scans the WAL returning only sys/dev/user/asst messages.
// Tool calls, tool results, and transaction markers are skipped.
func Project(w *wal.WAL) ([]Msg, error) {
	var msgs []Msg
	err := w.Scan(func(r wal.Rec, payload []byte) error {
		var role Role
		switch r.Type {
		case TypeSys:
			role = RoleSys
		case TypeDev:
			role = RoleDev
		case TypeUser:
			role = RoleUser
		case TypeAsst:
			role = RoleAsst
		default:
			return nil
		}
		msgs = append(msgs, Msg{Role: role, Text: string(payload)})
		return nil
	})
	if err != nil {
		return nil, e.Wrap("", e.Trans, "turn:project", err)
	}
	return msgs, nil
}
