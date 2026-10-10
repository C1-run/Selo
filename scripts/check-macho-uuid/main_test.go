package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	magic32     = 0xfeedface
	magic64     = 0xfeedfacf
	magicFat    = 0xcafebabe
	cpuTypeX86  = 0x01000007
	fileTypeExe = 2
	cmdUUID     = 0x1b
	uuidSize    = 24
)

// thinMachO builds a minimal Mach-O by hand: a header followed by the given load
// commands. Fixtures are assembled rather than compiled so the tests do not
// depend on the host having a Mach-O-producing toolchain, and so the exact shape
// a Go < 1.24 darwin build produces (no LC_UUID anywhere) can be reproduced.
func thinMachO(order binary.ByteOrder, is64 bool, loads ...[]byte) []byte {
	headerSize := 28
	if is64 {
		headerSize = 32
	}

	size := 0
	for _, load := range loads {
		size += len(load)
	}

	b := make([]byte, headerSize)
	magic := uint32(magic32)
	if is64 {
		magic = magic64
	}
	order.PutUint32(b[0:], magic)
	order.PutUint32(b[4:], cpuTypeX86)
	order.PutUint32(b[8:], 3) // cpusubtype
	order.PutUint32(b[12:], fileTypeExe)
	order.PutUint32(b[16:], uint32(len(loads))) // ncmds
	order.PutUint32(b[20:], uint32(size))       // sizeofcmds
	// b[24:] is flags (32-bit) or flags+reserved (64-bit); zero is fine.

	for _, load := range loads {
		b = append(b, load...)
	}
	return b
}

// uuidLoad is an LC_UUID load command: cmd, cmdsize, then the 16-byte UUID.
func uuidLoad(order binary.ByteOrder) []byte {
	load := make([]byte, uuidSize)
	order.PutUint32(load[0:], cmdUUID)
	order.PutUint32(load[4:], uuidSize)
	copy(load[8:], "0123456789abcdef")
	return load
}

// fatBinary wraps inner in a one-architecture fat header, as `lipo` would. A fat
// header is always big-endian, whatever the slices inside it are.
func fatBinary(inner []byte) []byte {
	const headerSize = 8 + 20
	header := make([]byte, headerSize)
	binary.BigEndian.PutUint32(header[0:], magicFat)
	binary.BigEndian.PutUint32(header[4:], 1) // nfat_arch
	binary.BigEndian.PutUint32(header[8:], cpuTypeX86)
	binary.BigEndian.PutUint32(header[12:], 3) // cpusubtype
	binary.BigEndian.PutUint32(header[16:], headerSize)
	binary.BigEndian.PutUint32(header[20:], uint32(len(inner)))
	binary.BigEndian.PutUint32(header[24:], 0) // align
	return append(header, inner...)
}

func fixture(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheckUUIDFindsTheLoadCommand(t *testing.T) {
	has, isMachO, err := checkUUID(fixture(t, "selo-darwin-arm64", thinMachO(binary.LittleEndian, true, uuidLoad(binary.LittleEndian))))
	if err != nil {
		t.Fatalf("checkUUID: %v", err)
	}
	if !isMachO {
		t.Fatal("fixture should be recognised as Mach-O")
	}
	if !has {
		t.Fatal("a Mach-O carrying LC_UUID was reported as missing it")
	}
}

func TestCheckUUIDReportsTheMissingLoadCommand(t *testing.T) {
	has, isMachO, err := checkUUID(fixture(t, "selo-darwin-arm64", thinMachO(binary.LittleEndian, true)))
	if err != nil {
		t.Fatalf("checkUUID: %v", err)
	}
	if !isMachO {
		t.Fatal("fixture should be recognised as Mach-O")
	}
	if has {
		t.Fatal("a Mach-O with no LC_UUID was reported as carrying it")
	}
}

// A 32-bit header is four bytes shorter and its commands start earlier, so the
// parser must not assume the 64-bit layout.
func TestCheckUUID32BitMachO(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"with uuid", true},
		{"without uuid", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var loads [][]byte
			if tc.want {
				loads = append(loads, uuidLoad(binary.LittleEndian))
			}
			has, isMachO, err := checkUUID(fixture(t, "thin32", thinMachO(binary.LittleEndian, false, loads...)))
			if err != nil {
				t.Fatalf("checkUUID: %v", err)
			}
			if !isMachO {
				t.Fatal("a 32-bit Mach-O should be recognised as Mach-O")
			}
			if has != tc.want {
				t.Errorf("has = %v, want %v", has, tc.want)
			}
		})
	}
}

// A big-endian image stores both the magic and the command word reversed. An
// implementation that hardcodes little-endian would read the wrong command word
// and report a UUID-carrying binary as broken.
func TestCheckUUIDBigEndianMachO(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"with uuid", true},
		{"without uuid", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var loads [][]byte
			if tc.want {
				loads = append(loads, uuidLoad(binary.BigEndian))
			}
			has, isMachO, err := checkUUID(fixture(t, "thin-be", thinMachO(binary.BigEndian, true, loads...)))
			if err != nil {
				t.Fatalf("checkUUID: %v", err)
			}
			if !isMachO {
				t.Fatal("a big-endian Mach-O should be recognised as Mach-O")
			}
			if has != tc.want {
				t.Errorf("has = %v, want %v", has, tc.want)
			}
		})
	}
}

func TestCheckUUIDFatBinary(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"with uuid", true},
		{"without uuid", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inner := thinMachO(binary.LittleEndian, true)
			if tc.want {
				inner = thinMachO(binary.LittleEndian, true, uuidLoad(binary.LittleEndian))
			}
			has, isMachO, err := checkUUID(fixture(t, "fat", fatBinary(inner)))
			if err != nil {
				t.Fatalf("checkUUID: %v", err)
			}
			if !isMachO {
				t.Fatal("a fat binary should be recognised as Mach-O")
			}
			if has != tc.want {
				t.Errorf("has = %v, want %v", has, tc.want)
			}
		})
	}
}

func TestCheckUUIDSkipsNonMachO(t *testing.T) {
	elf := make([]byte, 64)
	copy(elf, "\x7fELF")

	has, isMachO, err := checkUUID(fixture(t, "selo-linux-amd64", elf))
	if err != nil {
		t.Fatalf("a non-Mach-O input should be skipped, not failed: %v", err)
	}
	if isMachO || has {
		t.Fatal("an ELF was reported as Mach-O")
	}
}

// A file with a Mach-O magic number that does not parse must be a failure, not a
// silent skip: skipping it would let a truncated or corrupted darwin binary ship.
func TestCheckUUIDFailsClosedOnUnparsableMachO(t *testing.T) {
	truncated := thinMachO(binary.LittleEndian, true, uuidLoad(binary.LittleEndian))[:20]

	_, isMachO, err := checkUUID(fixture(t, "truncated", truncated))
	if err == nil {
		t.Fatal("a Mach-O that does not parse must be an error, not a skip")
	}
	if !isMachO {
		t.Fatal("a file with a Mach-O magic number should be reported as Mach-O")
	}
}

// An input that cannot be read must not be waved through as "not a Mach-O": a
// missing or renamed release artifact has to stop the gate. This was a real
// fail-open in the first draft, caught by TestRunExitCodes.
func TestCheckUUIDFailsClosedOnUnreadableInput(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "selo-darwin-nope")
	_, isMachO, err := checkUUID(missing)
	if err == nil {
		t.Fatal("a missing file must be an error, not a skip")
	}
	if isMachO {
		t.Fatal("a missing file is not Mach-O")
	}

	short := fixture(t, "selo-darwin-arm64", []byte{0xcf, 0xfa})
	if _, _, err := checkUUID(short); err == nil {
		t.Fatal("a file too short to hold a magic number must be an error, not a skip")
	}
}

func TestIsMachOMagic(t *testing.T) {
	for _, m := range []uint32{magic32, magic64, magicFat, 0xcefaedfe, 0xcffaedfe, 0xbebafeca} {
		if !isMachOMagic(m) {
			t.Errorf("magic %#x should be recognised as Mach-O", m)
		}
	}
	for _, m := range []uint32{0x7f454c46 /* ELF */, 0x4d5a /* MZ */, 0} {
		if isMachOMagic(m) {
			t.Errorf("magic %#x should not be recognised as Mach-O", m)
		}
	}
}

// The exit status is the whole contract of this command, so it is pinned here
// rather than left to the workflow to discover.
func TestRunExitCodes(t *testing.T) {
	good := fixture(t, "selo-darwin-arm64", thinMachO(binary.LittleEndian, true, uuidLoad(binary.LittleEndian)))
	bad := fixture(t, "selo-darwin-amd64", thinMachO(binary.LittleEndian, true))
	elf := fixture(t, "selo-linux-amd64", append([]byte("\x7fELF"), make([]byte, 60)...))
	short := fixture(t, "selo-darwin-truncated", []byte{0xcf, 0xfa})
	missing := filepath.Join(t.TempDir(), "selo-darwin-nope")

	for _, tc := range []struct {
		name  string
		paths []string
		want  int
	}{
		{"no arguments", nil, 2},
		{"a loadable binary", []string{good}, 0},
		{"a binary with no LC_UUID", []string{bad}, 1},
		{"a missing file", []string{missing}, 1},
		{"a file too short to hold a magic number", []string{short}, 1},
		{"one good, one broken", []string{good, bad}, 1},
		{"only non-Mach-O", []string{elf}, 2},
		{"a non-Mach-O plus a good binary", []string{elf, good}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run(tc.paths, &stdout, &stderr); got != tc.want {
				t.Errorf("run() = %d, want %d\nstdout: %s\nstderr: %s",
					got, tc.want, stdout.String(), stderr.String())
			}
		})
	}
}

// A binary that is broken must say so on stdout; the release log is the only
// place an operator will look when the step turns red.
func TestRunNamesTheBrokenBinary(t *testing.T) {
	bad := fixture(t, "selo-darwin-amd64", thinMachO(binary.LittleEndian, true))

	var stdout, stderr bytes.Buffer
	if got := run([]string{bad}, &stdout, &stderr); got != 1 {
		t.Fatalf("run() = %d, want 1", got)
	}
	if !strings.Contains(stdout.String(), bad) || !strings.Contains(stdout.String(), "LC_UUID") {
		t.Errorf("stdout should name the broken binary and the missing command, got: %s", stdout.String())
	}
}
