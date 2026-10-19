"""
quiet_decide — 静默期的决策入口,唯一的决策路径。

用法: python3 .claude/hooks/quiet/quiet_decide.py <blocked_human|dod_gate> [<TASK-ID>]
stdout 恒为一行 JSON,退出码恒为 0。调用方**不传任何事实**,入口自己读。

顺序: 配置 → 静默期 → 档位 → 算键 → 取锁 → 读事实 → 硬规则 → 模型前置 → 取意见 → 算 action → 记日志。
硬规则与前置每次都重跑;缓存的只是模型的意见,action 永不从日志重放。
影子档与放权档只在最后一步分叉。stdout 任何档位下都不含模型的意见——意见只进日志、只经简报呈现。
"""
from __future__ import annotations

import sys

sys.dont_write_bytecode = True

import json  # noqa: E402
import os  # noqa: E402
import re  # noqa: E402
import signal  # noqa: E402
import subprocess  # noqa: E402
import time  # noqa: E402
from pathlib import Path  # noqa: E402
from typing import Any, Dict, List, NamedTuple, Optional, Tuple  # noqa: E402

_HERE = str(Path(__file__).resolve().parent)
if _HERE not in sys.path:
    sys.path.insert(0, _HERE)
import quiet_common as qc  # noqa: E402
import quiet_rules as rules  # noqa: E402

deciders = qc.deciders

LOG = "quiet-decisions"
TASK_ID_RE = re.compile(r"^TASK-[A-Za-z0-9_-]{1,24}$")     # 上限 24:任务 ID 在日志行里出现两次(task 与 key)
ACTIONS = {"blocked_human": "redispatch", "dod_gate": "proceed"}
# 题目 id 与正面选项。正面选项只由入口掌握,不进题目、不发给模型。
QUESTION = {"blocked_human": ("retry", "retry"), "dod_gate": ("gate", "proceed")}
MAX_ERRORS = 2          # 同键同晚同模型最多实际调用几次失败
EVIDENCE = "02-plan/dod-gate-evidence.json"      # 相对 .arcforge/docs/
RELEASE = "02-plan/dod-gate-release.json"
REQUIREMENTS_BYTES = 60000          # 发出的需求文档上限(脱敏后截断);与质量闸门 max_diff_bytes 的默认值同值
# 送去脱敏的文本都要有上限:脱敏用的私钥规则在「只有 BEGIN 没有 END」的输入上是二次方的
# (实测 200 KB 约 4–8 秒),而脱敏发生在持锁区间内。
REQUIREMENTS_FILE_BYTES = 200000    # 需求文档文件本身的上限:超过就不读(整份读入再脱敏)
TASKS_BYTES = 200000                # 任务清单上限(脱敏前后都要过);超过不截断而是留到早上——模型不得对着半份验收标准作答
VALIDATOR_TIMEOUT_S = 30            # 校验器在持锁区间内跑,须远小于键锁的破锁阈值
VALIDATOR_REAP_S = 5                # 超时杀掉进程组之后的回收等待上限
MAX_COUNT = 10 ** 6     # 键里两个计数的上限:键在日志行里出现,不设上限就可能把那一行撑过 MAX_LINE


class Ctx(NamedTuple):
    root: Path
    st: qc.Settings
    kind: str
    task_id: Optional[str]
    night: str
    fp: str
    mode: str               # 生效档位
    mismatch: bool
    overridden: bool


# ---------------------------------------------------------------------------
# 事实:入口自己读,调用方不传
# ---------------------------------------------------------------------------

def _valid_id(task_id: Any) -> bool:
    # fullmatch:match 配 `$` 会放过串尾换行。
    return isinstance(task_id, str) and TASK_ID_RE.fullmatch(task_id) is not None


def _count(v: Any) -> bool:
    return isinstance(v, int) and not isinstance(v, bool) and 0 <= v < MAX_COUNT


def _read_task(root: Path, task_id: Optional[str]) -> Dict[str, Any]:
    if not _valid_id(task_id):
        raise qc.FactsError(f"任务 ID 非法: {str(task_id)[:40]!r}")
    path = root / ".arcforge" / "tasks" / f"{task_id}.json"
    try:
        if not path.is_file():      # FIFO 之类会让读取永久阻塞
            raise OSError("不是普通文件")
        task = json.loads(path.read_bytes())
    # ValueError 含 JSONDecodeError 与非 UTF-8 的 UnicodeDecodeError;RecursionError 来自超深嵌套。
    except (OSError, ValueError, RecursionError) as e:
        raise qc.FactsError(f"{task_id}: {type(e).__name__}") from e
    if not isinstance(task, dict):
        raise qc.FactsError(f"{task_id}: 不是对象")
    # 键、日志行与发给模型的内容都取自文件里的字段:它们必须与已校验过的命令行 ID 说的是同一个任务。
    if task.get("id") != task_id:
        raise qc.FactsError(f"{task_id}: 文件里的 id 与文件名不符")
    for field in ("assignment_epoch", "rework_count"):
        if not _count(task.get(field)):
            raise qc.FactsError(f"{task_id}: {field} 不是小的非负整数")
    return task


def run_validator(root: Path) -> Optional[int]:
    """入口自己跑校验器。返回退出码(恒为 int);超时或根本跑不起来返回 None(按不可用处理)。"""
    script = root / ".claude" / "scripts" / "validator-run.sh"
    try:
        if not script.is_file():    # 3.9 上目录不可进入时这里抛 PermissionError 而不是返回 False
            return None
        proc = subprocess.Popen(["bash", str(script), "validate", ".arcforge/tasks"], cwd=str(root),
                                stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                                start_new_session=True)
    except OSError:
        return None
    try:
        return int(proc.wait(timeout=VALIDATOR_TIMEOUT_S))
    except subprocess.TimeoutExpired:
        return None
    finally:
        # 无论超时与否都收拾整个进程组:脚本拉起的构建或后台进程不该在入口返回之后继续跑。
        try:
            os.killpg(proc.pid, signal.SIGKILL)
        except (ProcessLookupError, PermissionError):
            pass
        try:
            proc.wait(timeout=VALIDATOR_REAP_S)
        except subprocess.TimeoutExpired:
            pass


def _read_json(path: Path) -> Any:
    """读一个 JSON 文件。只读就在该路径上的普通文件:FIFO 会让读取永久阻塞,软链会把别处(可以是
    主仓库之外)的内容当成本仓库的事实,进而发给模型。失败统一抛 FactsError。"""
    try:
        if path.is_symlink() or not path.is_file():
            raise OSError("不是就地的普通文件")
        return json.loads(path.read_bytes())
    # ValueError 含 JSONDecodeError 与非 UTF-8 的 UnicodeDecodeError;RecursionError 来自超深嵌套。
    except (OSError, ValueError, RecursionError) as e:
        raise qc.FactsError(f"{path.name}: {type(e).__name__}") from e


def _read_evidence(root: Path) -> Any:
    try:
        return _read_json(root / ".arcforge" / "docs" / EVIDENCE)
    except qc.FactsError:
        return None         # 交给硬规则判 evidence_invalid,不算「事实读不到」


def _requirements_file(root: Path, rel: str) -> Path:
    try:
        base = root.resolve()
        path = (base / rel).resolve()
        inside = base in path.parents
        usable = inside and path.is_file() and path.stat().st_size <= REQUIREMENTS_FILE_BYTES
    except (OSError, ValueError, RuntimeError) as e:     # ValueError:路径里有 NUL;RuntimeError:3.9 的软链环
        raise qc.FactsError("requirements_path 不可用") from e
    if not inside:
        raise qc.FactsError("requirements_path 越出主仓库")
    if not usable:                          # 不存在、目录、FIFO,或大到不该整份读
        raise qc.FactsError("requirements_path 读不到或过大")
    return path


def _task_list(root: Path) -> List[Dict[str, Any]]:
    """全部任务的 {id, title, done_criteria},按文件名升序——与 qc.dod_hash 取的是同一批文件。"""
    tasks = []
    for f in sorted((root / ".arcforge" / "tasks").glob("TASK-*.json")):
        t = _read_json(f)
        if not isinstance(t, dict):
            raise qc.FactsError(f"{f.name}: 不是对象")
        tasks.append({"id": t.get("id"), "title": t.get("title"), "done_criteria": t.get("done_criteria")})
    return tasks


def _bounded_tasks(task_list: List[Dict[str, Any]]) -> str:
    """脱敏后的任务清单文本。超过 TASKS_BYTES 不截断而是抛 FactsError:模型不得对着半份验收标准作答。
    脱敏前先量一次——脱敏会把私钥块之类缩成一个标记,但送去脱敏的输入本身必须有界。"""
    try:
        raw = "\n".join(_flat(task_list))
    except RecursionError as e:
        raise qc.FactsError("任务内容嵌套过深") from e
    if len(raw.encode("utf-8", errors="replace")) > TASKS_BYTES:
        raise qc.FactsError("任务清单超过上限")
    text = _clean(raw)
    if len(text.encode("utf-8")) > TASKS_BYTES:
        raise qc.FactsError("任务清单超过上限")
    return text


def _read_dod_gate(root: Path, st: qc.Settings) -> Dict[str, Any]:
    evidence = _read_evidence(root)
    rel = evidence.get("requirements_path") if isinstance(evidence, dict) else None
    if isinstance(rel, str) and rel:
        # 在读事实这一步就核对,而不是等到拼题目时:命中缓存不拼题目,执行器也只走到这里。
        _requirements_file(root, rel)
    # 先读任务清单、后算哈希:调用方会拿这里的哈希与取锁前算出的键比对,于是「模型看到的任务内容」
    # 被前后两次哈希夹在中间——中途有人改了任务文件,键就对不上。
    tasks = _bounded_tasks(_task_list(root))
    rc = run_validator(root)
    return {"autonomy": st.config.get("autonomy", "dod-gate"), "evidence": evidence,
            "dod_sha256": qc.dod_hash(root),
            # 硬规则层对「不是 int 的 0」(False、0.0)是放行方向,这里只放行真正的 int。
            "validator_rc": rc if type(rc) is int else None,
            "tasks": tasks}         # 已脱敏的文本;硬规则不看它,只给 build() 用


def key_of(kind: str, root: Path, task_id: Optional[str]) -> str:
    """只算键,不做昂贵的事(校验器在取锁之后才跑)。"""
    if kind == "blocked_human":
        return facts_key(kind, _read_task(root, task_id))
    if kind == "dod_gate":
        _task_list(root)        # qc.dod_hash 自己不查文件类型:任务目录里有 FIFO 时它会在取锁之前就挂住
        return qc.dod_hash(root)
    raise qc.FactsError(f"未知的 kind: {kind!r}")


def read_facts(kind: str, root: Path, task_id: Optional[str], st: qc.Settings) -> Dict[str, Any]:
    if kind == "blocked_human":
        return _read_task(root, task_id)
    if kind == "dod_gate":
        return _read_dod_gate(root, st)
    raise qc.FactsError(f"未知的 kind: {kind!r}")


def facts_key(kind: str, facts: Dict[str, Any]) -> str:
    if kind == "blocked_human":
        return f"{facts.get('id')}:{facts.get('assignment_epoch')}:{facts.get('rework_count')}"
    if kind == "dod_gate":
        return str(facts.get("dod_sha256"))
    raise qc.FactsError(f"未知的 kind: {kind!r}")


def hit_rule(kind: str, facts: Dict[str, Any]) -> Optional[str]:
    if kind == "blocked_human":
        return rules.blocked_human(facts)
    if kind == "dod_gate":
        return rules.dod_gate(facts)
    raise qc.FactsError(f"未知的 kind: {kind!r}")


# ---------------------------------------------------------------------------
# 题目与出境内容
# ---------------------------------------------------------------------------

# 键名像密钥的,整棵子树不发。摊平会把嵌套的值换到下一行,「关键字 + 分隔符 + 值」就此被拆开,
# 脱敏规则要么认不出、要么遮错地方;在键这一层直接处理不依赖值长什么样。比脱敏规则的关键字表多收几个常见写法。
_SECRET_KEY = re.compile(r"(?i)(api[_-]?key|secret|token|passw(or)?d|pwd|passphrase|private[_-]?key|access[_-]?key"
                         r"|credential|authorization)")
_WITHHELD = "«REDACTED»"


def _flat(value: Any, pad: str = "") -> List[str]:
    """把结构化的值摊成「键: 值」的文本行,字符串原样不加引号、不转义。

    脱敏规则认的是「关键字 + 分隔符 + 值」;先 json.dumps 的话,转义出来的反斜杠与键值之间的引号会让它对不上。
    """
    if not isinstance(value, (dict, list)) or not value:
        return [pad + (value if isinstance(value, str) else json.dumps(value, ensure_ascii=False))]
    items = [(f"{k}: ", v) for k, v in value.items()] if isinstance(value, dict) else [("- ", v) for v in value]
    lines: List[str] = []
    for head, v in items:
        if isinstance(value, dict) and _SECRET_KEY.search(head):
            lines.append(pad + head + _WITHHELD)
        elif isinstance(v, (dict, list)) and v:
            lines.append(pad + head.rstrip())
            lines.extend(_flat(v, pad + "  "))
        else:
            lines.extend(pad + head + line for line in _flat(v))
    return lines


def _clean(value: Any) -> str:
    """摊平并脱敏。孤立代理先换掉:它编不成 UTF-8,会让适配器在发出前就失败并被记成模型的错。"""
    return qc.redact("\n".join(_flat(value)).encode("utf-8", errors="replace").decode("utf-8"))


def _text(value: Any, limit: int) -> str:
    """脱敏后再截断。"""
    return _clean(value)[:limit]


REPORT_TAIL_BYTES = 65536   # 只读报告末尾这么多:读取量有上限(这一步在持锁区间内)


def _verification(root: Path, task_id: str) -> str:
    """验证报告的末段(结论在报告末尾);读不到返回空串。
    顺序是:**先按字节取文件末尾 REPORT_TAIL_BYTES,再脱敏,再留最后 4000 个字符。**

    task_id 须是已校验过的那个,不取文件内容里的 id。只读恰在该路径上的普通文件:软链会把别的文件
    (仓库外的,或仓库内的 .git/config)原样发给模型,FIFO 会让读取永久阻塞。

    **第一刀在脱敏之前。** 报告超过 REPORT_TAIL_BYTES 时,读取窗口的起点可能把一段密文切开,把规则赖以
    认出它的那部分留在窗口之外。实测过两类:私钥块的 BEGIN 行留在窗口之外,键值对的键名与分隔符留在
    窗口之外——窗口里剩下的块体、剩下的值都没有被遮蔽(下称残片)。残片内部另有规则认得出的内容时,
    那一段照常遮蔽,其余原样。
    **残片之后的内容脱敏后不足 4000 个字符时,脱敏后的残片会进入返回值**——差多少个字符,它的末尾就带出多少;
    差得比它还长时,全部带出。下面两种情形都会让它不足:
      (甲) 残片自己一直延伸到报告末尾附近,它后面本来就没有多少内容;
      (乙) 残片之后的内容大部分会被脱敏整块遮掉(例如一连串完整的私钥块),原文再长,遮完也所剩无几。
    残片之后的内容脱敏后达到 4000 个字符时不带出;BEGIN 行或键名也在窗口之内时照常遮蔽。
    以上各句均有实测:私钥块与键值对两类残片,(甲)(乙)两种情形,残片内部夹着可识别内容的一格,以及两条不带出的对照。
    """
    try:
        base = root.resolve()
        rel = Path(".arcforge") / "docs" / "04-test" / f"{task_id}-verification.md"
        path = base / rel
        if path.resolve() != path or not path.is_file():
            return ""
        with open(path, "rb") as f:
            f.seek(max(0, path.stat().st_size - REPORT_TAIL_BYTES))
            data = f.read(REPORT_TAIL_BYTES)
    except OSError:
        return ""
    return qc.redact(data.decode("utf-8", errors="replace"))[-4000:]


def _requirements(root: Path, rel: str) -> str:
    """脱敏后的需求文档,截到 REQUIREMENTS_BYTES 字节。整份读入、先脱敏后截断(文件大小已在
    _requirements_file 里设了上限):只读前一段再脱敏的话,读取边界上会切出认不出的密钥残片。"""
    try:
        data = _requirements_file(root, rel).read_bytes()
    except OSError as e:
        raise qc.FactsError("requirements_path 读不到") from e
    # 带 NUL 的不是 UTF-8 文本(多半是 UTF-16):按 UTF-8 解出来字符之间夹着 NUL,脱敏规则会全部失配。
    if b"\x00" in data or not data.strip():
        raise qc.FactsError("需求文档为空或不是文本")
    text = qc.redact(data.decode("utf-8", errors="replace"))
    return text.encode("utf-8")[:REQUIREMENTS_BYTES].decode("utf-8", errors="ignore")


def _questions(kind: str) -> Dict[str, Any]:
    """题目只随 kind 变,不含任何事实;缓存行的校验也用它(与模型现场作答过同一道校验)。"""
    if kind == "dod_gate":
        return {"gate": {
            "type": "choice",
            "instructions": ("Are these acceptance criteria sufficient to start unattended development "
                             "of the requirements below?"),
            "criteria": {
                "proceed": "The criteria are specific, testable, and correspond to the requirements item by item",
                "hold": "Some criteria are vague, untestable, or do not match the requirements",
                "unclear": "The provided information is not enough to judge",
            },
        }}
    if kind != "blocked_human":
        raise qc.FactsError(f"未知的 kind: {kind!r}")
    return {"retry": {
        "type": "choice",
        "instructions": ("This task exhausted its rework budget. Would one more rework round by a developer "
                         "have a reasonable chance of satisfying the acceptance criteria?"),
        "criteria": {
            "retry": "The last failure is specific, fixable, and within the task's declared scope",
            "stop": ("The failure stems from the requirements or acceptance criteria themselves, "
                     "or successive rounds failed for the same reason"),
            "unclear": "The provided information is not enough to judge",
        },
    }}


def build(kind: str, root: Path, facts: Dict[str, Any], st: qc.Settings) -> Tuple[Dict[str, Any], Dict[str, Any]]:
    """(questions, state)。state 在这里脱敏完毕,适配器拿不到未脱敏的内容。"""
    if kind == "dod_gate":
        # 任务清单用 read_facts 读到的那一份(已脱敏、已过上限),不在这里重读:它必须与键对应的是同一份内容。
        return _questions(kind), {"requirements": _requirements(root, facts["evidence"]["requirements_path"]),
                                  "tasks": facts["tasks"]}
    state = {
        "task": _text(facts.get("title") or facts.get("id") or "", 500),
        "reject_reason": _text(facts.get("reject_reason") or "", 4000),
        "verification": _verification(root, facts["id"]),      # facts["id"] 已在 _read_task 里与命令行 ID 核对过
        "done_criteria": _text(facts.get("done_criteria") or {}, 8000),
        "rework_count": facts.get("rework_count"),
        "max_rework": st.config.get("max_rework") if _count(st.config.get("max_rework")) else None,
    }
    return _questions(kind), state


# ---------------------------------------------------------------------------
# 记日志并返回:先落盘,后输出
# ---------------------------------------------------------------------------

def _emit(ctx: Ctx, key: Optional[str], action: str, hold_reason: Optional[str] = None,
          hard_rule: Optional[str] = None, opinion: Optional[Dict[str, Any]] = None,
          latency_ms: Optional[float] = None) -> Dict[str, Any]:
    decision_id = f"{ctx.night}:{ctx.kind}:{qc.sha1(key or '-')[:12]}:{qc.line_count(ctx.root, LOG)}"
    record: Dict[str, Any] = {
        "id": decision_id, "ts": round(time.time(), 1), "night": ctx.night, "type": "decision",
        "kind": ctx.kind, "key": key,
        # 没通过校验的 ID 不进日志:它可以任意长,会把这一行撑到写不下来。
        "task": ctx.task_id if ctx.kind == "blocked_human" and _valid_id(ctx.task_id) else None,
        "mode": ctx.mode, "decider": {"name": ctx.st.decider, "fp": ctx.fp}, "hard_rule": hard_rule,
    }
    if opinion is not None:
        record["opinion"] = opinion
    record["action"] = action
    if hold_reason:
        record["hold_reason"] = hold_reason
    if ctx.overridden:
        record["clock"] = "override"
    if latency_ms is not None:
        record["latency_ms"] = round(latency_ms, 1)
    try:
        qc.append(ctx.root, LOG, record)
    except qc.LogError as e:
        # 记不下来的决策不执行:早上的简报里看不到它。
        qc.log_error(ctx.root, "quiet_log", str(e))
        return {"action": "hold", "kind": ctx.kind, "key": key}
    return {"action": action, "kind": ctx.kind, "key": key, "decision_id": decision_id}


def _same(ctx: Ctx, key: str) -> List[Dict[str, Any]]:
    """同键、同一晚、同一决策模型(名称与指纹)的既有记录;被覆写时钟的记录不算。"""
    me = {"name": ctx.st.decider, "fp": ctx.fp}
    return [r for r in qc.read(ctx.root, LOG)
            if r.get("type") == "decision" and r.get("kind") == ctx.kind
            and r.get("key") == key and r.get("night") == ctx.night
            and r.get("decider") == me and "clock" not in r]


def _locked(ctx: Ctx, key: str) -> Dict[str, Any]:
    try:
        facts = read_facts(ctx.kind, ctx.root, ctx.task_id, ctx.st)
        if facts_key(ctx.kind, facts) != key:
            raise qc.FactsError("取锁期间事实变了")
    except (qc.FactsError, RecursionError):
        return _emit(ctx, key, "hold", "facts_unreadable")

    rule = hit_rule(ctx.kind, facts)
    if rule:
        reason = rule if rule == rules.VALIDATOR_UNAVAILABLE else "hard_rule"
        return _emit(ctx, key, "hold", reason, hard_rule=rule)

    # 前置必须先于 ask():出境白名单与端点覆写的拦截只在 available() 里,ask() 自己不查。
    try:
        why = deciders.available(ctx.st.decider, ctx.st.config, ctx.root)
    except Exception as e:  # noqa: BLE001 —— 前置自己出错 = 不可用
        qc.log_error(ctx.root, "quiet_decider", f"available: {type(e).__name__}: {e}")
        why = "decider_unavailable"
    if why:
        return _emit(ctx, key, "hold", why)

    qid, positive = QUESTION[ctx.kind]
    same = _same(ctx, key)
    opinion: Optional[Dict[str, Any]] = None
    latency_ms: Optional[float] = None
    for row in reversed(same):
        # 日志行过与现场作答相同的校验;不合格的行当它不存在,既不复用也不让入口崩掉。
        try:
            cached = deciders.validate_answers(_questions(ctx.kind), {qid: row.get("opinion")}).answers.get(qid)
        except Exception:  # noqa: BLE001 —— 例:超大整数的置信度让 isfinite 溢出
            cached = None
        if cached is not None:
            opinion = {"choice": cached["choice"], "confidence": cached["confidence"], "cached": True}
            break
    if opinion is None:
        if sum(1 for r in same if r.get("hold_reason") == "decider_error" and "latency_ms" in r) >= MAX_ERRORS:
            return _emit(ctx, key, "hold", "decider_error")
        try:
            questions, state = build(ctx.kind, ctx.root, facts, ctx.st)
        except (qc.FactsError, RecursionError):
            return _emit(ctx, key, "hold", "facts_unreadable")
        t0 = time.monotonic()
        answer = None
        try:
            result = deciders.ask(ctx.st.decider, ctx.st.config, ctx.root, f"quiet.{ctx.kind}", questions, state)
            answer = result.answers.get(qid)
            failure = f"{qid}: {result.invalid.get(qid, 'unanswered')}"     # 没作答,或作答形状不对落入 invalid
        except deciders.DeciderError as e:
            failure = str(e)
        except Exception as e:  # noqa: BLE001 —— 适配器自己崩了与整次调用失败同样处理,同样占预算
            failure = f"{type(e).__name__}: {e}"
        latency_ms = (time.monotonic() - t0) * 1000
        if answer is None:
            qc.log_error(ctx.root, "quiet_decider", failure)
            return _emit(ctx, key, "hold", "decider_error", latency_ms=latency_ms)
        opinion = {"choice": answer["choice"], "confidence": answer["confidence"], "cached": False}

    if opinion["confidence"] < ctx.st.min_confidence:
        return _emit(ctx, key, "hold", "low_confidence", opinion=opinion, latency_ms=latency_ms)
    if opinion["choice"] != positive:
        return _emit(ctx, key, "hold", "declined", opinion=opinion, latency_ms=latency_ms)
    if ctx.overridden:
        return _emit(ctx, key, "hold", "clock_override", opinion=opinion, latency_ms=latency_ms)
    if ctx.mode != "enforce":       # 影子档与放权档的唯一分叉
        reason = "fp_mismatch" if ctx.mismatch else "shadow"
        return _emit(ctx, key, "hold", reason, opinion=opinion, latency_ms=latency_ms)
    return _emit(ctx, key, ACTIONS[ctx.kind], opinion=opinion, latency_ms=latency_ms)


def decide(kind: str, task_id: Optional[str] = None, cwd: Optional[Path] = None) -> Dict[str, Any]:
    human = {"action": "human", "kind": kind}
    blind = {"action": "hold", "kind": kind}        # 连日志该写到哪、该记成哪一晚都不知道时的 hold
    if not isinstance(kind, str) or kind not in ACTIONS:
        return blind
    try:
        root = qc.main_root(cwd)
    except (qc.RootError, OSError):                 # OSError:当前目录已被删除时 os.getcwd() 抛的
        return blind
    try:
        st = qc.load(root)
    except (qc.ConfigError, RecursionError) as e:
        qc.log_error(root, "quiet_config", f"{type(e).__name__}: {e}")
        return human
    if st is None or not st.enabled:
        return human
    try:
        at, overridden = qc.now()
        night = qc.window(st, at)
    except (OverflowError, ValueError):             # 覆写出来的时刻换不到配置的时区(公元 1 年、9999 年)
        return blind
    if night is None:
        return human

    try:
        fp = deciders.fingerprint(st.decider, st.config)
    except Exception as e:  # noqa: BLE001 —— 指纹算不出来 = 不知道问的是谁,按配置非法处理
        qc.log_error(root, "quiet_config", f"fingerprint: {type(e).__name__}: {e}")
        return human
    mode, mismatch = qc.effective_mode(st, kind, fp)
    ctx = Ctx(root, st, kind, task_id, night, fp, mode, mismatch, overridden)
    if mode == "off":
        return _emit(ctx, None, "hold", "mode_off")
    try:
        key = key_of(kind, root, task_id)
    except (qc.FactsError, RecursionError):     # RecursionError:任务内容嵌套过深
        return _emit(ctx, None, "hold", "facts_unreadable")
    lock = qc.acquire(root, key)
    if lock is None:        # 并发的同键调用得到 hold,而不是排队等结果
        return _emit(ctx, key, "hold", "lock_timeout")
    try:
        return _locked(ctx, key)
    finally:
        qc.release(lock)


def main(argv: List[str]) -> int:
    try:
        ok = (len(argv) == 2 and argv[0] == "blocked_human") or argv == ["dod_gate"]
        if not ok:
            print("用法: quiet_decide.py <blocked_human <TASK-ID> | dod_gate>", file=sys.stderr)
            out: Dict[str, Any] = {"action": "hold"}
        else:
            out = decide(argv[0], argv[1] if len(argv) == 2 else None)
    except Exception as e:  # noqa: BLE001 —— 顶层兜底:异常与正常 hold 对调用方同形
        print(f"quiet_decide 异常,按 hold 处理: {type(e).__name__}: {e}", file=sys.stderr)
        out = {"action": "hold"}
    print(json.dumps(out, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
