"""
deciders — 决策模型层:统一的批量问答接口 + 内置适配器。

不属于静默期功能:不 import hooks/quiet/ 下任何模块,质量闸门日后也经这里取模型意见。
接口按「一批问题、两种题型(choice / noul)」定义,这也是 command 协议 v1 的形状。
只依赖标准库(Python >= 3.9)与同仓库的 hooks/jev/jev_common.py。
"""
from __future__ import annotations

import sys

sys.dont_write_bytecode = True

import hashlib  # noqa: E402
import json  # noqa: E402
import math  # noqa: E402
import os  # noqa: E402
import re  # noqa: E402
import shutil  # noqa: E402
import signal  # noqa: E402
import subprocess  # noqa: E402
from pathlib import Path  # noqa: E402
from typing import Any, Dict, NamedTuple, Optional  # noqa: E402

_JEV_DIR = str(Path(__file__).resolve().parent.parent / "jev")
if _JEV_DIR not in sys.path:
    sys.path.insert(0, _JEV_DIR)
import jev_common  # noqa: E402

DEFAULT_TIMEOUT_S = 15
NAME_RE = re.compile(r"^[a-z0-9-]{1,24}$")
TYPES = ("jev", "command")


class DeciderError(Exception):
    """整次调用失败:传输或协议级。单题不合格不走这里,见 validate_answers。"""


class ConfigError(Exception):
    pass


class Result(NamedTuple):
    answers: Dict[str, Dict[str, Any]]   # 题目 id -> 通过校验的答案
    invalid: Dict[str, str]              # 题目 id -> 未通过的原因
    raw: Dict[str, Any]                  # 模型给出的完整原始答案映射,不过滤、不校验


def _prob(v: Any) -> bool:
    # bool 必须单独排除:JSON 的 true 在 Python 里是 int 的子类,等于 1。字符串不做强转。
    return (isinstance(v, (int, float)) and not isinstance(v, bool)
            and math.isfinite(v) and 0 <= v <= 1)


def _check(question: Dict[str, Any], answer: Any) -> Optional[str]:
    if not isinstance(answer, dict):
        return "answer_not_object"
    if question.get("type") == "choice":
        choice = answer.get("choice")
        if not isinstance(choice, str) or choice not in (question.get("criteria") or {}):
            return "choice_not_in_criteria"
        if not _prob(answer.get("confidence")):
            return "confidence_invalid"
        return None
    if question.get("type") == "noul":
        return None if _prob(answer.get("noul")) else "noul_invalid"
    return "unknown_question_type"


def validate_answers(questions: Dict[str, Any], raw: Dict[str, Any]) -> Result:
    """逐题校验。没作答的题既不进 answers 也不进 invalid;没问过的 id 只留在 raw。"""
    answers: Dict[str, Dict[str, Any]] = {}
    invalid: Dict[str, str] = {}
    for qid, question in questions.items():
        if qid not in raw:
            continue
        why = _check(question, raw[qid])
        if why:
            invalid[qid] = why
        elif question["type"] == "choice":
            answers[qid] = {"choice": raw[qid]["choice"], "confidence": raw[qid]["confidence"]}
        else:
            answers[qid] = {"noul": raw[qid]["noul"]}
    return Result(answers, invalid, raw)


def _str_list(v: Any) -> bool:
    return isinstance(v, list) and all(isinstance(x, str) and x for x in v)


def validate_decider(name: str, entry: Any) -> None:
    if not isinstance(name, str) or not NAME_RE.fullmatch(name):
        raise ConfigError(f"deciders 键名 {name!r} 非法(须匹配 [a-z0-9-]{{1,24}})")
    where = f"deciders.{name}"
    if not isinstance(entry, dict):
        raise ConfigError(f"{where} 须为对象")
    if entry.get("type") not in TYPES:
        raise ConfigError(f"{where}.type 须为 {' / '.join(TYPES)}")
    if "timeout_s" in entry:
        t = entry["timeout_s"]
        if not isinstance(t, (int, float)) or isinstance(t, bool) or not 1 <= t <= 120:
            raise ConfigError(f"{where}.timeout_s 须在 [1, 120]")
    if entry["type"] == "command":
        if not _str_list(entry.get("command")) or not entry["command"]:
            raise ConfigError(f"{where}.command 须为非空的字符串数组")
        for key in ("allowed_projects", "env_passthrough"):
            if key in entry and not _str_list(entry[key]):
                raise ConfigError(f"{where}.{key} 须为字符串数组")
        if any(v.startswith("ARCFORGE_") for v in entry.get("env_passthrough") or ()):
            raise ConfigError(f"{where}.env_passthrough 不得含 ARCFORGE_ 前缀的变量")


def _jev_cfg(config: Dict[str, Any]) -> Dict[str, Any]:
    return jev_common._deep_merge(dict(jev_common.DEFAULT_CONFIG), config.get("jev") or {})


def fingerprint(name: str, config: Dict[str, Any]) -> str:
    """决策模型指纹:只描述「问的是谁」。timeout_s 不计入——调超时不算换模型。"""
    entry = (config.get("deciders") or {})[name]
    obj = {k: v for k, v in entry.items() if k != "timeout_s"}
    if entry.get("type") == "jev":
        jc = _jev_cfg(config)
        obj["_jev"] = {"model": jc.get("model", jev_common.DEFAULT_MODEL),
                       "api_url": jc.get("api_url") or jev_common.DEFAULT_API_URL}
    blob = json.dumps(obj, sort_keys=True, ensure_ascii=False, separators=(",", ":"))
    return hashlib.sha256(blob.encode("utf-8")).hexdigest()[:8]


BASE_ENV = ("PATH", "HOME", "LANG", "LC_ALL", "TMPDIR")
MAX_STDOUT = 64 * 1024
REAP_S = 5


def _project_allowed(allowed: Any, root: Path) -> bool:
    """与 jev_common.project_allowed 同口径,但以调用方给的主仓库根为准。

    不直接调那个函数:它内部取 CLAUDE_PROJECT_DIR 或 cwd,在 linked worktree 里拿到的是 worktree。
    """
    allowed = allowed or []
    if "*" in allowed:
        return True
    names = {root.name} | jev_common._remote_urls(root)
    return any(a in n for a in allowed for n in names)


def _resolve_command(cmd0: str, root: Path) -> Optional[str]:
    if "/" in cmd0:
        p = Path(cmd0)
        if not p.is_absolute():
            p = root / p
        return str(p) if p.is_file() and os.access(str(p), os.X_OK) else None
    return shutil.which(cmd0, path=os.environ.get("PATH", os.defpath))


def available(name: str, config: Dict[str, Any], root: Path) -> Optional[str]:
    """None 表示可用;否则返回 hold_reason。"""
    entry = (config.get("deciders") or {})[name]
    if entry["type"] == "command":
        if _resolve_command(entry["command"][0], root) is None:
            return "decider_unavailable"
        if any(not os.environ.get(v) for v in entry.get("env_passthrough") or ()):
            return "decider_unavailable"
        # 缺失或为空 = 不可用:外部命令会把内容发到哪里无从得知,出境必须逐项目显式同意。
        allowed = entry.get("allowed_projects") or []
        if not allowed or not _project_allowed(allowed, root):
            return "project_not_allowed"
        return None
    jc = _jev_cfg(config)
    # ARCFORGE_JEV_API_URL 会让 call_jev() 换用另一个端点,而指纹只并入配置里的地址。
    if os.environ.get("ARCFORGE_JEV_API_URL"):
        return "decider_unavailable"
    if not jc.get("enabled"):
        return "decider_unavailable"
    if not _project_allowed(jc.get("allowed_projects"), root):
        return "project_not_allowed"
    if not os.environ.get(jc.get("api_key_env", "TYPESAFE_API_KEY")):
        return "decider_unavailable"
    return None


def _kill_group(proc: "subprocess.Popen[bytes]") -> None:
    try:
        os.killpg(proc.pid, signal.SIGKILL)
    except (ProcessLookupError, PermissionError):
        pass
    try:
        proc.communicate(timeout=REAP_S)
    except subprocess.TimeoutExpired:
        pass   # 逃出进程组的孙进程仍占着管道:不再读,交给解释器收尾
    finally:
        for stream in (proc.stdin, proc.stdout):
            try:
                if stream:
                    stream.close()
            except OSError:
                pass


def _ask_command(entry: Dict[str, Any], root: Path, purpose: str,
                 questions: Dict[str, Any], state: Any, timeout_s: float) -> Dict[str, Any]:
    exe = _resolve_command(entry["command"][0], root)
    if exe is None:
        raise DeciderError("command not found")
    names = BASE_ENV + tuple(entry.get("env_passthrough") or ())
    env = {k: os.environ[k] for k in names if k in os.environ}
    payload = json.dumps({"protocol": 1, "purpose": purpose, "questions": questions, "state": state},
                         ensure_ascii=False).encode("utf-8")
    try:
        proc = subprocess.Popen([exe] + list(entry["command"][1:]), cwd=str(root), env=env,
                                stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                stderr=subprocess.DEVNULL, start_new_session=True)
    except OSError as e:
        raise DeciderError(f"spawn failed: {e}") from e
    try:
        out, _ = proc.communicate(payload, timeout=timeout_s)
    except subprocess.TimeoutExpired:
        _kill_group(proc)
        raise DeciderError("timeout")
    if proc.returncode != 0:
        raise DeciderError(f"exit {proc.returncode}")
    if len(out) > MAX_STDOUT:
        raise DeciderError("stdout too large")
    try:
        obj = json.loads(out.decode("utf-8").strip())   # 两个对象 ⇒ JSONDecodeError(Extra data)
    except (UnicodeDecodeError, json.JSONDecodeError) as e:
        raise DeciderError(f"stdout is not a single JSON object: {e}") from e
    if not isinstance(obj, dict) or not isinstance(obj.get("answers"), dict):
        raise DeciderError("stdout has no answers object")
    return obj["answers"]


def ask(name: str, config: Dict[str, Any], root: Path, purpose: str,
        questions: Dict[str, Any], state: Any) -> Result:
    """问一批题。传输/协议级失败抛 DeciderError;单题不合格进 Result.invalid。"""
    entry = (config.get("deciders") or {})[name]
    timeout_s = float(entry.get("timeout_s", DEFAULT_TIMEOUT_S))
    if entry["type"] == "command":
        raw = _ask_command(entry, root, purpose, questions, state, timeout_s)
    else:
        # questions 原样传下去:不重建字典、不增删键、保持插入顺序。请求体按插入顺序序列化,
        # 质量闸门迁移的验收口径是「请求体逐字节相同」。
        try:
            raw, _latency_ms = jev_common.call_jev(_jev_cfg(config), state, questions, timeout_s)
        except jev_common.JevError as e:
            raise DeciderError(str(e)) from e
    return validate_answers(questions, raw)
