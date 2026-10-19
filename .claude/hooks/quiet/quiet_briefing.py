"""
quiet_briefing — 静默期的旁路记录与次日简报。只读决策日志,不做任何决策。

用法:
  quiet_briefing.py status                          # JSON:是否开启、是否在静默期、待呈现的夜晚、确认门是否待决、指纹
  quiet_briefing.py heartbeat                       # 夜间周期扫描每轮调一次
  quiet_briefing.py record-human <decision_id> <决定>
  quiet_briefing.py presented --night YYYY-MM-DD
  quiet_briefing.py render --night YYYY-MM-DD       # Markdown 输出到 stdout,由 Leader 经写通道落盘

「是否已落地」不信日志自报:一个键算已落地,当且仅当它有一条放权档下的正面决策行,且磁盘上的落地标记
(任务文件里的 auto_redispatch / dod-gate-release.json)的 decision_id 与这条决策逐字相等。applied 行只用来
回答「日志里记没记这次落地」,**不作为已落地的依据**——决策日志任何实例都能追加,落地标记要经写通道才写得上。
执行器在写通道没给出 0 时一律报「未落地」且不写 applied 行,写入其实已经完成的那种现场同样靠标记认出来。
"""
from __future__ import annotations

import sys

sys.dont_write_bytecode = True

import json  # noqa: E402
import re  # noqa: E402
import time  # noqa: E402
import unicodedata  # noqa: E402
from collections import Counter  # noqa: E402
from datetime import date, datetime, timedelta, timezone  # noqa: E402
from pathlib import Path  # noqa: E402
from typing import Any, Dict, List, Optional, Tuple  # noqa: E402

_HERE = str(Path(__file__).resolve().parent)
if _HERE not in sys.path:
    sys.path.insert(0, _HERE)
import quiet_common as qc  # noqa: E402
import quiet_decide as qd  # noqa: E402

deciders = qc.deciders

ACTIVITY = "quiet-activity"
PERMISSIONS = "quiet-permissions"
NIGHT_RE = re.compile(r"^\d{4}-\d{2}-\d{2}$")       # 须配 fullmatch:match 加 `$` 会放过串尾换行
# 人的决定的合法取值,按类别固定。一致率拿它与 ACTIONS 比:写成模型的用词(如 retry)会被静默算作不一致,
# 所以不认识的词直接拒绝。
HUMAN_DECISIONS = {"blocked_human": ("redispatch", "skip", "keep"), "dod_gate": ("proceed", "hold")}
KIND_LABEL = {"blocked_human": "熔断", "dod_gate": "DoD 确认门"}
HOLD_LABEL = {
    "shadow": "影子档（只记录）", "mode_off": "该类已关闭", "hard_rule": "命中硬规则",
    "facts_unreadable": "事实读不到", "decider_error": "决策模型调用失败", "low_confidence": "置信度不足",
    "declined": "决策模型不建议", "decider_unavailable": "决策模型不可用",
    "project_not_allowed": "项目不在出境白名单", "validator_unavailable": "校验器不可用，该项目无法委托 DoD 确认门",
    "lock_timeout": "并发占用", "clock_override": "测试时钟", "fp_mismatch": "放权指纹不符，已按影子档处理",
}
# 这些留下原因意味着「模型本来会同意」——影子对比与一致率据此判断模型的倾向。
WOULD_ACT = ("shadow", "fp_mismatch", "clock_override")
# 取自文件的文本(任务 ID、状态、指派对象、模型名、命令首词、where、decision_id)进简报前的上限,按字符计。
# 它们不受控,而简报是给 Leader 读、由 Leader 呈现的:一个换行加一句指令性文字不该能在简报里独占一行。
FIELD_MAX = 80
LINE_MAX = 1000
LIST_MAX = 12       # 名单至多列这么多个名字,其余只报总数(否则会被行长上限从中间截断)
_BREAKS = re.compile(r"[\x00-\x1f\x7f\x85\u2028\u2029]+")
INTERNAL = "内部错误: "       # status 的 error:配置非法保持 ConfigError 的原文,其余以它开头


class RenderError(Exception):
    pass


# ---------------------------------------------------------------------------
# 读文件与清洗
# ---------------------------------------------------------------------------

def _t(value: Any, limit: int = FIELD_MAX) -> str:
    """一段取自文件的文本:去掉换行、控制字符与不可见的格式字符,限长。凡是这类文本进简报之前都过这里。

    反引号换成撇号:简报自己把状态、指派对象、decision_id 放在行内代码里,值里的反引号会把它提前闭合。
    """
    out = []
    for c in value if isinstance(value, str) else str(value):
        cat = unicodedata.category(c)
        if cat in ("Cf", "Cs"):         # 方向控制符、零宽字符、孤立代理:直接去掉
            continue
        out.append(" " if cat in ("Cc", "Zl", "Zp") else "'" if c == "`" else c)
    s = " ".join("".join(out).split())
    return s if len(s) <= limit else s[:limit - 1] + "…"


def _names(items: List[str]) -> str:
    """一份名单:至多列 LIST_MAX 个,超出时在末尾给出总数——名单再长,也不能让人以为只有列出来的这些。"""
    shown = "、".join(items[:LIST_MAX])
    return shown if len(items) <= LIST_MAX else f"{shown} 等，共 {len(items)} 个"


def _one_line(text: str) -> str:
    """简报的每个条目恰占一行:这是全部条目进入 Markdown 之前的最后一道。"""
    s = _BREAKS.sub(" ", text)
    return s if len(s) <= LINE_MAX else s[:LINE_MAX - 1] + "…"


def _json(path: Path, any_depth: bool = False) -> Any:
    """读一个 JSON 文件;读不出来(不存在、不是普通文件、非 UTF-8、非 JSON、嵌套过深)返回 None。

    「嵌套过深」与 qc.read 同一个上限:有的解释器在较浅的深度上解析得出来,之后把它转成文本时才抛
    RecursionError——那样一个任务文件或放行文件里的一个字段就能让简报生成失败。
    any_depth 只给「取出来的字段只做相等比较、绝不转成文本」的调用点用。

    **返回 None 有两种意思:文件不存在,或文件在而读不出来。** 调用点若据 None 跳过一条提示,就得先用
    _unreadable() 分清是哪一种——读不出来的文件不该让本来会打出的提示无声消失。
    """
    return _load(path, any_depth)[1]


def _load(path: Path, any_depth: bool = False) -> Tuple[str, Any]:
    """(状态, 值)。状态: "missing" 文件不存在或不是普通文件;"broken" 读取出错或不是 JSON;"deep" 嵌套过深
    (深到解析不了,或解析得出来但超过上限);"ok" 解析出了一个值(未必是对象)。只有 "ok" 带值。"""
    try:
        if not path.is_file():      # FIFO 会让读取永久阻塞
            return "missing", None
        obj = json.loads(path.read_bytes())
    except RecursionError:
        return "deep", None
    except (OSError, ValueError):
        return "broken", None
    if not any_depth and qc._deeper_than(obj, qc.MAX_DEPTH):
        return "deep", None
    return "ok", obj


def _unreadable(path: Path) -> bool:
    """这个文件在,却读不出一个 JSON 对象:坏 JSON、嵌套过深,或解析出来的不是对象。"""
    state, obj = _load(path)
    return state in ("broken", "deep") or (state == "ok" and not isinstance(obj, dict))


def _too_deep(path: Path) -> bool:
    """这个文件在,只因为嵌套过深才读不出来——它本身可能是一份内容完好的文件(深的只是多出来的某个字段)。"""
    return _load(path)[0] == "deep"


def _bad_lines(path: Path) -> int:
    """一个 jsonl 文件里读不出来的行数(坏 JSON、不是对象、嵌套过深);空行不算;文件不存在为 0。
    _rows 把这些行跳过了:用到它的地方要把这个数说出来,不然被跳过的行里有什么都无人知晓。"""
    try:
        if not path.is_file():
            return 0
        data = path.read_bytes()
    except OSError:
        return 0
    bad = 0
    for line in data.split(b"\n"):
        if not line.strip():
            continue
        try:
            obj = json.loads(line)
        except (ValueError, RecursionError):
            bad += 1
            continue
        bad += not (isinstance(obj, dict) and not qc._deeper_than(obj, qc.MAX_DEPTH))
    return bad


def _rows(path: Path) -> Optional[List[Dict[str, Any]]]:
    """一个 jsonl 文件里的对象行,坏行跳过;文件不存在返回 None。只以 \\n 分行、逐行解码。"""
    try:
        if not path.is_file():
            return None
        data = path.read_bytes()
    except OSError:
        return None
    rows = []
    for line in data.split(b"\n"):
        try:
            obj = json.loads(line)
        except (ValueError, RecursionError):
            continue
        if isinstance(obj, dict) and not qc._deeper_than(obj, qc.MAX_DEPTH):    # 过深的行是坏行,同 qc.read
            rows.append(obj)
    return rows


def _docs(root: Path) -> Path:
    return root / ".arcforge" / "docs"


def _task(root: Path, task_id: Any) -> Dict[str, Any]:
    """一个任务文件的内容。ID 形状不对(它取自日志,要拼进路径,先校验)或读不出一个对象时返回空字典。"""
    if not isinstance(task_id, str) or not qd.TASK_ID_RE.fullmatch(task_id):
        return {}
    task = _json(root / ".arcforge" / "tasks" / f"{task_id}.json")
    return task if isinstance(task, dict) else {}


def _dispatched(root: Path) -> Tuple[int, int]:
    """(已派发的任务数, 读不出来的任务文件数)。已派发 = 有 status 且不是 pending;读不出来的不计入前者,
    所以要把后者一并报出来——否则派发数少了也看不出来。"""
    count = unreadable = 0
    for f in (root / ".arcforge" / "tasks").glob("TASK-*.json"):
        task = _json(f)
        if isinstance(task, dict):
            count += task.get("status") not in (None, "pending")
        else:
            unreadable += _unreadable(f)
    return count, unreadable


def _valid_night(night: Any) -> bool:
    if not isinstance(night, str) or not NIGHT_RE.fullmatch(night):
        return False
    try:
        date.fromisoformat(night)
    except ValueError:
        return False
    return True


def _number(v: Any) -> bool:
    """一个能当浮点数用的有限数值(bool、NaN、无穷、大到转不成浮点的整数都不算)。"""
    if not isinstance(v, (int, float)) or isinstance(v, bool):
        return False
    try:
        f = float(v)
    except OverflowError:
        return False
    return f == f and abs(f) != float("inf")


def _clock(ts: Any, tz: Any) -> Optional[str]:
    """时间戳在给定时区的 HH:MM;换算不了(过大、过小)返回 None。"""
    try:
        return datetime.fromtimestamp(float(ts), tz).strftime("%H:%M") if _number(ts) else None
    except (OverflowError, OSError, ValueError):
        return None


# ---------------------------------------------------------------------------
# status
# ---------------------------------------------------------------------------

def _window_only(root: Path) -> Optional[qc.Settings]:
    """配置非法时尽量只取出窗口(起止时刻与时区),别的字段一概不看;连窗口也取不出来返回 None。
    **只供判断某一晚是否已经结束**:其余字段是占位值,不得拿去做任何别的事。"""
    try:
        config = json.loads((root / qc.CONFIG_NAME).read_text(encoding="utf-8"))
        q = config["quiet_hours"]
        start, end = qc._minutes(q.get("start"), "start"), qc._minutes(q.get("end"), "end")
        if start == end:
            return None
        return qc.Settings(config=config, enabled=False, start=start, end=end, tz=qc.ZoneInfo(q["timezone"]),
                           decider="", min_confidence=1.0, delegate={}, enforce_fp=None)
    except Exception:  # noqa: BLE001 —— 读不出来的原因不重要,调用方退到与配置无关的判据
        return None


def _ended(st: Optional[qc.Settings], night: str, at: datetime) -> bool:
    """某一晚是否已经结束。窗口读不出来(st 为 None)时退到与配置无关的判据:该日期之后第 3 天的 UTC 零点已过。
    窗口至多跨到次日、时区偏移至多 14 小时,到那一刻无论原来怎么配都必然已经结束——宁可晚列,不把没结束的夜晚列出来。"""
    if st is not None:
        return qc.night_ended(st, night, at)
    d = date.fromisoformat(night) + timedelta(days=3)
    return at >= datetime(d.year, d.month, d.day, tzinfo=timezone.utc)


def _pending_nights(root: Path, st: Optional[qc.Settings], at: datetime) -> List[str]:
    """已经结束、且还有东西没呈现的夜晚:从未呈现过;或者呈现之后该晚又出现了决策行或落地行。

    后一条堵的是「先记一行已呈现,之后的决策就再也没人看」。时间读不出来的行(呈现行或决策行)一律按
    「决策在呈现之后」算:证明不了它已经被呈现过。呈现行的时间晚于此刻(真实时钟,留一分钟余量)同样按
    读不出来算——写着将来时刻的一行能把之后的决策全都盖住。
    """
    log, activity = qc.read(root, qd.LOG), qc.read(root, ACTIVITY)
    seen = {r.get("night") for r in log + activity if _valid_night(r.get("night"))}
    never, latest = float("-inf"), time.time() + 60
    shown: Dict[str, float] = {}        # 夜晚 → 最后一条 presented 行的时间
    for r in activity:
        if r.get("type") == "presented" and _valid_night(r.get("night")):
            ts = float(r["ts"]) if _number(r.get("ts")) and r["ts"] <= latest else never
            shown[r["night"]] = max(shown.get(r["night"], never), ts)
    fresh = {r["night"] for r in log
             if r.get("type") in ("decision", "applied") and _valid_night(r.get("night")) and r["night"] in shown
             and (not _number(r.get("ts")) or r["ts"] > shown[r["night"]])}
    pending = []
    for n in sorted((seen - set(shown)) | fresh):
        try:
            if _ended(st, n, at):
                pending.append(n)
        except (OverflowError, ValueError):     # 9999-12-31 这类日期算不出结束时刻:跳过它,别的夜晚照列
            continue
    return pending


def _release_matches(root: Path, current: Any) -> bool:
    # 只拿 dod_sha256 去比,不转成文本:不设深度上限(同证据文件),免得多一个很深的字段就把已放行判成没放行。
    release = _json(_docs(root) / qd.RELEASE, any_depth=True)
    return isinstance(current, str) and isinstance(release, dict) and release.get("dod_sha256") == current


def _current_dod(root: Path) -> Optional[str]:
    """当前 DoD 的哈希;算不出来(没有任务、任务文件读不出)返回 None。

    经决策入口的 key_of 取:它先查任务目录里每个文件的类型,qc.dod_hash 自己不查——目录里有 FIFO 时会挂住。
    """
    try:
        return qd.key_of("dod_gate", root, None)
    except (qc.FactsError, RecursionError):
        return None


def _dod_gate_pending(root: Path, st: qc.Settings) -> bool:
    """Leader 是否正停在 DoD 确认门:evidence 对得上当前 DoD,而 release 不存在或对不上。"""
    if st.config.get("autonomy", "dod-gate") == "full-auto":
        return False
    current = _current_dod(root)
    if current is None:
        return False
    # 证据文件在这里只拿 dod_sha256 去比,不转成文本:不设深度上限,免得多一个很深的字段就把「正停在确认门」判没了。
    evidence = _json(_docs(root) / qd.EVIDENCE, any_depth=True)
    if not isinstance(evidence, dict) or evidence.get("dod_sha256") != current:
        return False
    return not _release_matches(root, current)


def _blank_status() -> Dict[str, Any]:
    """status 的全部键及其「什么都还没算出来」时的取值。"""
    return {"enabled": False, "quiet": False, "night": None, "pending_nights": [],
            "dod_gate_pending": False, "decider": None, "enforce_fp_expected": None, "fp_mismatch": []}


def _leftover_nights(root: Path) -> List[str]:
    """配置读不出一份完整设置(非法,或 quiet_hours 段被删)时,此前攒下、还没呈现的夜晚。**不抛**。
    配置坏掉不等于那些夜晚不用看了:原先这里直接给空列表,周期任务于是回「无」,攒下的夜晚无声消失。"""
    try:
        return _pending_nights(root, _window_only(root), qc.now()[0])
    except Exception:  # noqa: BLE001 —— 这是出错路径上的附带信息,不得盖掉调用方已经拿到的那条 error
        return []


def status(root: Path) -> Dict[str, Any]:
    """**不抛**。配置非法时 error 是 ConfigError 的原文;其余任何出错 error 以「内部错误: 」开头,
    已经算出来的字段照给——会话启动检查要把前一种原样告诉人。
    配置非法或未配置时 pending_nights 照给(见 _leftover_nights),其余字段保持空白取值。"""
    out = _blank_status()
    try:
        try:
            st = qc.load(root)
        except qc.ConfigError as e:
            out["error"] = str(e)
            out["pending_nights"] = _leftover_nights(root)
            return out
        if st is None:
            out["pending_nights"] = _leftover_nights(root)
            return out
        out["enabled"] = bool(st.enabled)
        at, _ = qc.now()
        # 待呈现的夜晚只依赖日志与窗口,先算:后面指纹或档位算不出来时,攒下的夜晚不该跟着消失。
        out["pending_nights"] = _pending_nights(root, st, at)
        # 指纹与功能开关无关:人授权放权时要抄 enforce_fp_expected,功能关着也得打得出来。
        fp = deciders.fingerprint(st.decider, st.config)
        out["decider"] = {"name": st.decider, "fp": fp}
        out["enforce_fp_expected"] = qc.enforce_fp_expected(fp, st.min_confidence)
        out["fp_mismatch"] = [k for k in qc.KINDS if qc.effective_mode(st, k, fp)[1]]
        night = qc.window(st, at) if st.enabled else None
        out["quiet"], out["night"] = night is not None, night
        out["dod_gate_pending"] = _dod_gate_pending(root, st)
    except Exception as e:  # noqa: BLE001 —— status 恒给出一个 JSON
        out["error"] = INTERNAL + _t(f"{type(e).__name__}: {e}", 200)
    return out


# ---------------------------------------------------------------------------
# 旁路记录
# ---------------------------------------------------------------------------

def heartbeat(root: Path) -> None:
    """静默期内记一次心跳;不在静默期或功能关闭是 no-op。**不抛**:出错只记 errors.jsonl。"""
    try:
        st = qc.load(root)
        if st is None or not st.enabled:
            return
        night = qc.window(st, qc.now()[0])
        if night:
            qc.append(root, ACTIVITY, {"ts": round(time.time(), 1), "night": night, "type": "heartbeat"})
    except Exception as e:  # noqa: BLE001 —— 周期任务每轮都调它,不能因为它出错而中断后面的动作
        qc.log_error(root, "quiet_heartbeat", f"{type(e).__name__}: {e}")


def _countable(row: Dict[str, Any]) -> bool:
    """这条决策能不能进一致率:带着模型意见,且不是测试时钟下产生的。record-human 与 _agreement 共用这一条,
    免得「人的决定被收下了、一致率里却没有它」。"""
    return _has_opinion(row) and "clock" not in row


def record_human(root: Path, decision_id: str, decision: str, why: Optional[List[str]] = None) -> bool:
    """记下人对某条决策的决定。只用于呈现一致率,不参与任何放行判定(写者无法鉴别)。

    只收一致率会计数的那些:决策行存在、带模型意见、不是测试时钟下产生的,且取值是该类的合法词。
    其余一律拒绝(不落盘),原因追加到 why——收下却不计数,人会以为自己的决定已经算进去了。
    """
    def refuse(reason: str) -> bool:
        if why is not None:
            why.append(reason)
        return False

    if not isinstance(decision_id, str) or not decision_id or not isinstance(decision, str):
        return refuse("decision_id 与决定都须为非空字符串")
    rows = [r for r in qc.read(root, qd.LOG) if r.get("type") == "decision" and r.get("id") == decision_id]
    if not rows:
        return refuse("决策日志里没有这条 decision_id")
    row = rows[-1]      # 同一个 id 有多行时取最后一行:与 _agreement 的 by_id 同一口径
    kind = row.get("kind")
    if not isinstance(kind, str) or decision not in HUMAN_DECISIONS.get(kind, ()):
        allowed = " / ".join(HUMAN_DECISIONS.get(kind, ())) if isinstance(kind, str) else ""
        return refuse(f"决定不是该类的合法取值(可选: {allowed or '无'})")
    if not _countable(row):
        return refuse("这条决策没有模型意见(或产生于测试时钟下),人的决定无从与模型对比,不记")
    try:
        qc.append(root, ACTIVITY, {"ts": round(time.time(), 1), "type": "human", "night": row.get("night"),
                                   "kind": kind, "decision_id": decision_id, "decision": decision})
    except qc.LogError:
        return refuse("活动日志写不下来")
    return True


def presented(root: Path, night: str) -> bool:
    """记下某一晚的简报已经呈现。**拒绝尚未结束的夜晚**,也拒绝判断不了是否结束的(配置非法或未配置):
    窗口还开着就记「已呈现」,之后同一晚再产生的决策就不会再被提示。"""
    if not _valid_night(night):
        return False
    try:
        st = qc.load(root)
        if st is None or not qc.night_ended(st, night, qc.now()[0]):
            return False
    except Exception:  # noqa: BLE001 —— 配置非法、日期算不出来……判断不了是否结束,一律拒绝
        return False
    try:
        qc.append(root, ACTIVITY, {"ts": round(time.time(), 1), "type": "presented", "night": night})
    except qc.LogError:
        return False
    return True


# ---------------------------------------------------------------------------
# render 的各个部件
# ---------------------------------------------------------------------------

def _bounds(st: qc.Settings, night: str) -> Tuple[datetime, datetime]:
    d = date.fromisoformat(night)
    start = datetime(d.year, d.month, d.day, st.start // 60, st.start % 60, tzinfo=st.tz)
    e = d + timedelta(days=1) if st.start > st.end else d
    return start, datetime(e.year, e.month, e.day, st.end // 60, st.end % 60, tzinfo=st.tz)


def _in(ts: Any, start: datetime, end: datetime) -> bool:
    return _number(ts) and start.timestamp() <= ts < end.timestamp()


def _transitions(root: Path, start: datetime, end: datetime) -> Tuple[Optional[List[Dict[str, Any]]], int]:
    """(窗口内的状态迁移行, 时间读不出来的行数);文件不存在(已归档)时前者为 None。

    时间读不出来的行没法归到哪一晚,不能悄悄丢掉:其中若有熔断解除,对账就漏了它。
    """
    rows = _rows(root / ".arcforge" / "tasks" / "transitions.jsonl")
    if rows is None:
        return None, 0
    inside, undated = [], 0
    for r in rows:
        if "to" not in r:
            continue
        at = r.get("at")
        try:
            when = datetime.fromisoformat(at.replace("Z", "+00:00")) if isinstance(at, str) else None
            if when is not None and when.tzinfo is None:
                when = when.replace(tzinfo=timezone.utc)
            if when is not None and start <= when < end:
                inside.append(r)
        except (ValueError, OverflowError):
            when = None
        if when is None:
            undated += 1
    return inside, undated


def _kind(row: Dict[str, Any]) -> str:
    kind = row.get("kind")
    return KIND_LABEL[kind] if isinstance(kind, str) and kind in KIND_LABEL else _t(kind, 24)


def _subject(row: Dict[str, Any]) -> str:
    if row.get("kind") == "blocked_human":
        return _t(row.get("task"), 40)
    key = row.get("key")
    return f"DoD {_t(key, 12)}" if isinstance(key, str) else "DoD"


def _decider(row: Dict[str, Any]) -> Tuple[str, str]:
    dec = row.get("decider")
    dec = dec if isinstance(dec, dict) else {}
    return _t(dec.get("name"), 24), _t(dec.get("fp"), 16)


def _has_opinion(row: Dict[str, Any]) -> bool:
    return isinstance(row.get("opinion"), dict)


def _opinion(row: Dict[str, Any]) -> str:
    if not _has_opinion(row):
        return "无"
    op = row["opinion"]
    conf = op.get("confidence")
    shown = f"{conf:.2f}" if _number(conf) else _t(conf, 12)
    name, fp = _decider(row)
    return f"{_t(op.get('choice'), 24)}（置信度 {shown}，{name}/{fp}）"


def _why(row: Dict[str, Any]) -> str:
    reason = row.get("hold_reason")
    label = HOLD_LABEL[reason] if isinstance(reason, str) and reason in HOLD_LABEL else _t(reason, 40)
    return f"{label} `{_t(row['hard_rule'], 24)}`" if row.get("hard_rule") else label


def _positive(row: Dict[str, Any]) -> bool:
    """放权档下的一条正面决策(执行器肯落地的那种)。"""
    kind = row.get("kind")
    return (isinstance(kind, str) and kind in qd.ACTIONS and row.get("action") == qd.ACTIONS[kind]
            and row.get("mode") == "enforce" and "clock" not in row)


def _marker_names(root: Path, row: Dict[str, Any]) -> bool:
    """磁盘上的落地标记是否**正是这条决策**的:decision_id 逐字相等。

    只看「任务带着 auto_redispatch」或「release 文件存在」不够——那可能是上一晚或另一条决策落的地。
    decision_id 以它产生的那一晚开头(决策入口这样编号);行自称的夜晚与之不符时不认:标记永久留在任务文件里,
    把一条已落地的决策行照抄一份、只改 night,不得让它在另一晚再作保一次。
    """
    did = row.get("id")
    if not isinstance(did, str) or not did or not did.startswith(f"{row.get('night')}:"):
        return False
    if row.get("kind") == "blocked_human":
        mark = _task(root, row.get("task")).get("auto_redispatch")
    else:
        mark = _json(_docs(root) / qd.RELEASE)
    return isinstance(mark, dict) and mark.get("decision_id") == did


def _marker_file(root: Path, row: Dict[str, Any]) -> Tuple[str, Optional[Path]]:
    """(叫法, 路径):这条决策的落地标记所在的文件——熔断是任务文件,确认门是放行文件。任务 ID 形状不对时路径为 None。"""
    if row.get("kind") == "blocked_human":
        task_id = row.get("task")
        ok = isinstance(task_id, str) and qd.TASK_ID_RE.fullmatch(task_id)
        return "任务文件", (root / ".arcforge" / "tasks" / f"{task_id}.json") if ok else None
    return "放行文件", _docs(root) / qd.RELEASE


def _agreement(root: Path) -> List[str]:
    """累计一致率,按决策模型(名称 + 指纹)× 类别分开。每条决策只计一次(同一条记了多次以最后一次为准);
    人的决定须是该类的合法取值;测试时钟下产生的记录不计。"""
    by_id = {r.get("id"): r for r in qc.read(root, qd.LOG)
             if r.get("type") == "decision" and isinstance(r.get("id"), str)}
    answers: Dict[str, str] = {}
    for h in qc.read(root, ACTIVITY):
        did, answer = h.get("decision_id"), h.get("decision")
        row = by_id.get(did) if h.get("type") == "human" and isinstance(did, str) else None
        kind = row.get("kind") if row else None
        if (not row or not _countable(row) or not isinstance(kind, str)
                or not isinstance(answer, str) or answer not in HUMAN_DECISIONS.get(kind, ())):
            continue
        answers[did] = answer
    tally: Dict[Tuple[str, str, str], List[int]] = {}
    for did, answer in answers.items():
        row = by_id[did]
        kind = row["kind"]
        dec = row.get("decider") if isinstance(row.get("decider"), dict) else {}
        positive = qd.ACTIONS[kind]
        model = row.get("action") == positive or row.get("hold_reason") in WOULD_ACT
        # 分桶用原值,不用截断后的显示串:截断后同形的两个名字不是同一个决策模型。
        cell = tally.setdefault((str(dec.get("name")), str(dec.get("fp")), kind), [0, 0])
        cell[0] += int(model == (answer == positive))
        cell[1] += 1
    return [f"- {_t(name, 40)}（{_t(fp, 16)}）· {KIND_LABEL[kind]}：{agree} / {total}"
            for (name, fp, kind), (agree, total) in sorted(tally.items())]


LIMITS = [
    "- 一致率只衡量「决策模型与人是否一致」，**不衡量**「决定是否正确」。",
    "- 一致率只对产生它的决策模型（名称 + 指纹）成立；指纹覆盖不到外部脚本的内容与远端模型自身的更新。",
    "- 人的决定由 Leader 代为记录，写者无法鉴别，仅供参考、不参与任何放行判定。",
    "- 熔断的原因标记与 DoD 确认门的证据文件由 Leader 填写，入口无法独立核对其内容。",
    "- `arcforge.config.json` 不受写保护：任何 agent 都能改决策模型、档位与 `enforce_fp`。"
    "指纹绑定防的是遗忘，不防故意；请留意第 4 段列出的决策模型名称与指纹是否有变。",
    "- 「每个任务终身至多重派一次」这条规则在影子档下不会被触发，只由测试保证。",
    "- DoD 确认门按验收标准的内容去重：需求文档变了而验收标准没变时，同一晚会沿用之前的模型意见。",
    "- 决策日志任何实例都能追加，简报无法鉴别某一行是谁写的；放权档下，能追加决策日志的实例等同于决策模型。"
    "本简报只把「正面决策行 + 经写通道落下的标记」认作已落地，但决策行本身同样可以被追加。"
    "活动日志同样如此：一行追加进去的「已呈现」能让某一晚不再被提示。",
    "- 熔断的原因标记（`blocked`）可被任务的 owner 预先写好：入口只认得出 Leader 上一次留下的陈旧值，"
    "认不出别的写者在当前这一轮留下的值。",
    "- 影子档同样出境：影子档下也会把任务与验收标准发给决策模型；发往哪里由 `deciders` 的配置决定，改配置即改目的地。",
    "- 没有心跳、也没有任何决策的夜晚不会出现在待呈现的名单里，也就没有简报来提示这一晚其实没有运行——"
    "会话在入夜前已关闭、周期任务没有登记时正是这种情形。",
]


def render(root: Path, night: str) -> str:
    if not _valid_night(night):
        raise RenderError(f"night 非法: {_t(repr(night), 40)}")
    try:
        st = qc.load(root)
    except qc.ConfigError as e:
        raise RenderError(f"配置非法: {e}") from e
    if st is None:
        raise RenderError("没有 quiet_hours 配置")
    try:
        start, end = _bounds(st, night)
    except OverflowError as e:      # 9999-12-31 这类日期算不出窗口的结束时刻(日期本身已由 _valid_night 校验过)
        raise RenderError(f"night 算不出静默期窗口: {_t(repr(night), 40)}") from e

    log = qc.read(root, qd.LOG)
    rows = [r for r in log if r.get("type") == "decision" and r.get("night") == night]
    applied = [r for r in log if r.get("type") == "applied" and r.get("night") == night
               and isinstance(r.get("decision_id"), str)]
    applied_n = Counter(r["decision_id"] for r in applied)      # decision_id → 指向它的落地行数
    applied_ids = set(applied_n)
    known_ids = {r["id"] for r in rows if isinstance(r.get("id"), str)}
    # 同一个 id 在日志里有多行时,record-human 与一致率读的都是最后一行。
    final = {r["id"]: r for r in log if r.get("type") == "decision" and isinstance(r.get("id"), str)}
    # 指向当晚任何一条决策行都对不上的落地记录:没有键可挂,单列出来。连 decision_id 都没有的另计。
    orphans = sorted(applied_ids - known_ids)
    nameless = sum(1 for r in log if r.get("type") == "applied" and r.get("night") == night
                   and not isinstance(r.get("decision_id"), str))
    groups: Dict[Tuple[str, str], List[Dict[str, Any]]] = {}
    for r in rows:      # 周期扫描每轮对同一键追加一行,按键折叠
        groups.setdefault((str(r.get("kind")), str(r.get("key") or r.get("task") or "-")), []).append(r)
    transitions, undated = _transitions(root, start, end)

    # 每个键归到哪一段:磁盘上的落地标记正是其中某条**正面**决策的 ⇒ 第 2 段(该决策没有 applied 行时标 ⚠:
    # 已落地,但日志缺落地记录);其余 ⇒ 第 1 段。applied 行不决定归属:它指向的决策不是正面的、是影子档的,
    # 或标记对不上时,条目留在第 1 段并标 ⚠——日志说落地了,磁盘上找不到凭据。
    # (该键的全部决策行, 指向它却不作数的落地行数, 放行文件的槽位里是否记着另一条决策,
    #  标记所在的文件在而读不出来时它的叫法——否则为空串)
    waiting: List[Tuple[List[Dict[str, Any]], int, bool, str]] = []
    slot = _json(_docs(root) / qd.RELEASE)      # 放行文件只有一个槽位:它此刻记着哪条决策
    slot_id = slot.get("decision_id") if isinstance(slot, dict) else None
    # (代表行, 有落地记录的决策数, 是否缺落地记录, 不作数的落地行数, 这条决策的 id 是否已被前面的键占用)
    done: List[Tuple[Dict[str, Any], int, bool, int, bool]] = []
    claimed = set()     # 已经认过一次落地的 decision_id:一个标记只对应一次落地,不能为两个键作保
    for g in groups.values():
        cited = [r for r in g if isinstance(r.get("id"), str) and r["id"] in applied_ids]
        landed = [r for r in g if _positive(r) and _marker_names(root, r)]
        if landed:
            head = landed[0]
            # 确认门同一份 DoD 可以有多条正面决策各落地一次(放行文件只留最后一条的标记),熔断不可能。
            gate = head.get("kind") == "dod_gate"
            sound = [r for r in cited if r in landed or (gate and _positive(r))]
            unsound_n = sum(applied_n[r["id"]] for r in cited if r not in sound)
            done.append((head, len({r["id"] for r in sound}), head not in cited, unsound_n, head["id"] in claimed))
            claimed.add(head["id"])
        else:
            # 确认门同一晚先后放行两份 DoD 时,前一次的标记被后一次覆盖:落地行指向的是一条正面决策,
            # 而槽位里此刻记着别的决策。这一次是否落过地无法确认——不能写成「没有落地」。
            # 这里只知道「槽位里是另一条决策」,不知道它是不是后来才落的地(可能是很久以前的,也可能是人写的):
            # 所以文案只说槽位里记的是谁,不说先后。
            overwritten = (isinstance(slot_id, str) and bool(slot_id)
                           and all(r.get("id") != slot_id for r in g)
                           and any(_positive(r) and r.get("kind") == "dod_gate" for r in cited))
            # 这个键有正面决策,而标记所在的文件只因嵌套过深才读不出来:它是否落过地无法确认。不能当成「没有标记」
            # 写「没有落地」——没有这道深度上限的话,这一条可能本该在第 2 段。
            # 坏 JSON 的文件不在此列:那种情形下「没有落地」是引入上限之前就有的说法(放行文件另由第 4 段单列)。
            unverifiable = ""
            positives = [r for r in g if _positive(r)]
            if positives:
                label, marker_path = _marker_file(root, positives[0])
                if marker_path is not None and _too_deep(marker_path):
                    unverifiable = label
            waiting.append((g, sum(applied_n[r["id"]] for r in cited), overwritten, unverifiable))

    out: List[str] = []

    def add(line: str = "") -> None:
        out.append(_one_line(line))

    add(f"# 夜间简报 {night}")
    add()
    add(f"静默期 {start.strftime('%m-%d %H:%M')} – {end.strftime('%m-%d %H:%M')}（{_t(st.tz.key, 40)}）")
    add()

    # ---- 1 ----
    add("## 1. 等你决定")
    add()
    # 凡是没有落地的键都在这里。放权档下「模型已同意、执行器却拒绝或没被调用」的键,最后一行是
    # 正面取值而不是 hold——它同样没有落地,同样需要人来看,不能因为不是 hold 就从简报里消失。
    for g, cited_n, overwritten, unverifiable in waiting:
        last = g[-1]
        if last.get("action") == "hold":
            why = _why(last)
        elif overwritten or unverifiable:
            why = "**决策模型已同意；这一次是否落过地无法确认**（见本条末尾）"
        else:
            why = "**决策模型已同意，但没有落地**（执行器拒绝或未被调用，见第 4 段的错误记录）"
        # 给人的 decision_id 是「人的决定记在哪一条上」:取最后一条带模型意见的行。最后一行常常没有意见
        # (并发占用、调用失败),把它的 id 给出去,人的决定就对不上任何模型意见。
        opined = [r for r in g if _has_opinion(r)]
        ref = opined[-1] if opined else last
        opinion = _opinion(ref) + ("（取自当晚较早的一次询问）" if ref is not last else "")
        line = (f"- **[{_kind(last)}] {_subject(last)}** — {why}；模型意见：{opinion}；"
                f"当晚询问 {len(g)} 次；`{_t(ref.get('id'))}`")
        # 给出去的 id 若记不了,就在这里说。判的是 record-human 实际会读的那一行(同一个 id 的最后一行)。
        # id 取自日志,可以是任何 JSON 值:不是非空字符串就不拿它去查表(数组、对象当不了字典的键),
        # 这一行按「没有可用的 id」处理——record-human 按字符串查不到它。
        rid = ref.get("id")
        if not opined:
            line += "（当晚没有模型意见，这一条不记人的决定）"
        elif "clock" in ref:
            line += "（测试时钟下的记录，这一条不记人的决定）"
        elif not isinstance(rid, str) or not rid:
            line += "（这一行没有可用的 id，这一条不记人的决定）"
        elif not _countable(final.get(rid, ref)):
            line += "（这个 id 在日志里的最后一行不带可对比的模型意见，这一条不记人的决定）"
        if unverifiable:
            line += (f" ⚠ **{unverifiable}存在但读不出来**（嵌套过深），无法核对它是否带着这条决策的落地标记"
                     + (f"；日志里有 {cited_n} 条落地记录指向这个键" if cited_n else "") + "。请核实。")
        elif overwritten:
            line += (f" ⚠ **日志里有 {cited_n} 条落地记录指向这个键，而放行文件里记的是另一条决策**"
                     "（放行文件只有一个槽位）；这一次是否落过地无法确认，请核实。")
        elif cited_n:
            line += (f" ⚠ **日志里有 {cited_n} 条落地记录指向这个键，但找不到落地的凭据**"
                     "（它指向的不是放权档下的正面决策，或磁盘上的落地标记不是这条决策的）；按未落地处理，请核实。")
        add(line)
    if orphans:
        add(f"- ⚠ **有 {sum(applied_n[i] for i in orphans)} 条落地记录找不到对应的决策**："
            + _names([f"`{_t(i)}`" for i in orphans]) + " —— 当晚的决策日志里没有这些决策；按未落地处理，请核实。")
    if nameless:
        add(f"- ⚠ **有 {nameless} 条落地记录没有可用的 decision_id**，对不上任何决策；按未落地处理，请核实。")
    if not waiting and not orphans and not nameless:
        add("（无）")
    add()

    # ---- 2 ----
    add("## 2. 已代你决定并落地")
    add()
    for r, logged_n, unlogged, unsound_n, claimed_twice in done:
        if r.get("kind") == "blocked_human":
            task_id = r.get("task") if isinstance(r.get("task"), str) else ""
            task = _task(root, task_id)
            line = (f"- **[熔断] {_t(task_id, 40)}** — 已重派（当前状态 `{_t(task.get('status'))}`，指派给 "
                    f"`{_t(task.get('assigned_to'))}`）。**不可退回熔断状态**；可收回改派、跳过，或等其再次判定。")
            if transitions is not None and not any(
                    t.get("task") == task_id and t.get("from") == "blocked_human" and t.get("to") == "assigned"
                    for t in transitions):
                line += " ⚠ 有落地记录，但状态变更记录里找不到对应的迁移。"
        else:
            dispatched_n, unreadable_n = _dispatched(root)
            line = (f"- **[DoD 确认门]** — 已放行开工（当前已派发 {dispatched_n} 个任务"
                    + (f"；另有 {unreadable_n} 个任务文件读不出来，未计入" if unreadable_n else "")
                    + "）。可停掉团队后重新确认。")
            if not _release_matches(root, r.get("key")):
                line += " ⚠ 有落地记录，但放行文件不存在或与当时的 DoD 对不上。"
        if unlogged:
            line += " ⚠ **已落地，但日志缺落地记录**（写通道没有确认成功，或落地记录没写下来；见第 4 段的错误记录）。"
        elif logged_n > 1:
            line += f" 落地记录 {logged_n} 条（同一份 DoD 的多条决策各落地了一次）。"
        if unsound_n:
            line += f" ⚠ 另有 {unsound_n} 条落地记录指向这个键里没有落地凭据的决策，不作数。"
        if claimed_twice:
            line += (" ⚠ **这条决策的 id 已经在上面另一个键里出现过**：一个落地标记只对应一次落地，"
                     "这一条不为任何解除作保，请核实决策日志。")
        add(line + f" `{_t(r.get('id'))}`")
    if not done:
        add("（无）")
    add()

    # ---- 3 ----
    add("## 3. 影子对比")
    add()
    shadow = []
    for g in groups.values():       # 每个键取最后一条带模型意见的影子档记录
        opinions = [r for r in g if r.get("mode") == "shadow" and _has_opinion(r)]
        if opinions:
            shadow.append(opinions[-1])
    for r in shadow:
        would = "会放行" if r.get("hold_reason") in WOULD_ACT else "会留到早上"
        add(f"- [{_kind(r)}] {_subject(r)}：模型意见 {_opinion(r)} ⇒ 放权档下{would}；`{_t(r.get('id'))}`")
    if not shadow:
        add("（无）")
    add()

    # ---- 4 ----
    add("## 4. 执行概要")
    add()
    beats = sorted(r["ts"] for r in qc.read(root, ACTIVITY)
                   if r.get("type") == "heartbeat" and r.get("night") == night and _number(r.get("ts")))
    beats = [t for t in beats if _clock(t, st.tz)]        # 换算不了的时间戳不算心跳
    if beats:
        add(f"- 心跳 {len(beats)} 次（首 {_clock(beats[0], st.tz)}，末 {_clock(beats[-1], st.tz)}）")
    else:
        add("- **夜间未运行**：没有任何心跳。会话可能已关闭，或周期任务未登记。")
    models = sorted({_decider(r) for r in rows})
    if models:
        add("- 当晚给出意见的决策模型：" + "、".join(f"{n}（{f}）" for n, f in models))
    failures = sum(1 for r in rows if r.get("hold_reason") == "decider_error" and "latency_ms" in r)
    add(f"- 决策模型调用失败 {failures} 次")
    mismatched = sum(1 for r in rows if r.get("hold_reason") == "fp_mismatch")
    if mismatched:
        add(f"- ⚠ **放权指纹不符**：{mismatched} 次询问按影子档处理（档位配置为 enforce，"
            "但 `enforce_fp` 与当前放权指纹不一致）")
    # 上面那条只数「模型本会同意」的询问。模型本来就不同意、或命中硬规则的夜晚一条都数不到,
    # 而整晚其实被降成了影子档——所以另按**当前配置**报一次。
    try:
        now_fp = deciders.fingerprint(st.decider, st.config)
        now_off = [KIND_LABEL[k] for k in qc.KINDS if qc.effective_mode(st, k, now_fp)[1]]
    except Exception:  # noqa: BLE001 —— 指纹算不出来由 status 报,这里不因此废掉整份简报
        now_off = []
    if now_off:
        add("- ⚠ **当前配置下放权指纹不符**：" + "、".join(now_off) + " 配置为 enforce，实际按影子档运行")
    if transitions is None:
        add("- 状态变更记录已归档（见 `.arcforge/archive/`），迁移计数与对账不可用")
    else:
        counts = Counter(_t(t.get("to"), 32) for t in transitions)
        add("- 状态迁移：" + ("、".join(f"{k} ×{v}" for k, v in sorted(counts.items())) or "无"))
        # 逐条对账而不是按任务集合做差:同一晚同一任务先被执行器重派、又被手工解除一次时,后者不能被吞掉。
        lifted_n = Counter(_t(t.get("task"), 40) for t in transitions
                           if t.get("from") == "blocked_human" and t.get("to") == "assigned")
        # 「经决策模型解除」的次数 = 已落地的决策数(每个 decision_id 至多一次):作保的是「正面决策行 + 磁盘标记」,
        # 不是 applied 行——只追加一行 applied 就能替一次手工解除作保的话,这项对账就形同虚设。
        vouched = Counter(_t(r.get("task"), 40) for r, _, _, _, claimed_twice in done
                          if not claimed_twice and r.get("kind") == "blocked_human")
        lifted = sorted(t for t, n in lifted_n.items() if n > vouched.get(t, 0))
        if lifted:
            add("- ⚠ **夜间未经决策模型的熔断解除**：" + _names(lifted))
        if undated:
            add(f"- ⚠ 另有 {undated} 条状态变更记录的时间读不出来，未计入上面的计数与对账")
        bad_rows = _bad_lines(root / ".arcforge" / "tasks" / "transitions.jsonl")
        if bad_rows:
            add(f"- ⚠ 另有 {bad_rows} 行状态变更记录读不出来（坏行、不是对象或嵌套过深），未计入上面的计数与对账；"
                "其中若有熔断解除或开工，这里看不到")
        started = sorted({_t(t.get("task"), 40) for t in transitions
                          if t.get("from") == "pending" and t.get("to") == "assigned"})
        # 当前 DoD 的哈希算不出来时 _release_matches 为假:无从确认 = 未经确认。
        if (started and st.config.get("autonomy", "dod-gate") != "full-auto"
                and not _release_matches(root, _current_dod(root))):
            add("- ⚠ **夜间未经确认的开工**：" + _names(started))
    release = _json(_docs(root) / qd.RELEASE)
    if _unreadable(_docs(root) / qd.RELEASE):
        # 读不出来不等于没有:原先两种都跳过,放行文件里多一个很深的字段,下面两条对账就无声消失。
        add("- ⚠ **放行文件存在但读不出来**（坏 JSON、不是对象或嵌套过深）：无法核对它与决策日志是否相符，"
            "也无法确认它是不是决策模型写的")
    if isinstance(release, dict) and str(release.get("by", "")).startswith("decider:"):
        rid = release.get("decision_id")
        named = isinstance(rid, str) and bool(rid)      # 没有 decision_id 的放行文件对不上任何一行(None == None 不算)
        if not (named and any(r.get("type") == "applied" and r.get("decision_id") == rid for r in log)):
            add("- ⚠ **放行文件由决策模型写入，但决策日志里没有对应的落地记录**")
        elif not any(r.get("type") == "decision" and r.get("id") == rid and _positive(r)
                     and r.get("kind") == "dod_gate" and r.get("key") == release.get("dod_sha256") for r in log):
            add("- ⚠ **放行文件自称由决策模型写入，但决策日志里找不到这条放权档下的正面决策**"
                "（须是确认门的决策，且针对的正是放行文件里那份 DoD）")
    errors = Counter(_t(r.get("where"), 32) for r in qc.read(root, "errors") if _in(r.get("ts"), start, end))
    if errors:
        add("- 错误记录：" + "、".join(f"{k} ×{v}" for k, v in sorted(errors.items())))
    perms = [r for r in qc.read(root, PERMISSIONS) if r.get("night") == night]
    heads = Counter(_t(r.get("head") or r.get("tool"), 32) for r in perms)
    add(f"- 权限提示 {len(perms)} 条" + ("：" + "、".join(f"{k} ×{v}" for k, v in heads.most_common(8)) if perms else ""))
    queue_path = _docs(root) / "06-acceptance" / "jev-human-queue.jsonl"
    queue = _rows(queue_path) or []
    bad_queue = _bad_lines(queue_path)
    add(f"- 质量闸门转人工队列当晚新增 {sum(1 for r in queue if _in(r.get('ts'), start, end))} 条"
        + (f"（另有 {bad_queue} 行读不出来，不知道属于哪一晚）" if bad_queue else ""))
    add()

    add("## 一致率（累计）")
    add()
    for line in _agreement(root) or ["（还没有人的决定可供对比）"]:
        add(line)
    add()
    add("## 已知限制")
    add()
    for line in LIMITS:
        add(line)
    add()
    return "\n".join(out)


# ---------------------------------------------------------------------------
# 命令行
# ---------------------------------------------------------------------------

USAGE = ("用法: quiet_briefing.py status | heartbeat | record-human <decision_id> <决定> | "
         "presented --night YYYY-MM-DD | render --night YYYY-MM-DD")


def main(argv: List[str]) -> int:
    """退出码:status / heartbeat 恒 0;record-human 0 / 2(被拒时原因在 stderr);presented 0 / 1
    (1 含「这一晚还没结束」);render 0 / 1;用法错误 64;主仓库根不可决断 1(任何子命令)。"""
    cmd = argv[0] if argv else ""
    night_form = len(argv) == 3 and argv[1] == "--night"
    ok = (argv in (["status"], ["heartbeat"]) or (cmd == "record-human" and len(argv) == 3)
          or (cmd in ("presented", "render") and night_form))
    if not ok:
        print(USAGE, file=sys.stderr)
        return 64
    try:
        root = qc.main_root()
    except (qc.RootError, OSError) as e:        # OSError:当前目录已被删除
        print(f"主仓库根不可决断: {e}", file=sys.stderr)
        return 1
    if cmd == "status":
        try:
            out = status(root)
        except Exception as e:  # noqa: BLE001 —— status 自己不抛;这是最后一道
            out = dict(_blank_status(), error=INTERNAL + _t(f"{type(e).__name__}: {e}", 200))
        print(json.dumps(out, ensure_ascii=False))
        return 0
    if cmd == "heartbeat":
        try:
            heartbeat(root)
        except Exception:  # noqa: BLE001
            pass
        return 0
    if cmd == "record-human":
        why: List[str] = []
        if record_human(root, argv[1], argv[2], why):
            return 0
        print("没有记下: " + "；".join(why), file=sys.stderr)
        return 2
    if cmd == "presented":
        return 0 if presented(root, argv[2]) else 1
    try:
        text = render(root, argv[2])
    except Exception as e:  # noqa: BLE001 —— 生成失败:exit 1、stdout 为空,Leader 改列决策日志原文
        print(f"简报生成失败: {type(e).__name__}: {e}", file=sys.stderr)
        return 1
    try:
        print(text)
    except Exception as e:  # noqa: BLE001 —— 输出不了同样是生成失败
        print(f"简报生成失败: {type(e).__name__}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
