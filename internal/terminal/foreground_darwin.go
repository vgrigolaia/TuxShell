package terminal

import (
	"bytes"
	"encoding/binary"
	"os"

	"golang.org/x/sys/unix"
)

// foregroundArgs returns the command line of the terminal's foreground
// process group leader, or nil.
func foregroundArgs(ptmx *os.File) []string {
	pgid := foregroundGroup(ptmx)
	if pgid <= 0 {
		return nil
	}
	// kern.procargs2: argc (int32), the executable path, NUL padding, then argv.
	b, err := unix.SysctlRaw("kern.procargs2", pgid)
	if err != nil || len(b) < 4 {
		return nil
	}
	argc := int(binary.LittleEndian.Uint32(b))
	rest := b[4:]
	i := bytes.IndexByte(rest, 0)
	if i < 0 {
		return nil
	}
	rest = bytes.TrimLeft(rest[i:], "\x00")
	var argv []string
	for len(argv) < argc && len(rest) > 0 {
		j := bytes.IndexByte(rest, 0)
		if j < 0 {
			j = len(rest)
		}
		argv = append(argv, string(rest[:j]))
		rest = rest[min(j+1, len(rest)):]
	}
	return argv
}

// foregroundGroup asks the PTY for its foreground process group. It goes
// through SyscallConn, as Fd() would switch the PTY to blocking mode.
func foregroundGroup(ptmx *os.File) int {
	rc, err := ptmx.SyscallConn()
	if err != nil {
		return 0
	}
	pgid := 0
	_ = rc.Control(func(fd uintptr) {
		pgid, err = unix.IoctlGetInt(int(fd), unix.TIOCGPGRP)
	})
	if err != nil {
		return 0
	}
	return pgid
}
