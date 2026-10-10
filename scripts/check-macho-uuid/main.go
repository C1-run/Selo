// Command check-macho-uuid fails closed if a Mach-O binary lacks the LC_UUID
// load command.
//
// Newer macOS dyld refuses to load a Mach-O with no LC_UUID:
//
//	dyld: missing LC_UUID load command in <path>
//	signal: abort trap
//
// Go toolchains before 1.24 omit that load command on darwin, so a release built
// with one ships a macOS binary that cannot start on current macOS. Nothing else
// in this repository can catch it: install.sh verifies checksums, a cosign
// signature and SLSA provenance, all of which a non-loadable binary satisfies
// perfectly. That is how v0.4.1, v0.5.0 and v0.5.1 all shipped darwin binaries
// that newer macOS rejects.
//
// Usage:
//
//	check-macho-uuid <binary> [<binary> ...]
//
// Non-Mach-O inputs (Linux ELF) are reported and skipped. The exit status is 0
// only when every Mach-O input was checked and carries LC_UUID: 1 if any does
// not, 2 on a usage error or if no Mach-O was seen at all, so a glob that
// matched nothing cannot pass silently. A missing or unreadable input is a
// failure (exit 1) rather than a usage error: the release must not proceed on an
// absent artifact.
//
// It is a Go command rather than a script so that `go vet ./...`, `go build ./...`
// and the gofmt check in CI cover it, and so the release gate does not depend on
// an interpreter being installed on the runner.
//
// debug/macho parses the container (thin or fat, 32- or 64-bit, either byte
// order) but has no typed value for LC_UUID: the command falls through its
// parser's default branch and is returned as an uninterpreted LoadBytes, so it
// is matched on the raw command word instead.
package main

import (
	"debug/macho"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// uuidLoadCmd is LC_UUID. debug/macho exposes no constant or type for it.
const uuidLoadCmd = 0x1b

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is main's body, separated so the exit-code contract can be tested without
// spawning a process. It returns 0 when every Mach-O input carries LC_UUID, 1
// when any does not (or any input could not be read), and 2 when no Mach-O was
// seen at all, so a glob that matched nothing fails instead of passing.
func run(paths []string, stdout, stderr io.Writer) int {
	if len(paths) == 0 {
		fmt.Fprintln(stderr, "usage: check-macho-uuid <mach-o binary> [more binaries...]")
		return 2
	}

	var checked, failures int
	for _, path := range paths {
		has, isMachO, err := checkUUID(path)
		switch {
		case err != nil:
			fmt.Fprintf(stdout, "FAIL  %s: %v\n", path, err)
			checked++
			failures++
		case !isMachO:
			fmt.Fprintf(stdout, "skip  %s (not a Mach-O)\n", path)
		case has:
			fmt.Fprintf(stdout, "ok    %s carries LC_UUID\n", path)
			checked++
		default:
			fmt.Fprintf(stdout, "FAIL  %s has no LC_UUID: newer macOS dyld aborts on load\n", path)
			checked++
			failures++
		}
	}

	if checked == 0 {
		fmt.Fprintln(stderr, "error: no Mach-O binary was checked")
		return 2
	}
	if failures > 0 {
		fmt.Fprintf(stderr, "\n%d of %d Mach-O binaries are not loadable\n", failures, checked)
		return 1
	}
	return 0
}

// checkUUID reports whether the Mach-O at path carries an LC_UUID load command.
// isMachO is false when the file is demonstrably not Mach-O (a Linux ELF), which
// the caller skips; err is non-nil when the file claims to be Mach-O but could
// not be parsed, or could not be read at all, so a corrupt or absent artifact is
// a failure rather than a silent skip.
func checkUUID(path string) (has, isMachO bool, err error) {
	if f, openErr := macho.Open(path); openErr == nil {
		defer f.Close()
		return fileHasUUID(f), true, nil
	}

	if fat, fatErr := macho.OpenFat(path); fatErr == nil {
		defer fat.Close()
		for _, arch := range fat.Arches {
			if !fileHasUUID(arch.File) {
				return false, true, nil
			}
		}
		return true, true, nil
	}

	magic, magicErr := readMagic(path)
	if magicErr != nil {
		// Unreadable, or too short to hold a magic number. We cannot classify
		// it, so we must not call it "not a Mach-O" and move on: a missing or
		// renamed release artifact would then pass the gate untouched.
		return false, false, fmt.Errorf("cannot be read: %w", magicErr)
	}
	if isMachOMagic(magic) {
		return false, true, fmt.Errorf("has a Mach-O magic number but did not parse")
	}
	return false, false, nil
}

// fileHasUUID reports whether any load command in the parsed image is LC_UUID.
// It matches the command word in the file's own byte order rather than asserting
// on a concrete type, because debug/macho returns unrecognised commands --
// LC_UUID among them -- as raw LoadBytes.
func fileHasUUID(f *macho.File) bool {
	for _, load := range f.Loads {
		raw := load.Raw()
		if len(raw) >= 4 && f.ByteOrder.Uint32(raw[:4]) == uuidLoadCmd {
			return true
		}
	}
	return false
}

func readMagic(path string) (uint32, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var b [4]byte
	if _, err := io.ReadFull(f, b[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b[:]), nil
}

// isMachOMagic accepts the magics read little-endian in either byte order, so a
// big-endian Mach-O and a fat header are recognised as well as our own thin
// little-endian 64-bit binaries.
func isMachOMagic(m uint32) bool {
	return m == macho.Magic32 || m == macho.Magic64 || m == macho.MagicFat ||
		m == bswap32(macho.Magic32) || m == bswap32(macho.Magic64) || m == bswap32(macho.MagicFat)
}

func bswap32(m uint32) uint32 {
	return m<<24 | (m&0xff00)<<8 | (m>>8)&0xff00 | m>>24
}
