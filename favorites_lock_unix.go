//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package main

import (
	"os"
	stdsyscall "syscall"
)

func lockFavorites(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := stdsyscall.Flock(int(f.Fd()), stdsyscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() { _ = stdsyscall.Flock(int(f.Fd()), stdsyscall.LOCK_UN); _ = f.Close() }, nil
}
