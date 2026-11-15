package main

import (
	"runtime"
	"strings"
)

// Aliases are only added when they refer to the same syscall table and ABI.
func canonicalArch(arch string) string {
	arch = strings.ToLower(strings.TrimSpace(arch))
	switch arch {
	case "amd64", "x86_64":
		return "x64"
	case "i386", "i486", "i586", "i686", "386":
		return "x86"
	case "aarch64":
		return "arm64"
	case "armv7", "armv7l":
		return "arm"
	case "powerpc64", "ppc64le":
		return "ppc64"
	case "loong64":
		return "loongarch64"
	case "mips64le":
		return "mips64"
	case "mips", "mipsle":
		return "mipso32"
	case "powerpc":
		return "ppc"
	default:
		return arch
	}
}

// hostArch is the arch commands use when none is given: the machine's own on
// Linux, x64 anywhere else.
func hostArch() string {
	if runtime.GOOS != "linux" {
		return "x64"
	}
	return canonicalArch(runtime.GOARCH)
}
