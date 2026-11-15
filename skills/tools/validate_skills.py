#!/usr/bin/env python3
"""skills 静态校验器（M1 INF-01）。

用法：python3 tools/validate_skills.py [目录...] [--strict]
不传目录时校验 skills/ 下全部技能（排除清单与 workspace）。

检查项：
  F1 SKILL.md frontmatter 存在且 name == 目录名
  F2 description 非空且 ≤1024（--strict 时建议 ≤700，仅警告）
  F3 相对链接/引用的文件存在（支持 subskills/*/SKILL.md 通配白名单）
  F4 evals/*.json 可解析且满足最小 schema（id 唯一；行为 case 有 expectations）
  F5 fixture 路径存在
退出码 0 = 无 error；1 = 有 error；2 = 用法错误。
"""
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

SKILLS_ROOT = Path(__file__).resolve().parent.parent
EXCLUDED = {"skill-creator", "unslop", "oss-analysis", "tools", "plan", "output",
            ".session_tmps", "andrew-translate-workspace", "code-refactor-workspace"}
WORKSPACE_SUFFIX = "-workspace"
LINK_RE = re.compile(r"\]\(([^)#\s]+)(?:#[^)]*)?\)|`((?:references|scripts|subskills|evals|assets|benchmarks)/[^`\s]+)`")
WILDCARD_OK = re.compile(r"^subskills/\*/SKILL\.md$")
DESC_LIMIT = 1024
DESC_WARN = 700


def frontmatter(text: str):
    m = re.match(r"^---\n(.*?)\n---\n", text, re.S)
    if not m:
        return None, {}
    fields = {}
    cur = None
    for line in m.group(1).splitlines():
        km = re.match(r"^([A-Za-z_][\w-]*):\s*(.*)$", line)
        if km:
            cur = km.group(1)
            fields[cur] = km.group(2).strip().strip('"')
        elif cur and line.startswith(("  ", "\t")):
            fields[cur] += " " + line.strip()
    return m.group(1), fields


def check_skill(base: Path):
    errors, warns = [], []
    for skill_md in sorted(base.rglob("SKILL.md")):
        rel = skill_md.relative_to(SKILLS_ROOT)
        text = skill_md.read_text(encoding="utf-8")
        raw, fm = frontmatter(text)
        if raw is None:
            errors.append(f"{rel}: F1 missing frontmatter")
            continue
        if fm.get("name") != skill_md.parent.name:
            errors.append(f"{rel}: F1 frontmatter name={fm.get('name')!r} != dir {skill_md.parent.name!r}")
        desc = fm.get("description", "")
        if not desc:
            errors.append(f"{rel}: F2 empty description")
        elif len(desc) > DESC_LIMIT:
            errors.append(f"{rel}: F2 description {len(desc)} > {DESC_LIMIT}")
        elif len(desc) > DESC_WARN:
            warns.append(f"{rel}: F2 description {len(desc)} > {DESC_WARN} (recommended max)")
        # F3 链接
        for m in LINK_RE.finditer(text):
            target = (m.group(1) or m.group(2)).rstrip(".,;:")
            if re.match(r"^(https?|mailto):", target) or target.startswith("<"):
                continue
            if WILDCARD_OK.match(target):
                if list(skill_md.parent.glob(target)):
                    continue
                errors.append(f"{rel}: F3 no file matches wildcard {target}")
                continue
            if not (skill_md.parent / target).exists():
                errors.append(f"{rel}: F3 broken link {target}")
    # F4/F5 evals
    for ev in sorted(base.rglob("evals/*.json")):
        rel = ev.relative_to(SKILLS_ROOT)
        try:
            data = json.loads(ev.read_text(encoding="utf-8"))
        except json.JSONDecodeError as e:
            errors.append(f"{rel}: F4 invalid JSON: {e}")
            continue
        cases = data.get("evals") if isinstance(data, dict) else data
        if not isinstance(cases, list):
            errors.append(f"{rel}: F4 no 'evals' array")
            continue
        ids = set()
        is_trigger = "trigger" in ev.name
        # evals.json 的 files 路径以技能根为基准（如 evals/files/…），trigger-evals 是纯 query 列表
        file_base = base if base.name != "evals" else base.parent
        for c in cases:
            cid = c.get("id")
            if cid is not None:
                if cid in ids:
                    errors.append(f"{rel}: F4 duplicate id {cid}")
                ids.add(cid)
            if not is_trigger and not c.get("expectations"):
                errors.append(f"{rel}: F4 case {cid} has no expectations")
            if is_trigger and "should_trigger" not in c:
                errors.append(f"{rel}: F4 trigger case missing should_trigger")
            for f in c.get("files", []) or []:
                if not (file_base / f).exists():
                    errors.append(f"{rel}: F5 case {cid} missing file {f}")
    return errors, warns


def main():
    args = [a for a in sys.argv[1:] if not a.startswith("-")]
    strict = "--strict" in sys.argv
    if args:
        bases = [Path(a).resolve() for a in args]
    else:
        bases = [p for p in sorted(SKILLS_ROOT.iterdir())
                 if p.is_dir() and p.name not in EXCLUDED and not p.name.endswith(WORKSPACE_SUFFIX)
                 and not p.name.startswith(".")]
    total_err, total_warn = [], []
    for base in bases:
        e, w = check_skill(base)
        total_err += e
        total_warn += w
    for e in total_err:
        print("ERROR", e)
    for w in total_warn:
        print("WARN ", w)
    print(f"\n{len(bases)} skill dirs checked: {len(total_err)} errors, {len(total_warn)} warnings")
    sys.exit(1 if total_err else 0)


if __name__ == "__main__":
    main()
