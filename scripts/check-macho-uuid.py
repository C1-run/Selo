#!/usr/bin/env python3
"""Fail closed if a Mach-O binary lacks the LC_UUID load command.

Newer macOS dyld refuses to load a Mach-O with no LC_UUID:

    dyld: missing LC_UUID load command in <path>
    signal: abort trap

Go toolchains before 1.24 omit that load command on darwin, so a release built
with one ships a macOS binary that cannot start on current macOS. install.sh
verifies the bytes but cannot know whether the binary loads, and nothing else in
this repo could catch it either -- which is how v0.4.1, v0.5.0 and v0.5.1 all
shipped darwin binaries that newer macOS rejects.

Usage: check-macho-uuid.py <binary> [<binary> ...]

Non-Mach-O inputs (Linux ELF) are reported and skipped. Exits 0 only when every
Mach-O input was checked and carries LC_UUID.
"""

import struct
import sys

LC_UUID = 0x1B
MH_MAGIC = 0xFEEDFACE
MH_MAGIC_64 = 0xFEEDFACF
FAT_MAGICS = (0xCAFEBABE, 0xCAFEBABF)


def slices(data):
    """Yield the file offset of each Mach-O slice in the image."""
    if struct.unpack_from(">I", data, 0)[0] in FAT_MAGICS:
        count = struct.unpack_from(">I", data, 4)[0]
        for i in range(count):
            yield struct.unpack_from(">I", data, 8 + i * 20 + 8)[0]
    else:
        yield 0


def slice_has_uuid(data, offset):
    """True/False for a Mach-O slice, None when the bytes are not Mach-O."""
    magic = struct.unpack_from("<I", data, offset)[0]
    if magic == MH_MAGIC_64:
        cmd_offset = offset + 32
    elif magic == MH_MAGIC:
        cmd_offset = offset + 28
    else:
        return None

    ncmds = struct.unpack_from("<I", data, offset + 16)[0]
    for _ in range(ncmds):
        cmd, cmdsize = struct.unpack_from("<II", data, cmd_offset)
        if cmd == LC_UUID:
            return True
        if cmdsize == 0:
            return None
        cmd_offset += cmdsize
    return False


def main(argv):
    if len(argv) < 2:
        print(__doc__.strip(), file=sys.stderr)
        return 2

    failures = []
    checked = 0
    for path in argv[1:]:
        try:
            with open(path, "rb") as fh:
                data = fh.read()
        except OSError as exc:
            print(f"FAIL  {path}: {exc}")
            failures.append(path)
            continue

        if len(data) < 4:
            print(f"FAIL  {path}: too short to be a Mach-O")
            failures.append(path)
            continue

        verdicts = [slice_has_uuid(data, off) for off in slices(data)]
        macho = [v for v in verdicts if v is not None]
        if not macho:
            print(f"skip  {path} (not a Mach-O)")
            continue

        checked += 1
        if all(macho):
            print(f"ok    {path} carries LC_UUID")
        else:
            print(f"FAIL  {path} has no LC_UUID: newer macOS dyld aborts on load")
            failures.append(path)

    if checked == 0:
        print("error: no Mach-O binary was checked", file=sys.stderr)
        return 2

    if failures:
        print(f"\n{len(failures)} of {checked} Mach-O binaries are not loadable", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
