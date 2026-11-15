#!/usr/bin/env python3
"""Download the API dataset, check it against the manifest and write it to dataset/ for go:embed."""

import gzip
import hashlib
import json
import os
import pathlib
import re
import sys
import urllib.parse
import urllib.request

DEFAULT_URL = "https://api.syscall.sh/v2/dataset/syscalls.json"
MAX_SIZE = 32 << 20
VERSION = re.compile(r"[0-9]+\.[0-9]+")


def fetch(url, limit):
    req = urllib.request.Request(
        url,
        headers={
            "Accept": "application/json",
            "User-Agent": "syscall-build",
            "Cache-Control": "no-cache",
        },
    )
    with urllib.request.urlopen(req, timeout=30) as resp:
        data = resp.read(limit + 1)
    if len(data) > limit:
        raise ValueError(f"{url} is bigger than {limit} bytes")
    return data


def manifest(url):
    manifest_url = url.rsplit("/", 1)[0]
    info = json.loads(fetch(manifest_url, 64 << 10)).get("json") or {}
    path, size, digest = info.get("url"), info.get("size_bytes"), info.get("sha256")
    if (
        not isinstance(path, str)
        or type(size) is not int
        or not 0 < size <= MAX_SIZE
        or not re.fullmatch(r"[0-9a-f]{64}", str(digest))
    ):
        raise ValueError("bad JSON metadata in the dataset manifest")
    source = urllib.parse.urljoin(manifest_url + "/", path)
    if urllib.parse.urlsplit(source)[:2] != urllib.parse.urlsplit(url)[:2]:
        raise ValueError("manifest points the dataset at another host")
    return source, size, digest


def validate(raw):
    data = json.loads(raw)
    required = {
        "kernels": ("version", "arch"),
        "conventions": ("arch",),
        "syscalls": ("arch", "name", "since", "nr"),
    }
    for group, fields in required.items():
        rows = data.get(group)
        if not isinstance(rows, list) or not rows:
            raise ValueError(f"dataset has no {group}")
        for row in rows:
            for field in fields:
                value = row.get(field)
                ok = (
                    type(value) is int
                    if field == "nr"
                    else isinstance(value, str) and value
                )
                if not ok:
                    raise ValueError(f"bad {field} in {group}")
            nargs = row.get("nargs")
            if nargs is not None and (type(nargs) is not int or not 0 <= nargs <= 6):
                raise ValueError(f"bad nargs in {group}")
            for field in ("version", "since", "removed_in"):
                value = row.get(field)
                if value is not None and not (
                    isinstance(value, str) and VERSION.fullmatch(value)
                ):
                    raise ValueError(f"bad {field} in {group}")


def write(out, name, content):
    tmp = out / (name + ".tmp")
    tmp.write_bytes(content)
    tmp.replace(out / name)


def main():
    url = os.environ.get("SYSCALL_BUILD_DATASET_URL", DEFAULT_URL)
    source, size, digest = manifest(url)
    raw = fetch(source, MAX_SIZE)
    if len(raw) != size or hashlib.sha256(raw).hexdigest() != digest:
        raise ValueError("dataset size or SHA-256 does not match the manifest")
    validate(raw)
    out = pathlib.Path(__file__).resolve().parent.parent / "dataset"
    write(out, "syscalls.json.gz", gzip.compress(raw, mtime=0))
    write(
        out,
        "provenance.json",
        (
            json.dumps({"source_url": source, "dataset_sha256": digest}, indent=2)
            + "\n"
        ).encode(),
    )
    print(f"Prepared dataset from {source} (SHA-256 {digest})")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, AttributeError) as exc:
        sys.exit(f"cannot prepare dataset: {exc}")
