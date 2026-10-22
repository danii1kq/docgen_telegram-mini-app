#!/usr/bin/env python3
"""Validate a .docx with the sibling `docx` skill's validator (find it for you).

Usage:
    python scripts/validate_docx.py out.docx [--auto-repair]

The `docx` skill is expected next to this one (e.g. ``~/.agents/skills/docx``).
If its ``scripts/office/validate.py`` is not found, a lightweight fallback check runs:
ZIP integrity + XML well-formedness of word/document.xml and word/styles.xml.
"""
from __future__ import annotations

import os
import subprocess
import sys
import zipfile
import xml.etree.ElementTree as ET

HERE = os.path.dirname(os.path.abspath(__file__))
CANDIDATES = (
    os.path.join(HERE, os.pardir, os.pardir, "docx", "scripts", "office", "validate.py"),
    os.path.expanduser("~/.agents/skills/docx/scripts/office/validate.py"),
    os.path.expanduser("~/.claude/skills/docx/scripts/office/validate.py"),
    os.environ.get("DOCX_SKILL_VALIDATE", ""),
)


def find_validator():
    for cand in CANDIDATES:
        if cand and os.path.isfile(cand):
            return os.path.abspath(cand)
    return None


def fallback(path):
    print(f"validate_docx: docx-skill validator not found, using fallback check on {path}")
    try:
        with zipfile.ZipFile(path) as z:
            bad = z.testzip()
            if bad:
                print(f"FAIL: corrupt ZIP entry {bad}")
                return 1
            for part in ("word/document.xml", "word/styles.xml"):
                try:
                    ET.fromstring(z.read(part))
                except KeyError:
                    print(f"WARN: {part} missing")
                except ET.ParseError as exc:
                    print(f"FAIL: {part} is not well-formed XML: {exc}")
                    return 1
        print("OK: ZIP integrity + XML well-formedness (document.xml, styles.xml)")
        return 0
    except (FileNotFoundError, zipfile.BadZipFile) as exc:
        print(f"FAIL: {exc}")
        return 1


def main():
    if len(sys.argv) < 2:
        print(__doc__)
        return 2
    path = sys.argv[1]
    validator = find_validator()
    if validator:
        print(f"validate_docx: using {validator}")
        return subprocess.call([sys.executable, validator, *sys.argv[1:]])
    return fallback(path)


if __name__ == "__main__":
    sys.exit(main())
