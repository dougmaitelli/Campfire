"""Regenerate independent tar decoder fixtures using Python's standard library."""

import bz2
import io
import lzma
from pathlib import Path
import struct
import tarfile
import zlib


def archive(entries):
    output = io.BytesIO()
    with tarfile.open(fileobj=output, mode="w", format=tarfile.USTAR_FORMAT) as writer:
        for name, kind, content in entries:
            header = tarfile.TarInfo(name)
            header.mode = 0o000  # Extraction must restore private writable defaults.
            header.type = kind
            if kind == tarfile.SYMTYPE:
                header.linkname = "../outside"
            header.size = len(content) if kind == tarfile.REGTYPE else 0
            writer.addfile(header, io.BytesIO(content))
    return output.getvalue()


payload = b"saved world\n"
variants = {
    "valid": [("world/", tarfile.DIRTYPE, b""), ("world/save.txt", tarfile.REGTYPE, payload)],
    "duplicate": [("save.txt", tarfile.REGTYPE, b"first"), ("save.txt", tarfile.REGTYPE, b"second")],
    "traversal": [("../outside", tarfile.REGTYPE, payload)],
    "link": [("link", tarfile.SYMTYPE, b"")],
    "special": [("pipe", tarfile.FIFOTYPE, b"")],
    "type-conflict": [("world", tarfile.REGTYPE, payload), ("world/save.txt", tarfile.REGTYPE, payload)],
}

archives = {name: archive(entries) for name, entries in variants.items()}
oversized = tarfile.TarInfo("too-large")
oversized.size = 100 * 1024**3 + 1
archives["oversized"] = oversized.tobuf(format=tarfile.GNU_FORMAT) + bytes(1024)

for name, data in archives.items():
    root = Path(__file__).parent
    (root / f"{name}.tar").write_bytes(data)
    (root / f"{name}.tar.bz2").write_bytes(bz2.compress(data))
    (root / f"{name}.tar.xz").write_bytes(lzma.compress(data))
    compressed = lzma.compress(data, format=lzma.FORMAT_RAW, filters=[
        {"id": lzma.FILTER_LZMA1, "dict_size": 4096, "lc": 3, "lp": 0, "pb": 2}
    ])
    member = b"LZIP\x01\x0c" + compressed
    member += struct.pack("<IQQ", zlib.crc32(data), len(data), len(member) + 20)
    (root / f"{name}.tar.lz").write_bytes(member)
