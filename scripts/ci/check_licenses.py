#!/usr/bin/env python3
"""依存moduleのlicense検査（docs/11-quality-and-ci.md §1の`lint` job）。

`go mod download -json all`が返す全moduleのlicense fileを走査し、§1.1の許可list
に無いlicenseとlicense file不在をfail closedで拒否する。

module graph全体（`all`）を対象にするのは、build対象packageだけに絞ると、後から
importが増えたときに検査対象が静かに広がって見落とすためである。範囲を広く取って
偽陽性を明示的に潰す方を選ぶ。

外部toolを使わないのは、この検査が要求するのが「許可listとの照合」だけで、
汎用license分類器の分類能力までは不要だからである。判定は代表的な条文の
exact phraseで行い、判定できないものは通さない。

使い方:
    check_licenses.py [--root <dir>]
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path

# §1.1の許可license。permissiveのみとし、copyleftはgo.modへ入れる前に仕様判断を要する。
ALLOWED = frozenset({"MIT", "Apache-2.0", "BSD-2-Clause", "BSD-3-Clause", "ISC"})

# license fileの候補名。moduleのroot直下だけを見る。
LICENSE_NAMES = (
    "LICENSE",
    "LICENSE.txt",
    "LICENSE.md",
    "LICENCE",
    "LICENCE.txt",
    "COPYING",
    "COPYING.txt",
)

# 判定に使う代表条文。順序が意味を持つ（BSD-3はBSD-2の条文を含むため先に判定する）。
SIGNATURES: tuple[tuple[str, tuple[str, ...]], ...] = (
    ("Apache-2.0", ("apache license", "version 2.0")),
    ("MIT", ("permission is hereby granted, free of charge",)),
    (
        "BSD-3-Clause",
        ("redistribution and use in source and binary forms", "neither the name of"),
    ),
    ("BSD-2-Clause", ("redistribution and use in source and binary forms",)),
    (
        "ISC",
        ("permission to use, copy, modify, and/or distribute this software for any purpose",),
    ),
)


def force_utf8_stdio() -> None:
    """stdout/stderrをUTF-8へ固定する。

    Windowsのconsole既定encoding（cp1252等）では日本語のdiagnosticが
    UnicodeEncodeErrorになり、検査そのものではなく出力で落ちる。CI matrixは
    両OSで同じscriptを実行するため（docs/11-quality-and-ci.md §5.2）、
    platform差はruntime側で吸収する。
    """
    for stream in (sys.stdout, sys.stderr):
        reconfigure = getattr(stream, "reconfigure", None)
        if reconfigure is not None:
            reconfigure(encoding="utf-8", errors="backslashreplace")


def load_modules(root: Path) -> list[dict]:
    """`go mod download -json all`の連結JSON objectを読む。

    goは配列ではなくobjectを連結して出力するため、raw_decodeで順に切り出す。
    """
    result = subprocess.run(
        ["go", "mod", "download", "-json", "all"],
        cwd=root,
        capture_output=True,
        text=True,
        encoding="utf-8",
    )
    if result.returncode != 0:
        raise SystemExit(f"ERROR go mod download が失敗した\n{result.stderr.strip()}")

    text = result.stdout.strip()
    modules: list[dict] = []
    decoder = json.JSONDecoder()
    index = 0
    while index < len(text):
        obj, index = decoder.raw_decode(text, index)
        modules.append(obj)
        while index < len(text) and text[index] in " \r\n\t":
            index += 1
    return modules


def detect_license(directory: Path) -> tuple[str | None, str | None]:
    """(SPDX識別子, 判定に使ったfile名)を返す。判定できなければ(None, file名|None)。"""
    for name in LICENSE_NAMES:
        path = directory / name
        if not path.is_file():
            continue
        body = path.read_text(encoding="utf-8", errors="replace").lower()
        body = re.sub(r"\s+", " ", body)
        for spdx, phrases in SIGNATURES:
            if all(phrase in body for phrase in phrases):
                return spdx, name
        return None, name
    return None, None


def main() -> int:
    force_utf8_stdio()
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", default=str(Path(__file__).resolve().parents[2]))
    args = parser.parse_args()
    root = Path(args.root).resolve()

    if not (root / "go.mod").is_file():
        print(
            "go.mod が無いため依存license検査を実行していない。"
            "P0-02 で go.mod を導入した時点から実質的な検査になる（docs/13-progress.md P0-02）"
        )
        return 0

    modules = load_modules(root)
    errors: list[str] = []
    counts: dict[str, int] = {}

    for module in modules:
        path = module.get("Path", "?")
        version = module.get("Version", "?")
        directory = module.get("Dir")
        if not directory:
            errors.append(f"{path}@{version}: module cacheのDirが取得できない")
            continue
        spdx, file_name = detect_license(Path(directory))
        if spdx is None and file_name is None:
            errors.append(f"{path}@{version}: license fileが見つからない（{', '.join(LICENSE_NAMES)}）")
            continue
        if spdx is None:
            errors.append(f"{path}@{version}: {file_name} のlicenseを判定できない")
            continue
        if spdx not in ALLOWED:
            errors.append(f"{path}@{version}: {spdx} は許可listに無い（許可: {', '.join(sorted(ALLOWED))}）")
            continue
        counts[spdx] = counts.get(spdx, 0) + 1

    print(f"検査したmodule数: {len(modules)}")
    for spdx in sorted(counts):
        print(f"  {spdx}: {counts[spdx]}件")
    if errors:
        for error in errors:
            print(f"ERROR {error}")
        print(f"依存license検査に失敗した: {len(errors)}件")
        return 1
    print("依存license検査に成功した: 全moduleが許可listのlicense")
    return 0


if __name__ == "__main__":
    sys.exit(main())
