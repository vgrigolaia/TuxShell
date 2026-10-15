package askpass

import (
	"net"
	"os"

	"golang.org/x/sys/unix"
)

const (
	ioctlGetTermios = unix.TIOCGETA
	ioctlSetTermios = unix.TIOCSETA
)

// peer returns the connecting process's PID, and false unless it runs as us.
func peer(conn *net.UnixConn) (int, bool) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, false
	}
	var cred *unix.Xucred
	var pid int
	var credErr, pidErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		pid, pidErr = unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID)
	}); err != nil || credErr != nil || pidErr != nil {
		return 0, false
	}
	return pid, int(cred.Uid) == os.Getuid()
}

// parentPID asks the kernel for a process's parent, or 0 if it can't.
func parentPID(pid int) int {
	p, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return 0
	}
	return int(p.Eproc.Ppid)
}
