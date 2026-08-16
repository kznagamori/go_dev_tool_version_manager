#!/usr/bin/env python3
"""message catalogの網羅検査（docs/13-progress.md P4-02、`policy` job）。

docs/04-storage-and-data.md §20が`registry/messages/ja.toml`を「§7のmessage ID
grammarに従うASCII dotted key集合」と定め、docs/11-quality-and-ci.md §10が
「日本語message catalogはkey/parameter集合をschemaとtestで検査する」と定める。

message IDはpackageをまたいで散らばり、`internal/registry`から他packageをimport
して集めることはできない（docs/02-architecture.md §1の依存方向）。そこで、
source全体を走査してcatalogと突き合わせる検査をここへ置く。

検査は両方向である。

1. production GoとregistryのTOMLが出しうるmessage IDが、すべてcatalogにある。
   欠けていると、その失敗が起きたときに表示するtextが無い。
2. catalogのkeyが、すべてどこかから参照されている。CLAUDE.md §7の
   「未使用のenum値、kind、fieldを『将来のため』に残さない」をcatalogへも適用する。

使い方:
    check_messages.py [--root <dir>]
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

CATALOG = "registry/messages/ja.toml"

# docs/04-storage-and-data.md §7のmessage ID grammar。
MESSAGE_ID = re.compile(r"^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$")

# message IDを第1引数に取るtyped error helper（internal/config、internal/catalog）。
#
# 表で持つのは、helperを増やしたときに「どの引数がmessage IDか」を宣言させるため
# である。宣言の無いhelper経由のIDはcatalogへ載らず、表示できない失敗になる。
FIRST_ARG_HELPERS = (
    "configError",
    "usageError",
    "projectError",
    "filesystemError",
    "pathUnsafeError",
    "permissionError",
    "filesystemErrorWithRole",
    "messageID",
    "ParseMessageID",
)

# message IDを第2引数に取るhelper。
SECOND_ARG_HELPERS = ("requireSchema",)

CALL_FIRST = re.compile(
    r"\b(?:" + "|".join(FIRST_ARG_HELPERS) + r")\(\s*\"([a-z][a-z0-9_.]*)\""
)
CALL_SECOND = re.compile(
    r"\b(?:" + "|".join(SECOND_ARG_HELPERS) + r")\([^,()]+,\s*\"([a-z][a-z0-9_.]*)\""
)

# message IDを値に持つ定数宣言。`reason*`はdocs/06-tool-definition.md §13の
# stable reason codeで、Diagnostic.Reasonがmessage IDそのものを使う。
CONST_DECL = re.compile(
    r"\b(?:MessageID[A-Za-z]*|message[A-Z][A-Za-z]*|reason[A-Z][A-Za-z]*)\s*=\s*"
    r"\"([a-z][a-z0-9_.]*)\""
)

# registry definitionがmessage IDを書くkey（docs/06-tool-definition.md §5・§5.1）。
TOML_KEYS = re.compile(r"^\s*(?:license_notice|adoption_reason)\s*=\s*\"([^\"]+)\"")

# catalogのkey行。値は複数行を取らない前提で、key側だけを読む。
CATALOG_KEY = re.compile(r"^([A-Za-z0-9_.]+)\s*=")


def force_utf8_stdio() -> None:
    """stdout/stderrをUTF-8へ固定する。

    Windowsのconsole既定encodingでは日本語のdiagnosticがUnicodeEncodeErrorになる。
    CI matrixは両OSで同じscriptを実行するため（docs/11-quality-and-ci.md §5.2）、
    platform差はruntime側で吸収する。
    """
    for stream in (sys.stdout, sys.stderr):
        reconfigure = getattr(stream, "reconfigure", None)
        if reconfigure is not None:
            reconfigure(encoding="utf-8", errors="backslashreplace")


def production_go_files(root: Path) -> list[Path]:
    """testdataと`_test.go`を除くGo fileを返す。

    testが自分のfixtureとして書くIDはcatalogの対象ではない。
    """
    files: list[Path] = []
    for path in sorted(root.rglob("*.go")):
        if "testdata" in path.relative_to(root).parts:
            continue
        if path.name.endswith("_test.go"):
            continue
        files.append(path)
    return files


def referenced_ids(root: Path) -> dict[str, list[str]]:
    """source全体が参照するmessage IDを {ID: [参照元]} で返す。"""
    found: dict[str, list[str]] = {}

    def record(message_id: str, origin: str) -> None:
        if not MESSAGE_ID.match(message_id):
            return
        found.setdefault(message_id, []).append(origin)

    for path in production_go_files(root):
        rel = path.relative_to(root).as_posix()
        text = path.read_text(encoding="utf-8")
        for pattern in (CALL_FIRST, CALL_SECOND, CONST_DECL):
            for match in pattern.finditer(text):
                record(match.group(1), rel)

    for path in sorted((root / "registry").rglob("*.toml")):
        rel = path.relative_to(root).as_posix()
        if rel == CATALOG:
            continue
        for line in path.read_text(encoding="utf-8").splitlines():
            match = TOML_KEYS.match(line)
            if match:
                record(match.group(1), rel)

    return found


def catalog_keys(path: Path) -> list[str]:
    """catalogのkeyを宣言順で返す。"""
    keys: list[str] = []
    for line in path.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue
        match = CATALOG_KEY.match(stripped)
        if match:
            keys.append(match.group(1))
    return keys


def main() -> int:
    force_utf8_stdio()
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", default=str(Path(__file__).resolve().parents[2]))
    args = parser.parse_args()
    root = Path(args.root).resolve()

    catalog_path = root / CATALOG
    if not catalog_path.exists():
        print(f"ERROR {CATALOG} が無い（docs/07-registry-and-tools.md §2のexact tree）")
        return 1

    keys = catalog_keys(catalog_path)
    declared = set(keys)
    referenced = referenced_ids(root)

    errors: list[str] = []

    if len(keys) != len(declared):
        seen: set[str] = set()
        for key in keys:
            if key in seen:
                errors.append(f"{CATALOG}: key `{key}` が重複している")
            seen.add(key)

    for key in keys:
        if not MESSAGE_ID.match(key):
            errors.append(
                f"{CATALOG}: key `{key}` が§7のmessage ID grammarに合わない"
            )

    for message_id in sorted(referenced):
        if message_id not in declared:
            origins = sorted(set(referenced[message_id]))
            errors.append(
                f"{origins[0]}: message ID `{message_id}` が {CATALOG} に無い。"
                "表示textが無い失敗を作らないため、同じ変更でcatalogへ追加する"
            )

    for key in sorted(declared - set(referenced)):
        errors.append(
            f"{CATALOG}: key `{key}` をsourceが参照していない。"
            "使わないmessageを将来のために残さない（CLAUDE.md §7）"
        )

    print(f"catalog key数: {len(keys)} / source参照ID数: {len(referenced)}")
    if errors:
        for error in errors:
            print(f"ERROR {error}")
        print(f"message catalog検査に失敗した: {len(errors)}件")
        return 1
    print("message catalog検査に成功した: 網羅と未使用なし")
    return 0


if __name__ == "__main__":
    sys.exit(main())
