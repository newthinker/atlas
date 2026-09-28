"""
jev_common — Jev 闸门共用的 I/O：配置、HTTP 客户端、脱敏、日志、状态。

只依赖标准库（Python >= 3.9），hook 冷启动快，不要求项目装 typesafe-sdk。
"""
from __future__ import annotations

import json
import os
import re
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any, Dict, Optional, Tuple

DEFAULT_API_URL = "https://api.typesafe.ai/v1/systemone"

DEFAULT_CONFIG: Dict[str, Any] = {
    "enabled": False,
    "model": "jev-latest",
    "api_url": DEFAULT_API_URL,
    "api_key_env": "TYPESAFE_API_KEY",
    # 数据出境白名单：只有列在这里的项目（按 git remote 或目录名匹配）才会把内容发给 Jev
    "allowed_projects": [],
    "quality_gate": {
        "mode": "shadow",          # off | shadow | enforce
        "timeout_s": 15,
        "max_diff_bytes": 60000,
        # 与 arcforge.config.json 的 jev.quality_gate.dod_filter 同值；
        # 也必须与 dod_filter.judge 的签名默认值相等（两侧口径只有一份，见 GC-7）。
        "dod_filter": {"max_chars": 200, "max_semicolons": 1},
        "thresholds": {},          # 见 jev_routing.QualityThresholds
    },
    # 风险闸门（PreToolUse 代批）**不在本次范围内**，故这里没有 risk_gate 段：
    # 留一个半接线的默认值会让人以为它已经生效。要加回来时，
    # load_config() 里的 mode 覆盖也要同步补上——那两处必须一起动。
}

# 发送前脱敏：宁可多遮，不可漏发
SECRET_PATTERNS = [
    re.compile(r"(?i)(api[_-]?key|secret|token|passwd|password|private[_-]?key|access[_-]?key)"
               r"(\s*[:=]\s*)(['\"]?)[^\s'\"]{6,}\3"),
    re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----"),
    re.compile(r"\bAKIA[0-9A-Z]{16}\b"),                      # AWS access key id
    re.compile(r"\bgh[pousr]_[A-Za-z0-9]{36,}\b"),             # GitHub token
    re.compile(r"\bsk-[A-Za-z0-9_\-]{20,}\b"),                 # 常见 sk- 前缀密钥
    re.compile(r"\beyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\b"),  # JWT
]


def redact(text: str) -> str:
    out = text
    for pat in SECRET_PATTERNS:
        if pat.groups >= 2:
            out = pat.sub(lambda m: f"{m.group(1)}{m.group(2)}«REDACTED»", out)
        else:
            out = pat.sub("«REDACTED»", out)
    return out


# ---------------------------------------------------------------------------
# 路径与配置
# ---------------------------------------------------------------------------

def project_root() -> Path:
    return Path(os.environ.get("CLAUDE_PROJECT_DIR") or os.getcwd()).resolve()


def arcforge_dir() -> Path:
    return project_root() / ".arcforge"


def _deep_merge(base: Dict[str, Any], over: Dict[str, Any]) -> Dict[str, Any]:
    out = dict(base)
    for k, v in (over or {}).items():
        if isinstance(v, dict) and isinstance(out.get(k), dict):
            out[k] = _deep_merge(out[k], v)
        else:
            out[k] = v
    return out


def load_config() -> Dict[str, Any]:
    """读取 arcforge.config.json 的 "jev" 段；环境变量 ARCFORGE_JEV_MODE 可临时覆盖两个闸门的 mode。"""
    cfg = dict(DEFAULT_CONFIG)
    for candidate in (project_root() / "arcforge.config.json",
                      project_root() / ".claude" / "arcforge.config.json"):
        if candidate.exists():
            try:
                cfg = _deep_merge(cfg, json.loads(candidate.read_text()).get("jev", {}))
            except (OSError, json.JSONDecodeError) as e:
                log_error("config", f"{candidate}: {e}")
            break
    override = os.environ.get("ARCFORGE_JEV_MODE")
    if override in ("off", "shadow", "enforce"):
        # 只覆盖 quality_gate：risk_gate 已从 DEFAULT_CONFIG 移除，留着这一行会在
        # **任何设了 ARCFORGE_JEV_MODE 的人**身上抛 KeyError。而只断言
        # `assertNotIn("risk_gate", DEFAULT_CONFIG)` 的测试根本不执行这条路径，
        # 删字典不删这行会全绿着把缺陷放过去（TASK-009 的 error_handling[0] 钉的就是这个）。
        cfg["quality_gate"]["mode"] = override
    return cfg


def project_allowed(cfg: Dict[str, Any]) -> bool:
    """数据出境白名单（ADR-JEV-05）。空列表 = 不允许任何项目。"""
    allowed = cfg.get("allowed_projects") or []
    if "*" in allowed:
        return True
    root = project_root()
    names = {root.name}
    try:
        git_cfg = (root / ".git" / "config").read_text()
        names.update(re.findall(r"url\s*=\s*(\S+)", git_cfg))
    except OSError:
        pass
    return any(a in n for a in allowed for n in names)


# ---------------------------------------------------------------------------
# Jev 客户端
# ---------------------------------------------------------------------------

class JevError(Exception):
    pass


def call_jev(cfg: Dict[str, Any], state: Any, questions: Dict[str, Any],
             timeout_s: float) -> Tuple[Dict[str, Any], float]:
    """返回 (answers, latency_ms)。任何失败都抛 JevError，由调用方决定 fail 到哪里。"""
    key = os.environ.get(cfg.get("api_key_env", "TYPESAFE_API_KEY"))
    if not key:
        raise JevError("missing API key env")
    url = os.environ.get("ARCFORGE_JEV_API_URL") or cfg.get("api_url") or DEFAULT_API_URL
    body = json.dumps({"model": cfg.get("model", "jev-latest"),
                       "state": state, "questions": questions}).encode()
    req = urllib.request.Request(url, data=body, method="POST", headers={
        "Authorization": f"Bearer {key}",
        "Content-Type": "application/json",
    })
    t0 = time.monotonic()
    try:
        with urllib.request.urlopen(req, timeout=timeout_s) as resp:
            payload = json.loads(resp.read().decode())
    except urllib.error.HTTPError as e:
        raise JevError(f"HTTP {e.code}: {e.read()[:300]!r}") from e
    except (urllib.error.URLError, TimeoutError, OSError, json.JSONDecodeError) as e:
        raise JevError(f"{type(e).__name__}: {e}") from e
    latency_ms = (time.monotonic() - t0) * 1000
    answers = payload.get("answers")
    if not isinstance(answers, dict):
        raise JevError("response has no answers")
    return answers, latency_ms


# ---------------------------------------------------------------------------
# 日志与状态
# ---------------------------------------------------------------------------

def _append_jsonl(path: Path, record: Dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a", encoding="utf-8") as f:
        f.write(json.dumps(record, ensure_ascii=False) + "\n")


def log_event(kind: str, record: Dict[str, Any]) -> None:
    """kind: quality | risk | outcome。写到 .arcforge/jev/log/<kind>.jsonl，评估脚本读取这里。"""
    record = {"ts": time.time(), **record}
    try:
        _append_jsonl(arcforge_dir() / "jev" / "log" / f"{kind}.jsonl", record)
    except OSError:
        pass  # 日志失败不能影响 hook 行为


def log_error(where: str, msg: str) -> None:
    log_event("errors", {"where": where, "error": msg})


def read_state(task_id: str) -> Dict[str, Any]:
    p = arcforge_dir() / "state" / "jev" / f"{_safe(task_id)}.json"
    try:
        return json.loads(p.read_text())
    except (OSError, json.JSONDecodeError):
        return {}


def write_state(task_id: str, data: Dict[str, Any]) -> None:
    p = arcforge_dir() / "state" / "jev" / f"{_safe(task_id)}.json"
    p.parent.mkdir(parents=True, exist_ok=True)
    tmp = p.with_suffix(".tmp")
    tmp.write_text(json.dumps(data, ensure_ascii=False, indent=2))
    tmp.replace(p)


def _safe(s: str) -> str:
    return re.sub(r"[^A-Za-z0-9_.-]", "_", str(s))[:120] or "unknown"


def read_hook_input() -> Dict[str, Any]:
    raw = sys.stdin.read()
    try:
        return json.loads(raw) if raw.strip() else {}
    except json.JSONDecodeError:
        return {}
