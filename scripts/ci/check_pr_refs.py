#!/usr/bin/env python3
"""PRのhead/base検査（docs/11-quality-and-ci.md §5.3の`policy` job）。

§5.2の命名grammarと§5.3の表にないsource→targetを拒否する。統合方向を機械的に
固定しないと、異なるagentのwork branchをbaseにしたPRや、featureから直接
`develop/work`へ入れるPRがreview時にしか気付けない。

`<task-id>`はdocs/13-progress.mdに実在するIDのASCII lowercaseでなければならない。
台帳から集合を読むのは、branch名だけを見てもtask IDとslugの境界を一意に切れない
ためである（`p0-01-ci-matrix`はtask ID`p0-01`＋slug`ci-matrix`であって、
`p0`＋`01-ci-matrix`ではない）。

使い方:
    check_pr_refs.py [--head <ref>] [--base <ref>]

引数を省略した場合はGITHUB_HEAD_REF / GITHUB_BASE_REFを使う。どちらも空なら
pull_request event以外での実行とみなし、検査をskipして成功する。
"""

from __future__ import annotations

import argparse
import os
import re
import sys
from pathlib import Path

PROGRESS_DOC = Path("docs/13-progress.md")

# 台帳のcheckbox行。`- [x] **P0-01** ...` と `- [ ] **G-CI**: ...` の両方に合う。
TASK_ID = re.compile(r"^- \[[ x!~\-]\] \*\*([A-Za-z0-9][A-Za-z0-9-]*)\*\*")

# §5.2のslug grammar。1～48文字。
SLUG = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")
SLUG_MAX = 48

AGENTS = ("claude", "codex")

# §5.3の表。keyはbase、valueは許可するheadの集合または判定関数名。
INTEGRATION_TARGETS = {
    "claude/work": "claude-feature",
    "codex/work": "codex-feature",
    "develop/work": "agent-work",
    "main": "develop-work",
}


def load_task_ids(root: Path) -> set[str]:
    path = root / PROGRESS_DOC
    if not path.exists():
        raise SystemExit(f"ERROR 進捗台帳が見つからない: {PROGRESS_DOC}")
    ids = {
        match.group(1).lower()
        for match in (TASK_ID.match(line) for line in path.read_text(encoding="utf-8").splitlines())
        if match
    }
    if not ids:
        raise SystemExit(f"ERROR {PROGRESS_DOC}からtask IDを1件も抽出できなかった")
    return ids


def split_feature(ref: str, task_ids: set[str]) -> tuple[str, str, str] | None:
    """`<agent>/feature-<task-id>-<slug>`を分解する。合わなければNone。

    task IDは最長一致で切る。短いIDが長いIDのprefixになる場合（`p1`と`p1-01`）に
    誤った境界を選ばないようにするためである。
    """
    agent, _, rest = ref.partition("/")
    if agent not in AGENTS or not rest.startswith("feature-"):
        return None
    body = rest[len("feature-") :]
    for task_id in sorted(task_ids, key=len, reverse=True):
        prefix = f"{task_id}-"
        if body.startswith(prefix):
            return agent, task_id, body[len(prefix) :]
    return None


def check(head: str, base: str, task_ids: set[str]) -> list[str]:
    errors: list[str] = []
    expected = INTEGRATION_TARGETS.get(base)
    if expected is None:
        return [
            f"base `{base}` は§5.3の統合先ではない。"
            f"許可するbaseは {', '.join(sorted(INTEGRATION_TARGETS))}"
        ]

    if expected in ("claude-feature", "codex-feature"):
        agent = expected.split("-", 1)[0]
        parts = split_feature(head, task_ids)
        if parts is None:
            errors.append(
                f"head `{head}` が§5.2のfeature branch grammarに合わない。"
                f"`{agent}/feature-<task-id>-<slug>`とし、<task-id>は"
                f"{PROGRESS_DOC}に実在するIDのASCII lowercaseにする"
            )
            return errors
        head_agent, task_id, slug = parts
        if head_agent != agent:
            errors.append(
                f"head `{head}` のagentは`{head_agent}`だがbaseは`{base}`。"
                "異なるagentのwork branchをbaseにしない（§5.2）"
            )
        if not SLUG.match(slug):
            errors.append(
                f"head `{head}` のslug `{slug}` が grammar `[a-z0-9]+(?:-[a-z0-9]+)*` に合わない"
            )
        elif len(slug) > SLUG_MAX:
            errors.append(f"head `{head}` のslug `{slug}` が{SLUG_MAX}文字を超える ({len(slug)}文字)")
        if not errors:
            print(f"OK  {head} -> {base}  (task-id={task_id}, slug={slug})")
        return errors

    if expected == "agent-work":
        if head not in (f"{agent}/work" for agent in AGENTS):
            errors.append(
                f"head `{head}` から`{base}`へは統合できない。"
                "`claude/work`または`codex/work`だけが§5.3の統合元である"
            )
        else:
            print(f"OK  {head} -> {base}")
        return errors

    if head != "develop/work":
        errors.append(f"head `{head}` から`{base}`へは統合できない。`develop/work`だけが統合元である")
    else:
        print(f"OK  {head} -> {base}")
    return errors


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


def main() -> int:
    force_utf8_stdio()
    parser = argparse.ArgumentParser()
    parser.add_argument("--head", default=os.environ.get("GITHUB_HEAD_REF", ""))
    parser.add_argument("--base", default=os.environ.get("GITHUB_BASE_REF", ""))
    parser.add_argument("--root", default=str(Path(__file__).resolve().parents[2]))
    args = parser.parse_args()

    head = args.head.strip()
    base = args.base.strip()
    if not head or not base:
        print("pull_request eventではないためPR ref検査をskipした")
        return 0

    errors = check(head, base, load_task_ids(Path(args.root)))
    if errors:
        for error in errors:
            print(f"ERROR {error}")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
