package askpass

import (
	"syscall"
	"unsafe"
)

func uintptrOf(t *syscall.Termios) uintptr { return uintptr(unsafe.Pointer(t)) }
