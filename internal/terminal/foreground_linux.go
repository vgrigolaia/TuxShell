package terminal

import (
	"bytes"
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

// foregroundArgs returns the command line of the terminal's foreground
// process group leader, or nil. In the Flatpak the shell runs on the host,
// outside the sandbox's process list, so this finds nothing there.
func foregroundArgs(ptmx *os.File) []string {
	pgid := foregroundGroup(ptmx)
	if pgid <= 0 {
		return nil
	}
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pgid) + "/cmdline")
	if err != nil || len(b) == 0 {
		return nil
	}
	var argv []string
	for _, a := range bytes.Split(bytes.TrimRight(b, "\x00"), []byte{0}) {
		argv = append(argv, string(a))
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
