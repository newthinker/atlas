#!/usr/bin/env python3
"""
TaskCompleted · Jev 质量闸门

调用顺序（重要）：Claude Code 会并行执行同一事件的所有 hook，因此本脚本【不能】单独注册，
必须由 .claude/hooks/task-completed.sh 在确定性检查（测试、覆盖率）通过后再调用：

    task-completed.sh:  run tests → coverage → exec python3 jev/task_completed_jev.py

输入：stdin 的 TaskCompleted JSON（task_id / task_subject / task_description / teammate_name ...）
      + 任务 JSON（Leader 写入的任务契约）的 done_criteria —— 只取 verify_by: review 的条目
输出：
  shadow  模式：只记录，exit 0
  enforce 模式：route=qa → exit 0；其余 → stderr 写反馈 + exit 2（阻止任务标记为完成）
Jev 不可用：记录错误，exit 0 —— 回落到原有流程（QA 照常进行），不因外部服务故障卡住开发。
"""
from __future__ import annotations

import sys

# 关掉 .pyc 生成。本模块在目标仓库里以 hook 身份运行，而那些仓库的忽略规则不受我们控制；
# 一旦生成 __pycache__/，它会被 task-completed.sh 的 scope 漂移判据全额算到下一个转
# dev_done 的人头上——挡的是整个 wave，且被挡的人与污染源毫无关系。
# 位置必须在导入 jev_common / jev_routing / dod_filter 之前：晚一行，那几份 .pyc 就已经落盘了。
sys.dont_write_bytecode = True

import json
import subprocess
import time
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

sys.path.insert(0, str(Path(__file__).resolve().parent))

from jev_common import (  # noqa: E402
    JevError, arcforge_dir, call_jev, load_config, log_error, log_event,
    project_allowed, project_root, read_hook_input, redact,
)
from jev_routing import (  # noqa: E402
    ROUTE_HUMAN, ROUTE_QA, ROUTE_REWORK,
    Decision, QualityThresholds, parse_quality_answers, route_task,
)
from dod_filter import JUDGE_OK, judge  # noqa: E402

# 维度遍历顺序固定。dod_N 的编号按此顺序连续分配——顺序一变，同一条目在两次判定
# 之间就会换 key，影子期的逐条校准表直接失去对齐基准。
DIMENSIONS = ("functional", "boundary", "error_handling", "non_functional")

# 进 Jev 上下文的 verify_by：这些条目已有确定性证据，告诉 Jev「这些不用你判」，
# 让它把注意力集中在 review 条目上。manual 不进——它既不是证据，也不该被参考。
CONTEXT_VERIFY_BY = ("test", "benchmark")

VERDICT_CRITERIA = {
    "pass": "Every requirement appears implemented and the change is ready for code review",
    "needs_work": "At least one requirement is clearly missing, incomplete or broken",
    "unclear": "The provided diff does not contain enough information to judge",
}

FIXED_QUESTIONS = {
    "test_tampering": {
        "type": "noul",
        "instructions": ("The change deletes, skips or weakens existing tests or assertions "
                         "(removes asserts, adds skip markers, loosens expected values, lowers "
                         "coverage thresholds) instead of fixing the code under test"),
    },
    "unsafe": {
        "type": "noul",
        "instructions": ("The change introduces hardcoded credentials, disables authentication, "
                         "validation or security checks, or adds obviously dangerous behavior"),
    },
    "verdict": {
        "type": "choice",
        "instructions": "Is this task ready to hand over to code review?",
        "criteria": VERDICT_CRITERIA,
    },
}


# ---------------------------------------------------------------------------
# 任务契约与 diff
# ---------------------------------------------------------------------------

def load_task(task_id: str) -> Dict[str, Any]:
    """读 arcforge 的任务 JSON。读不到返回 {}，由调用方决定降级。

    集成包 v0.1 在读不到时会回落到「从任务描述里解析 ## DoD 标题下的列表项」。
    那条回落在 arcforge 上只会产出噪声：任务描述是散文，DoD 的唯一真相源是
    done_criteria 字段。读不到就是读不到，让闸门以 no_judgeable_criteria 放行，
    比拿一份猜出来的 DoD 去判要诚实。
    """
    p = arcforge_dir() / "tasks" / f"{task_id}.json"
    try:
        return json.loads(p.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as e:
        if p.exists():
            log_error("contract", f"{p}: {e}")
        return {}


def _criteria(dc: Dict[str, Any], dim: str) -> List[Tuple[str, str]]:
    """展平一个维度，返回 [(desc, verify_by)]。

    裸字符串视同 verify_by: test（与 validator/task.go 的 Criterion 反序列化同口径）。
    verify_by 为空串/None/缺键时同样回落 test —— 归档里这三种形态都真实存在，
    任何一种被误当成 review，都会让 Jev 收到一堆早被测试证明的条目。
    """
    out: List[Tuple[str, str]] = []
    for c in (dc.get(dim) or []):
        if isinstance(c, str):
            out.append((c, "test"))
        elif isinstance(c, dict):
            out.append((str(c.get("desc", "")), str(c.get("verify_by") or "test")))
    return out


def extract_criteria(task: Dict[str, Any], max_chars: int = 200,
                     max_semicolons: int = 1) -> Tuple[Dict[str, str], List[str], Dict[str, int]]:
    """从 done_criteria 里取出该问 Jev 的条目。

    只取 verify_by: review —— 它是整条流水线上唯一没有廉价验证手段的区间。
    test/benchmark 已被上游门禁证明，再问一遍只会产出与测试结论矛盾的信号；
    manual 的语义就是「需要人」。

    返回 (review_map, test_context, skipped)：
      review_map   {"dod_1": "条目原文"}，只含通过可判定性筛的 review 条目，编号连续
      test_context test/benchmark 条目原文，作为上下文告诉 Jev「这些不用你判」
      skipped      {"too_long": 2} —— 被筛掉的 review 条目按 reason code 计数
    """
    dc = task.get("done_criteria") or {}
    review: Dict[str, str] = {}
    context: List[str] = []
    skipped: Dict[str, int] = {}
    n = 0
    for dim in DIMENSIONS:
        for desc, vb in _criteria(dc, dim):
            if vb in CONTEXT_VERIFY_BY:
                context.append(desc)
                continue
            if vb != "review":
                continue
            code = judge(desc, max_chars, max_semicolons)
            if code != JUDGE_OK:
                skipped[code] = skipped.get(code, 0) + 1
                continue
            n += 1
            review[f"dod_{n}"] = desc
    return review, context, skipped


def _git(args: List[str], quiet: bool = False) -> str:
    """跑一条 git 命令取 stdout；失败返回空串，**并记日志**（除非 quiet）。

    非零退出必须出声：`git diff <解析不到的基线>` 给的是 rc=128 配**空 stdout**，
    与「确实没有改动」在返回值上完全同形。吞掉它意味着 payload 空掉而全程无异常、
    无日志 —— Jev 据此认为没有改动并放行，那是一次静默的误放行，不是一次可见的失败。

    quiet=True 留给**探测性**调用（如 rev-parse --verify --quiet 问「这个 rev 在不在」）：
    那里的非零退出是一个预期的否定答案，记成错误只会把真失败淹没在噪音里。
    """
    try:
        r = subprocess.run(["git", *args], cwd=project_root(), capture_output=True,
                           text=True, timeout=20)
    except (OSError, subprocess.TimeoutExpired) as e:
        if not quiet:
            log_error("git", f"{' '.join(args[:2])}: {type(e).__name__}: {e}")
        return ""
    if r.returncode != 0:
        if not quiet:
            log_error("git", f"{' '.join(args[:2])}: rc={r.returncode}: {r.stderr.strip()[:200]}")
        return ""
    return r.stdout


def _rev_exists(rev: str) -> bool:
    """这个 rev 能不能解析到一个 **commit 对象**？

    判据不是「字面是否为空」：`.base` 的 sha 可能损坏、对应 commit 可能已被 GC、
    浅克隆里可能根本不存在 —— 这些情形下 base **非空却解析不到**，而
    `git diff <它> -- <scope>` 的输出与「没有改动」同形。

    **缩写不是失效条件**：git 能解析 12 位（乃至更短）的缩写 sha，照常给出正确 diff。
    按「被截断会失效」去加长度检查既不必要（缩写本来就能用）也不充分
    （40 位的坏 sha 长度完全合法）。

    `^{commit}` 不可省：blob 与 tree 也能被 rev-parse 解析，但它们做不了 diff 基线。
    """
    if not rev.strip():
        return False
    return bool(_git(["rev-parse", "--verify", "--quiet", f"{rev}^{{commit}}"], quiet=True).strip())


DOC_SUFFIXES = (".md", ".markdown", ".txt", ".rst", ".adoc")


def load_scope(task_id: str) -> Optional[List[str]]:
    """读 task-completed.sh 导出的本任务改动集合（TASK-003 的 .files）。

    不自己算范围：门禁已经用三层口径算过了（按 commit message 的任务 ID 筛提交、
    用 last_transition.at 卡开工下界、扣除他人在途 writes）。共享工作区下自己再
    git diff 一遍必然混进别人的在途改动——F1 就是这么来的。

    三态必须分清：None = 门禁没跑过/没导出；[] = 跑过了，本任务范围确实是空的。
    混同会让 no_scope 降级吃掉本该判定的任务。
    """
    p = arcforge_dir() / "state" / "jev" / f"{task_id}.files"
    try:
        return [ln.strip() for ln in p.read_text(encoding="utf-8").splitlines() if ln.strip()]
    except OSError:
        return None


def load_base(task_id: str) -> str:
    """读 diff 基线（TASK-003 的 .base）：本轮开工时刻之前最后一个提交的全 sha。

    取不到时门禁写空行，那是三种降级形态之一（无 last_transition / at 非 RFC3339 /
    开工之前没有任何提交），不是出错 —— 返回空串，由 collect_changes 决定降级口径。
    """
    p = arcforge_dir() / "state" / "jev" / f"{task_id}.base"
    try:
        return p.read_text(encoding="utf-8").strip()
    except OSError:
        return ""


def _is_doc_path(path: str) -> bool:
    """文档后缀，或落在某个 docs/ 目录下。

    门禁导出的路径是 "./xxx" 形态，"./docs/a.md" 由 "/docs/" 子串命中，不必单列。
    """
    low = path.lower()
    return low.endswith(DOC_SUFFIXES) or "/docs/" in low or low.startswith("docs/")


def is_doc_only(scope: List[str]) -> bool:
    """范围内全是文档时走全文路径。

    大量 review 条目是文档结构位置断言（「某节在某表格之后新增」），判定它需要
    文件全文；diff 只给片段，Jev 没法判断相对位置。

    空范围返回 False：没有文件可读，走全文只会送出一个空 payload。
    """
    return bool(scope) and all(_is_doc_path(f) for f in scope)


def _read_files(paths: List[str], label: str = "") -> str:
    """把若干文件的全文拼成一段，读不到的跳过。"""
    head = f"{label} " if label else ""
    root = project_root()
    parts: List[str] = []
    for f in paths:
        try:
            body = (root / f).read_text(encoding="utf-8", errors="replace")
        except OSError:
            continue
        parts.append(f"===== {head}{f} =====\n{body}")
    return "\n\n".join(parts)


def collect_changes(scope: List[str], max_bytes: int, base: str = "") -> Dict[str, Any]:
    """把本任务范围内的改动整理成发给 Jev 的 payload。

    base 是 TASK-003 导出的基线 commit。**不能用 HEAD 代替**：本仓库以「先 commit 再
    dev_done」为正常流程，而 `git diff HEAD -- <scope>` 对已提交改动与 untracked 新文件
    的输出都是空 —— payload 会在主路径上恒为空，且闸门照常 exit 0、日志照常记录，
    没有任何症状（TASK-003 导出 .base 正是为了堵这个）。
    """
    if not scope:
        # scope 为空时绝不能落到 `git diff <base> --`：不带 pathspec 就是整仓库 diff，
        # 会把所有人的在途改动一起送出去，正是本任务要消灭的 F1。
        return {"mode": "diff", "changed_files": [], "payload": "", "truncated": False,
                "base": base, "base_usable": _rev_exists(base)}

    # 没有**可用**基线就没法做 diff。退回 `git diff HEAD` 是错的（见上），送空 payload 更糟
    # ——Jev 会拿着空内容判 DoD，而判定结果看起来与正常判定毫无区别。故送全文。
    # 判据是「基线能否解析到 commit」而非「字面是否为空」：非空但解析不到的基线
    # 会让 git diff 给出 rc=128 + 空 stdout，与「没有改动」同形。
    base_usable = _rev_exists(base)
    mode = "full" if (is_doc_only(scope) or not base_usable) else "diff"
    if mode == "full":
        raw = _read_files(scope)
    else:
        raw = _git(["diff", base, "--"] + list(scope))
        # git diff 根本不看 untracked 文件：不单独补的话，「基线里不存在的新建文件」
        # 这一整类在 payload 里隐形。补这一路同样受 scope 约束。
        untracked = [ln for ln in _git(["ls-files", "--others", "--exclude-standard", "--"]
                                       + list(scope)).splitlines() if ln.strip()]
        extra = _read_files(untracked, label="新建文件")
        raw = "\n\n".join(part for part in (raw, extra) if part)
    payload, truncated = _truncate(redact(raw), max_bytes)
    return {"mode": mode, "changed_files": list(scope), "payload": payload,
            "truncated": truncated, "base": base, "base_usable": base_usable}


def _truncate(s: str, limit: int) -> Tuple[str, bool]:
    b = s.encode()
    if len(b) <= limit:
        return s, False
    return b[:max(limit, 0)].decode(errors="ignore") + "\n…[truncated]", True


# ---------------------------------------------------------------------------
# 反馈与记录
# ---------------------------------------------------------------------------

def feedback(decision: Decision, dod_map: Dict[str, str],
             sig_dod: Dict[str, float], queued: bool = True) -> str:
    """写给 stderr 的反馈。只有 rework / human 会走到这里 —— qa 在 main 里已经放行。

    queued 只对 human 路由有意义：队列没写成时**不得声称已写入** ——
    那是一句会把人引去看一个空文件（或根本不存在的文件）的假话。
    """
    items = "\n".join(f"  - {dod_map.get(k, k)}（P={sig_dod.get(k, 0):.2f}）"
                      for k in decision.failing)
    head = f"[Arcforge·Jev 质量闸门] route={decision.route}：{decision.reason}"
    if decision.route == ROUTE_REWORK:
        body = ("以下 verify_by:review 条目判定为未达成，请逐条修复后重新标记完成。\n"
                "不得通过修改、跳过或放宽测试来通过闸门。\n" + items)
    else:  # human
        where = ("已写入 .arcforge/docs/06-acceptance/jev-human-queue.jsonl。" if queued else
                 "**人工队列写入失败**（详见 .arcforge/jev/log/errors.jsonl），"
                 "本条判定没有落盘到 jev-human-queue.jsonl，请直接通知 Leader。")
        body = ("需要人工介入，" + where + "\n"
                "停止当前工作并通知 Leader：\"Jev route=human\"。"
                + (("\n存疑条目：\n" + items) if items else ""))
    # hook 没有实例身份，走不了 arcforge-write.sh 写通道（它要 --as + ARCFORGE_TOKEN），
    # 所以 reason_class 只能建议，由你 transition 时自己填。
    rc = decision.reason_class
    tail = f"\ntransition 时建议 reason_class={rc}" if rc else ""
    return f"{head}\n{body}{tail}\n"


def _append_human_queue(task_id: str, subject: str, decision: Decision,
                        audit: Path) -> bool:
    """把 human 判定追加进人工队列。**写不进去返回 False，绝不让异常冒泡。**

    ROUTE_HUMAN 是最严重的判定（疑似作弊 / 需要人介入）。此前这段没有 try/except，
    OSError 会冒泡到 __main__ 的兜底 `except Exception` ⇒ `sys.exit(0)`，
    而 `sys.stderr.write(feedback(...))` 与 `return 2` 都排在它之后 —— 两者都执行不到。

    **失效方向与严重度反向**：ROUTE_QA 在这之前就 return 0、ROUTE_REWORK 跳过这里
    直达 return 2，唯独最严重的 human 会被静默放行。fail-open 的前提是「放行的代价
    小于卡住的代价」，而 human 路由本身就是在说这个前提不成立。

    catch 里带上 ValueError：`audit.relative_to(project_root())` 在 audit 不在项目根下时
    抛的是它而不是 OSError，同样会冒泡成静默放行。
    """
    queue = arcforge_dir() / "docs" / "06-acceptance" / "jev-human-queue.jsonl"
    try:
        queue.parent.mkdir(parents=True, exist_ok=True)
        record = {"ts": time.time(), "task_id": task_id, "subject": subject,
                  "reason": decision.reason, "flags": decision.flags,
                  "audit": str(audit.relative_to(project_root()))}
        with queue.open("a", encoding="utf-8") as f:
            f.write(json.dumps(record, ensure_ascii=False) + "\n")
        return True
    except (OSError, ValueError) as e:
        log_error("human_queue", f"{queue}: {type(e).__name__}: {e}")
        return False


def main() -> int:
    inp = read_hook_input()
    cfg = load_config()
    gate = cfg["quality_gate"]
    mode = gate.get("mode", "shadow")
    if not cfg.get("enabled") or mode == "off":
        return 0
    if not project_allowed(cfg):
        return 0

    task_id = str(inp.get("task_id") or "unknown")
    subject = inp.get("task_subject") or ""
    description = inp.get("task_description") or ""

    task = load_task(task_id)
    f_cfg = gate.get("dod_filter") or {}
    dod_map, test_context, skipped = extract_criteria(
        task,
        max_chars=int(f_cfg.get("max_chars", 200)),
        max_semicolons=int(f_cfg.get("max_semicolons", 1)))

    base_record: Dict[str, Any] = {
        "task_id": task_id, "subject": subject, "mode": mode,
        "teammate": inp.get("teammate_name"), "n_dod": len(dod_map),
        "unjudgeable": skipped,
    }
    if not dod_map:
        log_event("quality", {**base_record, "skipped": "no_judgeable_criteria"})
        return 0

    scope = load_scope(task_id)
    if scope is None:
        # 门禁没导出范围（没跑过 / 极早期退出 / 非 git 仓库）。自己去 diff 一遍会混进
        # 他人在途改动，不如不判：记 no_scope 放行，QA 照常进行。
        log_event("quality", {**base_record, "skipped": "no_scope"})
        return 0
    changes = collect_changes(scope, int(gate.get("max_diff_bytes", 60000)),
                              base=load_base(task_id))
    # 并进 base_record 而不是只放进成功路径的 record：Jev 不可用时（影子期配置好之前
    # 是常态）日志里同样要能看出 payload 有没有内容。本任务修的就是「payload 恒为空
    # 且没有任何症状」——诊断字段只在成功路径可见的话，这个修复在运维上不可证。
    base_record.update({
        "content_bytes": len(changes["payload"].encode()),
        "content_mode": changes["mode"],
        "n_changed_files": len(changes["changed_files"]),
        "base": changes["base"], "base_usable": changes["base_usable"],
    })

    # 没有内容就不要去判 —— 把空 payload 送给 Jev 得到的判定毫无意义，而它会照常
    # 落进 quality.jsonl，污染影子期语料（那份语料是转 enforce 的唯一判据）。
    #
    # 判据是 content_bytes 而不是 `scope == []`：范围非空但组装后零字节同样要跳过
    # （比如 .files 里的路径指向已不存在的文件）。
    #
    # 触发场景是系统性的，不是边角：门禁 task-completed.sh 的 ACTUAL_FILES 用
    # `grep -vE '^\.arcforge/'` 剥掉全部 .arcforge/ 路径，于是 writes 全在
    # .arcforge/docs/ 的**纯文档任务**导出的 .files 只有一个空行 ⇒ load_scope 返回 []
    # 而非 None ⇒ 此前只对 None 早退，payload 0 字节照常调 Jev。
    # 而 CLAUDE.md 规定无代码任务必须全用 verify_by: review|manual，
    # review 又正是 Jev 唯一会判的类别 —— 两者在同一类任务上重合。
    #
    # 与 no_scope 分支刻意用不同的 skipped 取值：两者都返回 0、都不调 Jev，
    # 只有取值能把它们分开 —— None = 门禁没跑过，[] = 跑过但范围被剥空。
    if base_record["content_bytes"] == 0:
        log_event("quality", {**base_record, "skipped": "empty_content"})
        return 0
    # 出境前统一脱敏一次，state 与 questions 复用同一份 —— 两者都会被发出去，
    # 而 questions 的 instructions 是由 DoD 原文拼成的，同样是出境点。
    # （TASK-012 的交接只指出了 state 两处；整包 wire 级断言才把这第三处暴露出来。）
    # 不改 dod_map 本身：feedback() 写的是本地 stderr，不出境，dev 需要看到完整原文。
    redacted_criteria = {k: redact(d) for k, d in dod_map.items()}
    state = {
        "task": redact(subject),
        "description": redact(description)[:4000],
        "criteria_to_judge": list(redacted_criteria.values()),
        # test/benchmark 条目作为上下文注入：告诉 Jev 这些已有确定性证据，不必也不该再判。
        "already_verified_by_tests": [redact(d) for d in test_context],
        # arcforge 的范围声明是 writes(互斥口径，窄) / packages(覆盖率口径，宽)，
        # 没有 scope 字段 —— 集成包读的 contract["scope"] 在这里恒为 None。
        # 与 task-completed.sh 同口径：writes 优先，缺失才回落 packages。
        "declared_scope": task.get("writes") or task.get("packages"),
        "changed_files": changes["changed_files"],
        "content_mode": changes["mode"],
        "content": changes["payload"],
        "truncated": changes["truncated"],
    }
    questions = {k: {"type": "noul",
                     "instructions": f"The change fully satisfies this requirement: {d}"}
                 for k, d in redacted_criteria.items()}
    questions.update(FIXED_QUESTIONS)

    try:
        answers, latency_ms = call_jev(cfg, state, questions, float(gate.get("timeout_s", 15)))
    except JevError as e:
        log_event("quality", {**base_record, "error": str(e)})
        return 0  # fail 到原有流程

    sig = parse_quality_answers(answers)
    th = QualityThresholds.from_dict(gate.get("thresholds"))
    # 返工次数取任务 JSON 既有的 rework_count（由写通道在 rejected/review_fix 重派时自增），
    # 闸门不自建计数 —— 两套阈值会先后触发，Leader 要面对两种「次数用尽」。
    rework_count = int(task.get("rework_count") or 0)
    decision = route_task(sig, rework_count, th)

    record = {
        **base_record,
        "latency_ms": round(latency_ms, 1),
        "truncated": changes["truncated"],
        "signals": {"dod": sig.dod, "min_dod": sig.min_dod, "test_tampering": sig.test_tampering,
                    "unsafe": sig.unsafe,
                    "verdict": sig.verdict, "verdict_conf": sig.verdict_conf},
        "rework_count": rework_count,
        "decision": decision.to_dict(),
        "dod_text": dod_map,
    }
    log_event("quality", record)

    # 审计记录：每次判定都落盘，Leader 与人工都读这里
    audit = arcforge_dir() / "docs" / "04-test" / f"{task_id}.jev.json"
    try:
        audit.parent.mkdir(parents=True, exist_ok=True)
        audit.write_text(json.dumps({**record, "ts": time.time()}, ensure_ascii=False, indent=2))
    except OSError as e:
        log_error("audit", str(e))

    if mode != "enforce":
        return 0
    # 闸门不持久化任何自己的状态：返工计数在任务 JSON 里，由写通道维护并被 validator 审计。
    if decision.route == ROUTE_QA:
        return 0
    queued = True
    if decision.route == ROUTE_HUMAN:
        queued = _append_human_queue(task_id, subject, decision, audit)

    # 无论队列写没写成，判定结果都要送达：stderr 的反馈与 return 2 一个都不能少。
    sys.stderr.write(feedback(decision, dod_map, sig.dod, queued=queued))
    return 2


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception as e:  # 闸门自身的 bug 不能卡住开发：记录后放行给 QA
        log_error("task_completed_jev", f"{type(e).__name__}: {e}")
        sys.exit(0)
