# Data attribution

The binary embeds a snapshot of the dataset behind [syscall.sh](https://syscall.sh) and [api.syscall.sh](https://api.syscall.sh), both by the same author as this CLI. `dataset/provenance.json` has its source and checksum, and `syscall version` prints them.

The data is extracted from the Linux kernel sources. Descriptions include summaries from the [Linux man pages project](https://www.kernel.org/doc/man-pages/), mostly under the Linux man pages copyleft license, and the `io_uring_*` summaries come from the [liburing](https://github.com/axboe/liburing) man pages (LGPL/MIT). Those parts stay under their original licenses.

Release archives also ship `THIRD_PARTY_LICENSES.txt` with the license files of the Go toolchain and every module in that build.
