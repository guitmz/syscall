#!/usr/bin/env python3
"""Collect license files for every module in the current Go build target.

Run with the same GOOS, GOARCH and CGO_ENABLED as the build.
"""

import pathlib
import subprocess
import sys


def go(*args):
    return subprocess.check_output(["go", *args], text=True).strip()


def main():
    if len(sys.argv) != 2:
        sys.exit("usage: collect-licenses.py OUTPUT")
    template = "{{if and .Module (not .Module.Main)}}{{.Module.Path}}\t{{.Module.Version}}\t{{.Module.Dir}}{{end}}"
    modules = {}
    for line in go("list", "-deps", "-f", template, ".").splitlines():
        if line:
            path, version, directory = line.split("\t", 2)
            modules[(path, version)] = [pathlib.Path(directory)]
    goroot = pathlib.Path(go("env", "GOROOT"))
    # some distros put the Go license one level above GOROOT
    if not (goroot / "LICENSE").is_file():
        goroot = goroot.parent
    modules[("Go toolchain", go("env", "GOVERSION"))] = [goroot]

    output = ["Third party licenses for the Go build target.\n"]
    for (module, version), dirs in sorted(modules.items()):
        files = sorted(
            f
            for d in dirs
            for pattern in ("LICENSE*", "COPYING*", "NOTICE*", "PATENTS*")
            for f in d.glob(pattern)
            if f.is_file()
        )
        if not any(f.name.startswith(("LICENSE", "COPYING")) for f in files):
            sys.exit(f"no license file for {module} {version} in {dirs}")
        for f in files:
            output.append(
                f"\n===== {module} {version}: {f.name} =====\n{f.read_text(encoding='utf-8')}\n"
            )
    pathlib.Path(sys.argv[1]).write_text("".join(output), encoding="utf-8")


if __name__ == "__main__":
    main()
