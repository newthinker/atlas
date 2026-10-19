"""
quiet_common — 静默期功能的公共层:主仓库根、配置、时钟、放权指纹、日志、键锁、DoD 哈希。

一切读写以**主仓库根**为准。linked worktree 里的 arcforge.config.json 与 .arcforge/ 都是按
checkout commit 铺开的陈旧副本,读它们不会有任何报错(CLAUDE.md「隔离引入一个新的静默失败模式」)。
"""
from __future__ import annotations

import sys

sys.dont_write_bytecode = True

import hashlib  # noqa: E402
import json  # noqa: E402
import os  # noqa: E402
import re  # noqa: E402
import shutil  # noqa: E402
import subprocess  # noqa: E402
import time  # noqa: E402
from dataclasses import dataclass  # noqa: E402
from datetime import date, datetime, timedelta, timezone  # noqa: E402
from pathlib import Path  # noqa: E402
from typing import Any, Dict, List, Optional, Tuple  # noqa: E402
from zoneinfo import ZoneInfo  # noqa: E402

_HOOKS = Path(__file__).resolve().parent.parent
for _sub in ("decider", "jev"):
    if str(_HOOKS / _sub) not in sys.path:
        sys.path.insert(0, str(_HOOKS / _sub))
import deciders  # noqa: E402
from jev_common import redact  # noqa: E402,F401  (供同目录模块经本模块取用)

CONFIG_NAME = "arcforge.config.json"
KINDS = ("blocked_human", "dod_gate")
MODES = ("off", "shadow", "enforce")
_HHMM = re.compile(r"^([01]\d|2[0-3]):([0-5]\d)$")


class RootError(Exception):
    """主仓库根不可决断。"""


class ConfigError(Exception):
    pass


@dataclass
class Settings:
    config: Dict[str, Any]          # 整份 arcforge.config.json
    enabled: bool
    start: int                      # 当日分钟数
    end: int
    tz: ZoneInfo
    decider: str
    min_confidence: float
    delegate: Dict[str, str]        # KINDS 的每一项都有值
    enforce_fp: Optional[str]


# ---------------------------------------------------------------------------
# 主仓库根
# ---------------------------------------------------------------------------

def _rev_parse(cwd: Path, *what: str) -> Optional[List[str]]:
    """git rev-parse 对所问各项的回答:每项一行,都是绝对路径。

    git 不可用、超时或它自己报错(如不在仓库里)时返回 None——那是「没有 git 可问」。
    git 答了、答的却不是所问的那几个绝对路径时抛 RootError——那是「问了也拿不准」,不能当成没有 git:
    2.31 之前的 git 不认识 --path-format=absolute,把它原样回显成第一行,后面的路径也不保证是绝对的;
    把那一行当路径解析,会让 linked worktree 自己看起来像主仓库根(读到的是那里的陈旧副本)。
    路径里含换行(或 str.splitlines 认的其它行分隔符)时行数同样对不上。
    """
    try:
        r = subprocess.run(["git", "-C", str(cwd), "rev-parse", "--path-format=absolute", *what],
                           capture_output=True, text=True, timeout=5)
    except (OSError, subprocess.TimeoutExpired):
        return None
    if r.returncode != 0:
        return None
    lines = r.stdout.splitlines()
    if len(lines) != len(what) or not all(os.path.isabs(p) for p in lines):
        raise RootError(f"git 在 {cwd} 给出的不是所问的 {len(what)} 个绝对路径(git 早于 2.31 时不认识 "
                        f"--path-format=absolute;路径里含换行等行分隔符时也会这样),无法定位主仓库;"
                        f"git 给出的是 {repr(lines)[:200]}")
    return lines


def _is_main_tree(root: Path, common_dir: Path) -> bool:
    """root 确是一棵工作树的顶层,且它的 git 目录就是公共目录(裸仓库的父目录不是)。"""
    there = _rev_parse(root, "--git-dir", "--show-toplevel")
    return bool(there) and [Path(p).resolve() for p in there] == [common_dir, root]


def main_root(cwd: Optional[Path] = None) -> Path:
    """主仓库根。拿不准当前目录属于哪个项目时抛 RootError——各入口据此一律留给人,不去猜。

    公共 git 目录的父目录只是候选:它下面有配置文件,不说明当前目录就属于那个项目。有两类情形
    它指向的是另一个项目,所以再过两道判据:
      (1) 当前工作树须是该根本身,或它的 linked worktree。--separate-git-dir、裸仓库加兄弟工作树、
          子模块都不满足——那几种布局里公共目录的父目录与当前工作树没有关系。
      (2) 从当前目录向上到当前工作树顶层(不含顶层),途中任何一级有配置文件,当前目录就属于嵌套在
          仓库子目录里的另一个项目。不含顶层:linked worktree 的顶层本来就有随 checkout 铺开的一份。
          一路向上都没碰到顶层(GIT_DIR / GIT_WORK_TREE 把 git 指到了别处)同样拒绝。
    """
    cwd = Path(cwd or os.getcwd())
    found = _rev_parse(cwd, "--git-common-dir")
    common = found[0].strip() if found else ""      # strip 沿用旧实现对整段输出的处理
    if common:
        common_dir = Path(common).resolve()
        root = common_dir.parent
        if not (root / CONFIG_NAME).is_file():
            raise RootError(f"{root} 下没有 {CONFIG_NAME}")
        here = _rev_parse(cwd, "--git-dir", "--show-toplevel")
        if not here:
            raise RootError(f"git 未能给出 {cwd} 所在的工作树(多见于 git 目录内部或裸仓库),无法确认它属于 {root}")
        git_dir, top = Path(here[0]).resolve(), Path(here[1]).resolve()     # _rev_parse 保证恰好两行
        if git_dir == common_dir:
            if top != root:
                raise RootError(f"当前工作树 {top} 的 git 目录在 {common_dir},它不是 {root} 的工作树;"
                                f"无法确认当前目录属于哪个项目")
        elif git_dir.parent != common_dir / "worktrees" or not _is_main_tree(root, common_dir):
            raise RootError(f"当前工作树 {top} 不是 {root} 的 linked worktree(git 目录 {git_dir});"
                            f"无法确认当前目录属于哪个项目")
        at = cwd.resolve()
        for d in (at, *at.parents):
            if d == top:
                return root
            if (d / CONFIG_NAME).is_file():
                raise RootError(f"{d} 下另有一份 {CONFIG_NAME}:它是嵌套在工作树 {top} 子目录里的另一个项目,"
                                f"而本功能按 git 仓库定位项目,会读到 {root} 的配置与任务;无法确认该用哪一个")
        raise RootError(f"{at} 不在当前工作树 {top} 之内,无法确认它属于哪个项目")
    fallback = Path(os.environ.get("CLAUDE_PROJECT_DIR") or cwd).resolve()
    if (fallback / ".git").is_file():
        raise RootError(f"{fallback} 是 linked worktree 或子模块,而 git 不可用,无法定位主仓库")
    if not (fallback / CONFIG_NAME).is_file():
        raise RootError(f"{fallback} 下没有 {CONFIG_NAME}")
    return fallback


# ---------------------------------------------------------------------------
# 配置
# ---------------------------------------------------------------------------

def _minutes(value: Any, field: str) -> int:
    m = _HHMM.fullmatch(value) if isinstance(value, str) else None  # match 的 `$` 会放过串尾换行
    if not m:
        raise ConfigError(f"quiet_hours.{field} 须为 HH:MM")
    return int(m.group(1)) * 60 + int(m.group(2))


def parse(config: Dict[str, Any]) -> Optional[Settings]:
    """没有 quiet_hours 段 = 功能未配置,返回 None。其余任何不合法都抛 ConfigError(含 enabled=false 时)。"""
    q = config.get("quiet_hours")
    if q is None:
        return None
    if not isinstance(q, dict):
        raise ConfigError("quiet_hours 须为对象")
    enabled = q.get("enabled", False)
    if not isinstance(enabled, bool):
        raise ConfigError("quiet_hours.enabled 须为布尔值")
    start, end = _minutes(q.get("start"), "start"), _minutes(q.get("end"), "end")
    if start == end:
        raise ConfigError("quiet_hours.start 与 end 不得相同")
    try:
        tz = ZoneInfo(q["timezone"])
    except Exception as e:  # ZoneInfoNotFoundError / KeyError / TypeError / ValueError,形态随平台而异
        raise ConfigError(f"quiet_hours.timezone 不可解析: {e}") from e

    name = q.get("decider")
    table = config.get("deciders")
    if not isinstance(name, str) or not isinstance(table, dict) or name not in table:
        raise ConfigError("quiet_hours.decider 须是 deciders 里的一个键")
    try:
        deciders.validate_decider(name, table[name])
    except deciders.ConfigError as e:
        raise ConfigError(str(e)) from e

    policy = q.get("policy")
    pol = policy.get(name) if isinstance(policy, dict) else None
    where = f"quiet_hours.policy.{name}"
    if not isinstance(pol, dict):
        raise ConfigError(f"{where} 缺失或不是对象")
    mc = pol.get("min_confidence")
    if not isinstance(mc, (int, float)) or isinstance(mc, bool) or not 0 < mc <= 1:
        raise ConfigError(f"{where}.min_confidence 必填,取 (0, 1]")
    raw_delegate = pol.get("delegate", {})
    if not isinstance(raw_delegate, dict):
        raise ConfigError(f"{where}.delegate 须为对象")
    for kind, mode in raw_delegate.items():
        if kind not in KINDS:
            raise ConfigError(f"{where}.delegate 含未知类别 {kind!r}(可选: {' / '.join(KINDS)})")
        if mode not in MODES:   # 大小写敏感;拼错不得静默退化
            raise ConfigError(f"{where}.delegate.{kind}={mode!r} 非法(可选: {' / '.join(MODES)})")
    efp = pol.get("enforce_fp")
    if efp is not None and not isinstance(efp, str):
        raise ConfigError(f"{where}.enforce_fp 须为字符串或 null")
    return Settings(config=config, enabled=enabled, start=start, end=end, tz=tz, decider=name,
                    min_confidence=float(mc),
                    delegate={k: raw_delegate.get(k, "shadow") for k in KINDS}, enforce_fp=efp)


def load(root: Path) -> Optional[Settings]:
    try:
        config = json.loads((root / CONFIG_NAME).read_text(encoding="utf-8"))
    except (OSError, ValueError) as e:  # ValueError 含 JSONDecodeError 与非 UTF-8 的 UnicodeDecodeError
        raise ConfigError(f"{CONFIG_NAME} 不可读: {e}") from e
    if not isinstance(config, dict):
        raise ConfigError(f"{CONFIG_NAME} 顶层须为对象")
    return parse(config)


# ---------------------------------------------------------------------------
# 时钟与静默期窗口
# ---------------------------------------------------------------------------

def now() -> Tuple[datetime, bool]:
    """(带时区的此刻, 是否被 ARCFORGE_QUIET_NOW 覆写)。覆写仅供测试;被覆写的调用不得产生正面动作。"""
    raw = os.environ.get("ARCFORGE_QUIET_NOW")
    if not raw:
        return datetime.now(timezone.utc), False
    try:
        at = datetime.fromisoformat(raw.replace("Z", "+00:00"))
    except ValueError:
        return datetime.now(timezone.utc), True
    if at.tzinfo is None:
        at = at.replace(tzinfo=timezone.utc)
    return at, True


def window(st: Settings, at: datetime) -> Optional[str]:
    """在静默期 [start, end) 内返回归属的夜晚(窗口起始时刻所在的本地日期),否则 None。"""
    local = at.astimezone(st.tz)
    minute = local.hour * 60 + local.minute
    if st.start < st.end:
        return local.date().isoformat() if st.start <= minute < st.end else None
    if minute >= st.start:
        return local.date().isoformat()
    if minute < st.end:
        return (local.date() - timedelta(days=1)).isoformat()
    return None


def night_ended(st: Settings, night: str, at: datetime) -> bool:
    day = date.fromisoformat(night)
    if st.start > st.end:
        day += timedelta(days=1)
    end = datetime(day.year, day.month, day.day, st.end // 60, st.end % 60, tzinfo=st.tz)
    return at >= end


# ---------------------------------------------------------------------------
# 放权指纹
# ---------------------------------------------------------------------------

def enforce_fp_expected(decider_fp: str, min_confidence: float) -> str:
    blob = json.dumps([decider_fp, float(min_confidence)], separators=(",", ":"))
    return hashlib.sha256(blob.encode("utf-8")).hexdigest()[:8]


def effective_mode(st: Settings, kind: str, decider_fp: str) -> Tuple[str, bool]:
    """(生效档位, 是否因放权指纹不符而由 enforce 降为 shadow)。"""
    mode = st.delegate[kind]
    if mode != "enforce":
        return mode, False
    if st.enforce_fp == enforce_fp_expected(decider_fp, st.min_confidence):
        return "enforce", False
    return "shadow", True


# ---------------------------------------------------------------------------
# 日志:只追加;单行 <= MAX_LINE 字节,O_APPEND 单次 write
# ---------------------------------------------------------------------------

MAX_LINE = 480          # macOS 的 PIPE_BUF 是 512
LOCK_STALE_S = 300


class LogError(Exception):
    pass


class FactsError(Exception):
    """决策所需的事实读不到或格式不符。"""


def log_dir(root: Path) -> Path:
    return root / ".arcforge" / "quiet" / "log"


def _encode(record: Dict[str, Any]) -> bytes:
    return (json.dumps(record, ensure_ascii=False, separators=(",", ":")) + "\n").encode("utf-8")


def append(root: Path, name: str, record: Dict[str, Any]) -> None:
    """失败即抛。调用方据此决定「记不下来的决策不执行」。"""
    try:
        data = _encode(record)
        if len(data) > MAX_LINE and "latency_ms" in record:
            data = _encode({k: v for k, v in record.items() if k != "latency_ms"})
    except (TypeError, ValueError) as e:  # 不可序列化的值;孤立代理编不成 UTF-8(UnicodeEncodeError)
        raise LogError(f"{name}: 记录不可序列化: {e!r}") from e
    if len(data) > MAX_LINE:
        raise LogError(f"{name}: 单行 {len(data)} 字节,超过 {MAX_LINE}")
    try:
        directory = log_dir(root)
        directory.mkdir(parents=True, exist_ok=True)
        fd = os.open(str(directory / f"{name}.jsonl"), os.O_WRONLY | os.O_APPEND | os.O_CREAT, 0o600)
        try:
            if os.write(fd, data) != len(data):
                raise LogError(f"{name}: 短写")
        finally:
            os.close(fd)
    except OSError as e:
        raise LogError(f"{name}: {e}") from e


def _lines(root: Path, name: str) -> List[bytes]:
    """只以 \\n 分行、不整体解码:str.splitlines 还认 U+2028 / U+0085 等,而它们在 ensure_ascii=False 下
    原样落盘,会把一条记录切成两条坏行;整体按 UTF-8 解码则一处坏字节就让整份日志读不出来。"""
    try:
        data = (log_dir(root) / f"{name}.jsonl").read_bytes()
    except OSError:
        return []
    lines = data.split(b"\n")
    if not lines[-1]:
        lines.pop()
    return lines


def line_count(root: Path, name: str) -> int:
    return len(_lines(root, name))


MAX_DEPTH = 32      # 日志行的嵌套上限;各处写入的行不过三四层


def _deeper_than(value: Any, limit: int) -> bool:
    """value 的嵌套是否超过 limit 层(对象或数组各算一层)。不递归:它要判的正是递归会出事的输入。"""
    stack = [(value, 1)]
    while stack:
        node, depth = stack.pop()
        if isinstance(node, (dict, list)):
            if depth > limit:
                return True
            children = node.values() if isinstance(node, dict) else node
            stack.extend((child, depth + 1) for child in children)
    return False


def read(root: Path, name: str) -> List[Dict[str, Any]]:
    """日志里的对象行;坏行跳过(仍占一个序号,见 line_count)。

    嵌套过深的行也是坏行。解析它时解释器可能抛 RecursionError(不是 ValueError),也可能解析成功——
    那样更糟:这一行进了结果,之后任何一处比较、序列化或转成文本时才在调用方那里抛出来。
    两种情形都在这里挡住:一行追加进日志的东西不该能让读同一份日志的每个调用方出错。
    """
    rows = []
    for line in _lines(root, name):
        try:
            obj = json.loads(line)
        except (ValueError, RecursionError):  # JSONDecodeError、坏字节的 UnicodeDecodeError;深到解析不了的一行
            continue
        if isinstance(obj, dict) and not _deeper_than(obj, MAX_DEPTH):
            rows.append(obj)
    return rows


def _clip(text: str, limit: int) -> str:
    return text.encode("utf-8", errors="replace")[:limit].decode("utf-8", errors="ignore")


def log_error(root: Path, where: str, message: str) -> None:
    """尽力而为,绝不抛:它是别处出错时的出声通道,自己再抛就把原来的错误盖掉了。"""
    for limit in (300, 120, 0):     # 引号、换行等经 JSON 转义后会变长,超限就截得更短再试
        try:
            append(root, "errors", {"ts": round(time.time(), 1), "where": where,
                                    "error": _clip(str(message), limit)})
            return
        except LogError:
            continue
        except Exception:
            return


# ---------------------------------------------------------------------------
# 键锁:mkdir 原子性;取锁不等待;残留锁满 LOCK_STALE_S 方可破
# ---------------------------------------------------------------------------

def sha1(text: str) -> str:
    return hashlib.sha1(text.encode("utf-8")).hexdigest()


def _lock_age(lock: Path) -> float:
    try:
        ts = float(json.loads((lock / "owner").read_text())["ts"])
        if not abs(ts) < float("inf"):  # json 接受 NaN / Infinity:前者让新锁当场被破,后者让残留锁永远破不掉
            raise ValueError(ts)
        return time.time() - ts
    except (OSError, ValueError, KeyError, TypeError):
        try:
            return time.time() - lock.stat().st_mtime
        except OSError:
            return 0.0


def acquire(root: Path, key: str) -> Optional[Path]:
    """拿到返回锁目录,拿不到返回 None。并发的同键调用应得到 hold,而不是排队等结果。"""
    lock = log_dir(root) / f".lock-{sha1(key)}"
    for attempt in (0, 1):
        try:
            lock.parent.mkdir(parents=True, exist_ok=True)
            lock.mkdir()
            (lock / "owner").write_text(json.dumps({"pid": os.getpid(), "ts": time.time()}))
            return lock
        except FileExistsError:
            if attempt or _lock_age(lock) < LOCK_STALE_S:
                return None
            log_error(root, "quiet_lock_broken", key)
            shutil.rmtree(lock, ignore_errors=True)
        except OSError:
            return None
    return None


def release(lock: Optional[Path]) -> None:
    if lock is not None:
        shutil.rmtree(lock, ignore_errors=True)


# ---------------------------------------------------------------------------
# DoD 哈希:全系统只此一处实现
# ---------------------------------------------------------------------------

def dod_hash(root: Path) -> str:
    files = sorted((root / ".arcforge" / "tasks").glob("TASK-*.json"))
    if not files:
        raise FactsError("没有任何 TASK-*.json")
    lines = []
    for f in files:
        try:
            task = json.loads(f.read_text(encoding="utf-8"))
            item = {"id": task["id"], "done_criteria": task["done_criteria"]}
        except (OSError, ValueError, KeyError, TypeError) as e:  # ValueError 含 JSONDecodeError 与非 UTF-8
            raise FactsError(f"{f.name}: {e}") from e
        lines.append(json.dumps(item, sort_keys=True, ensure_ascii=False, separators=(",", ":")))
    try:
        return hashlib.sha256("\n".join(lines).encode("utf-8")).hexdigest()
    except UnicodeEncodeError as e:  # 任务文件里的 "\\ud800" 一类孤立代理
        raise FactsError(f"任务内容编不成 UTF-8: {e}") from e


def _cli(argv: List[str]) -> int:
    if argv != ["dod-hash"]:
        print("用法: quiet_common.py dod-hash", file=sys.stderr)
        return 64
    try:
        print(dod_hash(main_root()))
    except (RootError, FactsError) as e:
        print(f"dod-hash 失败: {e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(_cli(sys.argv[1:]))
