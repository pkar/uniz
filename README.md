# uniz

Unix-style utilities written in Go, usable from the command line or through a single library import. It implements 88 commands; run `uniz --help` for the list and `uniz <command> --help` for each command's options. [COMMANDS.md](COMMANDS.md) lists what is implemented and what is planned. No external commands or third-party dependencies are required.

## Install a release

Each tagged release on [GitHub](https://github.com/pkar/uniz/releases) ships `uniz-linux-amd64`, `uniz-linux-arm64`, `uniz-darwin-arm64`, and `uniz-darwin-amd64` along with a `checksums.txt` of their SHA-256 sums. `install.sh` downloads the binary for your platform from the latest release, checks it against that release's `checksums.txt`, and installs it into `~/.local/bin` (set `UNIZ_INSTALL_DIR` to change this). If there is no prebuilt binary for your platform and Go is installed, it builds the same tag from source. It needs `curl`, not sudo. The checksums catch corrupted downloads, but they aren't a signature: they come from the same place as the binaries.

```sh
curl -fsSL -o install.sh https://raw.githubusercontent.com/pkar/uniz/main/install.sh
less install.sh
sh install.sh
```

Or use the Go toolchain: `go install github.com/pkar/uniz/cmd/uniz@latest` (this sets `uniz --version` to `dev`).

To cut a release, push a `v*` tag, e.g. `git tag v0.1.0 && git push origin v0.1.0`. The release workflow runs the tests on Linux and macOS, cross-builds the four binaries with `CGO_ENABLED=0`, and publishes them with generated notes.

## Build and run

Requires Go 1.26 or newer.

```sh
make build
./bin/uniz ls
./bin/uniz ls -al .
./bin/uniz ls -tr README.md ls
./bin/uniz ls -- -filename
make install                     # installs to ~/.local/bin
make install BINDIR=/your/bin    # choose a different directory
```

Add the installation directory to your `PATH`. `PREFIX` selects `PREFIX/bin`; `BINDIR` takes precedence. `DESTDIR` supports staged installation. Use `make build VERSION=0.1.0` to set `uniz --version` (default: `dev`). Run `make` for target help and `make check` for formatting checks, vet, and tests.

## `uniz ls`

```
uniz ls [-aAdhlrt1] [--] [paths...]
```

| Flag | Behavior |
| --- | --- |
| `-a` | Include hidden entries, including `.` and `..` |
| `-A` | Include hidden entries, except `.` and `..` |
| `-l` | Show mode, byte size, local modification time, and symlink target |
| `-h` | With `-l`, show sizes like `1.1K` and `15M` (powers of 1024, rounded up) |
| `-d` | List directories themselves rather than their contents |
| `-r` | Reverse the selected order |
| `-t` | Sort newest first, breaking timestamp ties by name |
| `-1` | One entry per line (already the default) |
| `--` | Treat all following arguments as paths |

Short flags can be combined and placed before or after paths. With no paths, `ls` lists the current directory. Explicit hidden files are always listed. `-a` takes precedence over `-A`. Multiple operands are handled in argument order; directory listings have headers. Filesystem errors are reported on stderr while remaining operands are processed.

This is a small, portable subset, not a drop-in GNU/BSD `ls` replacement. Names sort lexically, not by locale. Symlinks, including directory symlinks, are not followed. Long listings omit owner, group, link count, and block totals. There are no recursive listings, terminal columns, or colors yet. Names containing control characters or invalid UTF-8 are Go-quoted for terminal safety; output is intended for people, not machine parsing.

Exit codes: `0` success, `1` filesystem/output failure, `2` invalid usage. Use `uniz --help` or `uniz ls --help` for help.

## More commands

```sh
uniz cat file.txt another.txt    # concatenate files unchanged
uniz cat file.txt -              # then read stdin
uniz wc file.txt another.txt     # lines, words, bytes, and a total
uniz wc -lwcm file.txt           # include Unicode character count
uniz wc -c                      # count stdin bytes
uniz pwd                        # logical working-directory path
uniz pwd -P                     # resolve symbolic links
```

- **`cat [--] [files...]`** streams bytes unchanged. No operands or `-` reads stdin. Repeated `-` operands read the same stream, not a replay. Numbering and display flags are not yet implemented.
- **`wc [-lwcm] [--] [files...]`** supports newline (`-l`), word (`-w`), byte (`-c`), and Unicode character (`-m`) counts. Default output is lines, words, bytes; selected fields always appear in lines/words/bytes/chars order, separated by single spaces rather than GNU/BSD column padding. Multiple operands add a total. Explicit operands get filename labels; implicit stdin does not. Lines count newline bytes, so an unterminated last line is not counted. Words use Unicode whitespace regardless of locale. Invalid UTF-8 bytes each count as one character. Maximum-line-length and other GNU extensions are not supported.
- **`pwd [-L|-P]`** prints the process working directory; `-L` is the default and preserves a valid logical `PWD`. `-P` resolves symlinks. The last flag wins. It never changes the caller's working directory.

Use `--help` with any command. `cat` and `wc` continue after input errors, return a joined error, and stop on output failure. `wc` includes partial counts from read failures in its output and totals; files that cannot be opened are skipped. Counts use bounded-memory streaming, with no line-size limit. Filenames in `wc` labels are emitted verbatim; do not use them as a machine-readable or terminal-safe format for untrusted names.

## Path commands and directory creation

```sh
uniz basename /some/path/report.txt .txt   # report
uniz dirname /some/path/report.txt        # /some/path
uniz dirname a/b c/d                     # one result per operand
uniz mkdir new-directory
uniz mkdir -p parent/child another/path
uniz mkdir -- -directory-name
```

- **`basename [--] name [suffix]`** strips trailing slashes and directory components, then an optional suffix unless it matches the whole basename. This version accepts a single name; GNU `-a`, `-s`, and `-z` are not implemented.
- **`dirname [--] names...`** prints the directory portion of each name. A name without a directory component yields `.`. NUL-delimited output is not implemented.
- Both path commands use Unix `/` separators regardless of host platform, preserve `.` and `..` components, and do not access the filesystem. All-slash paths yield `/`; exactly two leading slashes receive no special treatment. Empty input yields an empty basename and `.` for dirname. Output is verbatim with newlines, not an escaped or machine-safe format for arbitrary path names.
- **`mkdir [-p] [--] directories...`** creates directories with mode `0777` filtered by the process umask. `-p` creates missing ancestors and accepts existing directories without changing their permissions. Files in place of directories are errors. Failures do not prevent later operands from being attempted. Mode (`-m`) and verbose (`-v`) options are not implemented. Cancellation cannot interrupt a filesystem operation; created directories are not rolled back on failure or cancellation.

## Text and stream commands

```sh
uniz echo -e 'a\tb'              # escapes with -e; -n omits the newline
uniz head -n 5 file              # also -5, -c BYTES; multiple files get headers
uniz tail -n +20 file            # from line 20; -n N, -c N, -c +N
uniz sort -nru numbers.txt       # -f fold case, -n numeric, -r reverse, -u unique
uniz sort words | uniz uniq -c   # uniq: -c count, -d repeated, -u unique, -i case
uniz seq -s , 1 2 9              # 1,3,5,7,9
uniz some-command | uniz tee -a log.txt
uniz yes | uniz head -n 3
uniz sleep 1.5 2m                # sums durations; s, m, h, d suffixes
uniz printenv HOME; uniz whoami; uniz hostname; uniz true; uniz false
```

- Options are parsed before any work starts, so `uniz touch new -c other` applies `-c` to both names. Use `--` before operands that start with `-`.
- `echo`, `true`, `false`, and `yes` treat all arguments as text, apart from `echo`'s leading `-n`/`-e`/`-E` options. `--help` works only as the sole argument.
- `false`, and `printenv` with an unset name, exit with status 1 and print no message.
- `sort` and `tail` read their whole input before writing. If `sort` can't read an input, it writes nothing.
- `head`, `tail`, `uniq`, `sort`, and `wc` keep going after an input fails to open, then exit 1. A failure writing to stdout stops at once.
- Not supported yet: `sort -k/-t/-o`, `uniq` skip options and output files, `tail -f`, negative `head` counts, and floating-point `seq`.

## File commands

```sh
uniz touch notes.txt             # -c: don't create missing files
uniz cp -r src backup            # -p keeps modes and times; symlinks copied as links
uniz cp a b existing-dir/        # several sources need an existing directory
uniz mv old new                  # falls back to copy+remove across filesystems
uniz ln -s target link           # -f replaces atomically, -n treats a dir symlink as a file
uniz rm -r build                 # -f ignores missing paths, -d removes empty dirs
uniz rmdir -p a/b/c              # removes c, then b, then a
uniz realpath link; uniz readlink link; uniz readlink -f link
```

Safety rules:

- `rm` never follows symlinks, even with a trailing slash: `rm -r link/` removes the link and leaves the target alone. It refuses `.`, `..`, and `/`. It never prompts, and `-i` isn't implemented.
- `cp` and `mv` refuse to copy a file onto itself (including through a hard link) and refuse to put a directory inside itself. `cp` overwrites existing files without asking.
- `cp` without `-r` follows a symlinked source. `cp -r` and `mv` copy symlinks as links.
- If a cross-filesystem `mv` fails partway, the source is kept and partial output may remain at the destination.
- Only regular files, directories, and symlinks can be copied. Ownership, ACLs, and extended attributes are not copied.

## Text transforms, checksums, and formatting

```sh
uniz cut -d, -f1,3 data.csv      # also -b and -c (UTF-8) lists like 2-4,7-
uniz tr -s 'a-z' 'A-Z'           # -d delete, -c complement, [:classes:]
uniz paste -d, names ages        # -s serial; paste - - pairs stdin lines
uniz nl -ba file; uniz tac file; uniz rev file; uniz fold -s -w 60 file
uniz sha256sum *.tar > SUMS && uniz sha256sum -c SUMS
uniz md5sum, sha1sum, sha224sum, sha384sum, sha512sum, cksum
uniz base64 -w0 file; uniz base64 -d encoded
uniz printf '%-10s %5.2f\n' apples 1.5 pears 2
uniz date -u '+%Y-%m-%dT%H:%M:%SZ'; uniz date -d @0; uniz date -r file
uniz test -f go.mod && echo yes; uniz [ 3 -lt 10 ]
```

- `tr` and `cut -c` work on Unicode characters, but `tr`'s character classes are ASCII only. `tr` does not support `[c*n]` or `[=c=]`.
- `printf` reuses the format while arguments remain. An invalid number prints as `0`, and the command fails after printing everything.
- `date` can't set the clock. Names are English and there are no locales.
- `test` and `[` exit 1 for false and 2 for a syntax error, and print nothing for false.

## Search, files, system, and processes

```sh
uniz grep -rn 'TODO' src         # -E, -F, -i, -v, -w, -x, -c, -l, -L, -o, -q, -m N
uniz find . -name '*.go' -type f -newer go.mod
uniz find . -name .git -prune -o -type f -print0 | uniz xargs -0 uniz wc -l
uniz chmod -R go-w,a+rX dir; uniz chmod 0640 secret
uniz mktemp -d; uniz truncate -s 1M file; uniz link a b; uniz unlink b
uniz nproc; uniz uname -a; uniz id; uniz groups
uniz env -i PATH=/usr/bin:/bin sh -c 'env'
uniz timeout -k 5 30 long-job
```

- `grep` uses Go's RE2 engine. Matching is leftmost-longest, as in POSIX, but back-references (`\1`) are rejected and context options (`-A`, `-B`, `-C`) aren't supported. Exit status: 0 if a line matched, 1 if none did, 2 on an error.
- `find` never follows symlinks and visits entries in name order. It has no `-exec` or `-delete`; pipe `-print0` into `xargs -0`.
- `chmod -R` skips symlinks below the operand, so a link can't redirect a change outside the tree. Symbolic modes ignore the umask.
- `env`, `xargs`, and `timeout` run external programs. The child's stderr goes to `Config.Stderr`, and the exit statuses follow GNU (126, 127, 123–125, 124). `xargs` with no program uses the built-in `echo`. `timeout` sends SIGTERM, adds SIGKILL after `-k`, and kills the child outright if the caller's context is canceled.
- `id` and `groups` read the user database. They don't distinguish real from effective IDs.

## Paragraphs, tables, splitting, and dumps

```sh
uniz fmt -w 60 notes.txt; uniz expand -t 4 code.c; uniz unexpand -a file
uniz comm -12 <(uniz sort a) <(uniz sort b)    # lines in both
uniz join -t , -a 1 -o 0,1.2,2.3 -e NA left.csv right.csv
uniz split -b 10M -d big.iso part.; uniz csplit -f ch book.txt '/^Chapter/' '{*}'
uniz od -A x -t x1 -t c file | uniz head; uniz base32 file; uniz base32 -d enc
uniz shuf -n 3 names.txt; uniz shuf -i 1-100 -n 1; uniz tsort deps.txt
```

- `fmt` joins words with single spaces and treats an indentation change after the second line as a paragraph break. It doesn't do GNU's optimal line breaking or sentence spacing.
- `join` and `csplit` hold their inputs in memory. `comm` and `join` compare bytewise and don't check that inputs are sorted.
- `split` and `csplit` overwrite existing regular files but refuse to write through a symbolic link. `csplit` removes the pieces it wrote if a pattern fails, unless `-k` is given.
- `od` reads integers in the machine's byte order and prints `*` for repeated lines unless `-v` is given.
- `tsort` still prints every item when it finds a cycle, then reports the loop and exits 1.

## File metadata, space, and process control

```sh
uniz stat file; uniz stat -c '%n %s %a %U %y' *.go
uniz du -sh dir; uniz du -a -d 1 .; uniz df -h .
uniz install -m 0755 -D bin/tool ~/.local/bin/tool
uniz shred -u -z secret.txt; uniz sync; uniz mkfifo -m 600 pipe
uniz expr 3 '*' 7; uniz expr "$file" : '\(.*\)\.go'
uniz tty; uniz logname; uniz pathchk -p some/name
uniz nice -n 10 make; uniz nohup long-job &; uniz kill -TERM 1234; uniz kill -l
uniz time -p go build ./...
```

- `stat` follows GNU's `-c` directives (`%n %N %s %b %B %o %f %F %a %A %u %U %g %G %h %i %d`, the time letters `%x %y %z %w` and their epoch forms `%X %Y %Z %W`). Fields a platform lacks print `?`. There's no `--printf`, `-f`, or `-t`.
- `du` doesn't follow symlinks and counts hard-linked files once. It reports 1024-byte blocks by default; `-h` and `-b` are the alternatives.
- `df` reads `getfsstat` on macOS and `/proc/self/mounts` on Linux. It's unsupported on Windows, as are `mkfifo` and `sync` with no operands.
- `install` writes to a temporary file next to the destination and renames it into place, so a symlink at the destination is replaced rather than followed. It doesn't support `-o`, `-g`, or `-s`.
- `shred` refuses symbolic links, non-regular files, `.`, `..`, and `/`. On journaling or copy-on-write filesystems (APFS, btrfs, ZFS) and on SSDs, old copies of the data may survive.
- `expr` exits 0 for a non-null result, 1 for empty or `0`, and 2 on errors. Arithmetic is 64-bit and fails on overflow.
- `tty` works only when uniz runs with a real stdin file. Through the library, it reports "not a tty" unless `Config.Stdin` is an `*os.File` terminal. `logname` reads `$LOGNAME`.
- `nice` changes the child's niceness just after it starts. If lowering niceness isn't permitted, the program runs at the current niceness without a warning. `nohup` starts the program in a new session rather than ignoring SIGHUP, and redirects terminal output to `nohup.out`. `time` writes POSIX `real`/`user`/`sys` lines to `Config.Stderr`.
- `kill` on Windows supports only `KILL`/`TERM` (both terminate outright) and signal `0`.

## Library

The module path is `github.com/pkar/uniz`; creating this local repository does not publish it there. Until it is published, consumers can use a local Go workspace or a `replace` directive pointing to this checkout.

Import **only `github.com/pkar/uniz`**, whether you want command dispatch or typed operations. Both execute inside your binary; no separate executable is needed.

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/pkar/uniz"
)

func main() {
    ctx := context.Background()
    runner := uniz.New(uniz.Config{Stdout: os.Stdout})
    if err := runner.Run(ctx, []string{"ls", "-A", "."}); err != nil {
        log.Fatal(err)
    }

    entries, err := uniz.List(ctx, ".", uniz.ListOptions{SortTime: true})
    for _, entry := range entries {
        log.Printf("%s: %d bytes", entry.Name, entry.Info.Size())
    }
    // Partial results may accompany an error.
    if err != nil {
        log.Fatal(err)
    }
}
```

`Runner.Run` returns errors without printing diagnostics or exiting. Use `errors.Is`/`errors.As` to inspect filesystem errors, cancellation, or `*uniz.UsageError`. `uniz.ExitCode(err)` maps an error to a CLI exit code. The standalone executable prints the returned error once and exits with that code.

Nil streams mean empty input and discarded output, not implicit process-global streams. `cat` and `wc` use `Config.Stdin` when no files are given or an operand is `-`. `Config.Stderr` receives only the stderr of programs started by `env`, `xargs`, `timeout`, `nice`, `nohup`, and `time`, plus the timings `time` writes; uniz's own errors are returned, not printed there. A zero-value `Runner` is usable. Concurrent calls require concurrency-safe streams. Pass a non-nil context; cancellation is cooperative between operations and cannot interrupt an in-flight filesystem call or stream write.

`uniz.List` returns `[]uniz.Entry` with lstat-style `Info`, a `Path`, and a `LinkTarget` for symbolic links. `ListOptions.Long` affects command rendering only, not structured results.

Additional typed operations use the same import:

```go
n, err := uniz.Cat(ctx, destinationWriter, sourceReader)
counts, err := uniz.Count(ctx, sourceReader)
// counts.Lines, counts.Words, counts.Bytes, counts.Chars
path, err := uniz.WorkingDirectory(ctx, uniz.DirectoryOptions{Physical: true})
```

These snippets illustrate separate calls; each error should be handled. `Cat` and `Count` accept non-nil streams, leave them open, and return partial progress on failure. They check cancellation between reads/writes but cannot interrupt a blocked stream operation. Filesystem commands use the process working directory; concurrent callers should not change it.

Path and creation APIs are also available through the root package:

```go
name := uniz.BaseName("/some/path/report.txt", ".txt") // report
parent := uniz.DirName("/some/path/report.txt")       // /some/path
err := uniz.MakeDirectory(ctx, "parent/child", uniz.MakeDirectoryOptions{
    Parents: true,
})
```

Every command also has a typed function in the root package. `Head`, `Tail`, `Sort`, `SortLines`, `ReadLines`, `Uniq`, `Seq`, `Sequence` (an `iter.Seq[int64]`), and `Yes` work on readers and writers. `Touch`, `Remove`, `RemoveDirectory`, `Copy`, `Move`, `Link`, `RealPath`, and `ReadLink` work on paths:

```go
err := uniz.Copy(ctx, "src", "dst", uniz.CopyOptions{Recursive: true, Preserve: true})
if errors.Is(err, uniz.ErrIntoItself) { /* ... */ }
sorted := uniz.SortLines(lines, uniz.SortOptions{Numeric: true, Reverse: true})
for n := range uniz.Sequence(1, 2, 9) { fmt.Println(n) }
```

The typed file functions take an exact destination. Unlike the `cp`, `mv`, and `ln` commands, they never put a source inside an existing directory. A command that exits with a specific status returns `*uniz.ExitError`, and `uniz.ExitCode` maps it to that status. When its `Err` field is nil (`false`, `test`, `grep` without a match), there is nothing to print.

The newer commands have typed functions too: `Cut`, `Translate`, `Paste`, `NumberLines`, `ReverseLines`, `ReverseCharacters`, `Fold`, `Checksum`, `CRC`, `Base64Encode`, `Base64Decode`, `Printf`, `Strftime`, `ParseDate`, `Test`, `CompilePattern` with `Grep`, `FindPaths`, `ParseMode` with `Chmod`, `MakeTemp`, `ParseSize` with `Truncate`, `Unlink`, and `Uname`:

```go
sum, err := uniz.Checksum(ctx, uniz.SHA256, file)
m, err := uniz.CompilePattern([]string{`^func `}, uniz.PatternOptions{Syntax: uniz.ExtendedRegexp})
n, err := uniz.Grep(ctx, os.Stdout, file, m, uniz.GrepOptions{LineNumbers: true})
paths, err := uniz.FindPaths(ctx, []string{"."}, []string{"-name", "*.go", "-type", "f"})
change, err := uniz.ParseMode("go-w")
err = uniz.Chmod(ctx, "dir", change, uniz.ChmodOptions{Recursive: true})
```

The root `uniz` package is the public API for the Unix command suite. Implementations live under `internal/` (for example `internal/text`, `internal/search`, and `internal/fileops`); argument handling lives in `internal/cli`. `cmd/uniz` uses the same public API as an embedding application. Future commands and typed operations will be added through this single import.
