//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly && !windows

package main

import "errors"

func lockFavorites(string) (func(), error) {
	return nil, errors.New("saving favorites is not supported on this platform")
}
