# Command roadmap

The goal is to cover the common POSIX and GNU coreutils commands. Each command
gets an implementation under `internal/`, a CLI entry in
`internal/cli/commands.go` (the registry), a typed function in the root
package, tests that use only the public import, and README notes.

## Implemented

| Area | Commands |
| --- | --- |
| Listing and paths | `ls`, `pwd`, `basename`, `dirname`, `realpath`, `readlink` |
| Files and directories | `mkdir`, `rmdir`, `touch`, `rm`, `cp`, `mv`, `ln` |
| Text and streams | `cat`, `wc`, `head`, `tail`, `sort`, `uniq`, `tee`, `echo`, `seq`, `yes`, `cut`, `tr`, `paste`, `nl`, `tac`, `rev`, `fold`, `base64`, `base32`, `fmt`, `expand`, `unexpand`, `comm`, `join`, `split`, `csplit`, `od`, `shuf`, `tsort` |
| Search | `grep`, `find`, `xargs` |
| Checksums | `cksum`, `md5sum`, `sha1sum`, `sha224sum`, `sha256sum`, `sha384sum`, `sha512sum` |
| Files | `chmod`, `mktemp`, `truncate`, `link`, `unlink`, `stat`, `du`, `df`, `install`, `shred`, `sync`, `mkfifo` |
| Environment and control | `printenv`, `whoami`, `hostname`, `sleep`, `true`, `false`, `env`, `id`, `groups`, `uname`, `nproc`, `date`, `printf`, `test`/`[`, `timeout`, `expr`, `tty`, `logname`, `pathchk` |
| Process control | `nice`, `nohup`, `kill`, `time` |

## Gaps in implemented commands

- `ls`: recursion (`-R`), columns, color, owner and group, `--si`
- `cat`: `-n`, `-b`, `-s`, `-v`
- `sort`: keys (`-k`), separators (`-t`), output file (`-o`), `-c`, `-h`, `-V`
- `uniq`: `-f`, `-s`, `-w`, output file operand
- `head`/`tail`: negative `head` counts, `tail -f`
- `seq`: floating point, `-w`, `-f`
- `cp`: `-i`, `-n`, `-u`, `-v`, ownership and extended attributes
- `rm`: `-i`, `-v`; `mkdir`: `-m`, `-v`; `touch`: `-d`, `-t`, `-r`, `-a`, `-m`
- `basename`: `-a`, `-s`, `-z`
- `grep`: back-references, context (`-A`/`-B`/`-C`), `--include`/`--exclude`, color
- `find`: `-exec`, `-delete`, `-perm`, `-user`, `-L`/`-H`, `-printf`
- `tr`: `[c*n]`, `[=c=]`, non-ASCII classes; `cut`: `--complement`, `-z`
- `date`: setting the clock, locales; `printf`: `%q`, positional arguments
- `chmod`: `-v`, `-c`, `--reference`; `truncate`: `-r`; `mktemp`: `-u`
- `timeout`: `-s SIGNAL`, `--preserve-status`; `xargs`: `-P`, `-L`, `-s`, `-p`
- `*sum`: `--tag`, `--status`, `--quiet`, GNU name escaping; `cksum`: `-a`
- `fmt`: optimal line breaking, `-p` prefixes, `-t`, `-c`, two spaces after sentences
- `join`: `--check-order`, `--header`, `-o auto`; `comm`: order checking, `-z`
- `split`: `-C`, `-l`/`-n` modes such as `l/N` and `r/N`, `--additional-suffix`, `--filter`
- `od`: `-t ...z`, `-S` strings, `--endian`, traditional offset syntax
- `shuf`: `-o`, `--random-source`, `-z`; `base32`: `-i`
- `stat`: `--printf`, `-f` (filesystem status), `-t`
- `du`: `-L`, `-x`, `--exclude`, `-l`; `df`: `-i`, `-t`/`-x` type filters, Windows
- `install`: `-o`, `-g`, `-s`, `-b`, `-C`; `shred`: `-s`, `-x`, `-v`, devices
- `mkfifo`, `sync` without files, and `nice` aren't available on Windows
- `nohup` moves the program to a new session rather than ignoring SIGHUP
- `kill`: Windows supports only `KILL`/`TERM`/`0`; `time`: `-f`/`-v` formats
- `tty` and `logname` don't consult utmp (`getlogin`), since that needs cgo

## Planned

Every command from the original roadmap is implemented. New requests are tracked in the gaps above.

`b2sum` needs BLAKE2, which isn't in the Go standard library, so it stays out
of scope while the project has no third-party dependencies.

Some commands are deliberately out of scope because they depend on platform
services or privileges that a portable standard-library build can't cover well:
`chown`/`chgrp`, `chroot`, `stty`, `who`/`users`/`pinky`, `runcon`/`chcon`, and `stdbuf`.
