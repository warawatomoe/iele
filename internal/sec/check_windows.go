//go:build windows

package sec

import (
	"os"
	"runtime"
	"syscall"
	"unsafe"

	e "iele/internal/err"
)

const (
	seFileObject = 1

	ownerSecurityInformation = 0x00000001
	daclSecurityInformation  = 0x00000004

	tokenQuery = 0x0008
	tokenUser  = 1

	accessAllowedAceType = 0

	winLocalSystemSid = 18

	sensitiveAccessMask = 0x00000001 |
		0x00000002 |
		0x00000004 |
		0x00000020 |
		0x00020000 |
		0x00040000 |
		0x00080000 |
		0x00100000 |
		0x001F01FF |
		0x40000000 |
		0x80000000
)

var (
	advapi32 = syscall.NewLazyDLL("advapi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procGetNamedSecurityInfoW = advapi32.NewProc("GetNamedSecurityInfoW")
	procGetAce                = advapi32.NewProc("GetAce")
	procEqualSid              = advapi32.NewProc("EqualSid")
	procCreateWellKnownSid    = advapi32.NewProc("CreateWellKnownSid")
	procOpenProcessToken      = advapi32.NewProc("OpenProcessToken")
	procGetTokenInformation   = advapi32.NewProc("GetTokenInformation")
	procGetCurrentProcess     = kernel32.NewProc("GetCurrentProcess")
)

type aclHeader struct {
	AclRevision uint8
	Sbz1        uint8
	AclSize     uint16
	AceCount    uint16
	Sbz2        uint16
}

type aceHeader struct {
	AceType  uint8
	AceFlags uint8
	AceSize  uint16
}

func check(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return e.Wrap("", e.Trans, "sec:stat", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return e.New("", e.Prov, "sec:perm", "symlink")
	}
	if !info.Mode().IsRegular() {
		return e.New("", e.Prov, "sec:perm", "not_regular")
	}

	sd, owner, dacl, err := queryFileSecurity(path)
	if err != nil {
		return e.Wrap("", e.Trans, "sec:perm", err)
	}
	defer syscall.LocalFree(sd)

	if dacl == nil {
		return e.New("", e.Prov, "sec:perm", "null_dacl")
	}

	me, meBuf, err := currentUserSID()
	if err != nil {
		return e.Wrap("", e.Trans, "sec:perm", err)
	}

	if !equalSID(owner, me) {
		return e.New("", e.Prov, "sec:perm", "not_owner")
	}

	system, sysBuf, err := localSystemSID()
	if err != nil {
		return e.Wrap("", e.Trans, "sec:perm", err)
	}

	err = checkDACL(dacl, owner, system)
	runtime.KeepAlive(meBuf)
	runtime.KeepAlive(sysBuf)
	return err
}

func queryFileSecurity(path string) (sd syscall.Handle, owner *syscall.SID, dacl unsafe.Pointer, err error) {
	pPath, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, nil, nil, err
	}

	var pSD uintptr
	var pOwner uintptr
	var pDacl uintptr

	r, _, errno := procGetNamedSecurityInfoW.Call(
		uintptr(unsafe.Pointer(pPath)),
		seFileObject,
		ownerSecurityInformation|daclSecurityInformation,
		uintptr(unsafe.Pointer(&pOwner)),
		0,
		uintptr(unsafe.Pointer(&pDacl)),
		0,
		uintptr(unsafe.Pointer(&pSD)),
	)
	if r != 0 {
		if r <= 0xffff {
			return 0, nil, nil, syscall.Errno(r)
		}
		return 0, nil, nil, errno
	}

	return syscall.Handle(pSD),
		(*syscall.SID)(unsafe.Pointer(pOwner)),
		unsafe.Pointer(pDacl),
		nil
}

func currentUserSID() (*syscall.SID, []byte, error) {
	proc, _, _ := procGetCurrentProcess.Call()

	var token syscall.Handle
	r, _, errno := procOpenProcessToken.Call(proc, tokenQuery, uintptr(unsafe.Pointer(&token)))
	if r == 0 {
		return nil, nil, errno
	}
	defer syscall.CloseHandle(token)

	var needed uint32
	procGetTokenInformation.Call(
		uintptr(token), tokenUser, 0, 0,
		uintptr(unsafe.Pointer(&needed)),
	)
	if needed == 0 {
		return nil, nil, e.New("", e.Trans, "sec:perm", "token_size_zero")
	}

	buf := make([]byte, needed)
	r, _, errno = procGetTokenInformation.Call(
		uintptr(token),
		tokenUser,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(needed),
		uintptr(unsafe.Pointer(&needed)),
	)
	if r == 0 {
		return nil, nil, errno
	}

	sidPtr := *(*uintptr)(unsafe.Pointer(&buf[0]))
	return (*syscall.SID)(unsafe.Pointer(sidPtr)), buf, nil
}

func localSystemSID() (*syscall.SID, []byte, error) {
	buf := make([]byte, 68)
	sid := (*syscall.SID)(unsafe.Pointer(&buf[0]))
	size := uint32(len(buf))

	r, _, errno := procCreateWellKnownSid.Call(
		winLocalSystemSid, 0,
		uintptr(unsafe.Pointer(sid)),
		uintptr(unsafe.Pointer(&size)),
	)
	if r == 0 {
		return nil, nil, errno
	}
	return sid, buf, nil
}

func equalSID(a, b *syscall.SID) bool {
	r, _, _ := procEqualSid.Call(
		uintptr(unsafe.Pointer(a)),
		uintptr(unsafe.Pointer(b)),
	)
	return r != 0
}

// Rejects ACEs granting sensitive access to principals other than owner or SYSTEM.
func checkDACL(dacl unsafe.Pointer, owner, system *syscall.SID) error {
	hdr := (*aclHeader)(dacl)
	if hdr.AceCount == 0 {
		return e.New("", e.Prov, "sec:perm", "empty_dacl")
	}

	for i := uint16(0); i < hdr.AceCount; i++ {
		var acePtr uintptr
		r, _, errno := procGetAce.Call(
			uintptr(dacl),
			uintptr(i),
			uintptr(unsafe.Pointer(&acePtr)),
		)
		if r == 0 {
			return e.Wrap("", e.Trans, "sec:perm", errno)
		}

		hdrACE := (*aceHeader)(unsafe.Pointer(acePtr))
		if hdrACE.AceType != accessAllowedAceType {
			continue
		}

		mask := *(*uint32)(unsafe.Pointer(acePtr + unsafe.Sizeof(aceHeader{})))
		if mask&sensitiveAccessMask == 0 {
			continue
		}

		aceSID := (*syscall.SID)(unsafe.Pointer(acePtr + 8))
		if equalSID(aceSID, owner) || equalSID(aceSID, system) {
			continue
		}

		return e.New("", e.Prov, "sec:perm", "extra_access")
	}
	return nil
}

