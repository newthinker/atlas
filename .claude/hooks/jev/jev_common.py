"""
jev_common — Jev 闸门共用的 I/O：配置、HTTP 客户端、脱敏、日志、状态。

只依赖标准库（Python >= 3.9），hook 冷启动快，不要求项目装 typesafe-sdk。
"""
from __future__ import annotations

import json
import os
import re
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any, Dict, Optional, Set, Tuple

DEFAULT_API_URL = "https://api.typesafe.ai/v1/systemone"
# 钉死带版本号的 ID，不用 jev-latest：别名会移动，而阈值是对某一版本调出来的，放权指纹也只认
# 配置里的字面字符串——别名移动时指纹不变，enforce 档会静默换模型。新版本先评估再改这里。
DEFAULT_MODEL = "jev-1.13.0"

DEFAULT_CONFIG: Dict[str, Any] = {
    "enabled": False,
    "model": DEFAULT_MODEL,
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
#
# 每条规则是一个已编译的正则。带命名组 keep 的规则只遮 keep 之后的部分（键名、分隔符、
# 命令前缀留着，否则遮完的文本读不懂）；不带的整段换成 «REDACTED»。
# 写新规则时守两条：
#   · 「先认出前缀、再隔一段去找标志」的间隔必须有上限。不设上限时，前缀重复而标志不出现的
#     输入会让每个前缀都扫到行尾，耗时是二次方（原私钥规则 400 KB 实测 14–41 秒）。
#   · 顺序有意义：私钥块在最前，否则键值规则会先吃掉 BEGIN 行，块体反而漏出去。
_STRONG = (r"(?:api[_-]?key|secret(?:[_-]?key)?|token|passwd|password|passphrase"
           r"|private[_-]?key|access[_-]?key)")
# pwd / pass 太短，单独出现时多半不是密钥（测试输出的 "PASS: 用例名"、环境变量 PWD=/路径、
# JSON 里的 "pass" 选项键、"suite_pass": 263 这类计数），所以只认下面几种形态：
#   · pwd 作为名字的后缀（MYSQL_PWD、db.pwd）；
#   · 大写环境变量风格的 …_PASS（DB_PASS、REDIS_PASS）——小写的 …_pass 多半是「通过」的意思；
#   · 常见服务名之后的 pass（smtp.pass、db_pass）；URL 查询串里的 ?pass= / &pass=；
#   · 小写独立出现且用等号赋值（pwd=…、pass=…）。
_WEAK_SUFFIX = (r"(?:(?<=[_.\-&?])pwd|(?<=[&?])pass|(?-i:(?<=[A-Z0-9]_)PASS)"
                r"|(?:db|database|mysql|redis|smtp|mail|user|admin|root|ftp|ssh|auth|login|proxy|mongo|pg"
                r"|postgres|ldap|http)[_.\-]pass)")
_WEAK_BARE = r"(?<![\w.&?-])(?:pwd|pass)"
# 值：成对引号内的整段（可含空格）、不含空白的一段，或引号不成对 / 被转义时的第一段。
_VAL = r"""(?:"(?:[^"\\\n]|\\.)*"|'(?:[^'\\\n]|\\.)*'|[^\s'"\\]+|\\?['"][^\s'"\\]*)"""
_VAL_ASCII = r"""(?:"(?:[^"\\\n]|\\.)*"|'(?:[^'\\\n]|\\.)*'|[!#-&(-\[\]-~]+)"""   # 无引号时只认 ASCII 可见字符
SECRET_PATTERNS = [
    # PEM 私钥：到 END 为止整块遮掉；块体不跨过下一个 BEGIN/END 标记，所以每个 BEGIN 只扫到
    # 下一个标记为止。没有 END（被截断）时退而遮掉紧随其后的 base64 样正文。
    re.compile(r"-----BEGIN [A-Z ]{0,40}PRIVATE KEY-----"
               r"(?:[^-]*(?:-(?!----(?:BEGIN|END) )[^-]*)*-----END [A-Z ]{0,40}PRIVATE KEY-----"
               r"|[A-Za-z0-9+/=\s\\]*)"),
    # URL 与连接串里内嵌的 user:<密码>@。从 :// 起认，不从 scheme 起认——后者在长字母串上是二次方。
    re.compile(r"""(?P<keep>://[^\s/:@'"]{0,100}:)[^\s/@'"]{1,200}(?=@)"""),
    # Authorization 头（含 Proxy-Authorization）：头的值整个遮掉，不看认证方案。方案千奇百怪、
    # 凭据也不止一段（`Hawk id=… mac=…`、Digest 的多个字段）；只认几个方案名的话，不认识的方案
    # 会被当成凭据遮掉，真正的凭据原样留下——看上去遮过了，比不遮更糟。
    # 遮到哪里：值自己带开引号 ⇒ 到同型的闭引号；头名前紧挨着一个引号（-H '…' 这种）⇒ 到那个引号；
    # 否则到行尾。
    re.compile(r"""(?i)(?P<keep>(?P<q0>['"])?(?:proxy-)?authorization(?:\\?['"])?[ \t]*[:=][ \t]*"""
               r"""(?:\\?(?P<q1>['"]))?)(?(q1)(?:(?!\\?(?P=q1))[^\n])*|(?(q0)(?:(?!(?P=q0))[^\n])+|[^\n]+))"""),
    re.compile(r"(?i)(?P<keep>\bbearer[ \t]+)[A-Za-z0-9._~+/-]{20,}=*"),
    # curl -u / --user <用户>:<密码>。必须有 curl 在前：`date -u +%Y-%m-%dT%H:%M:%SZ`、
    # `docker run -u 1000:1000` 长得一样而不是凭据。带引号时遮到引号为止（口令可含空格）。
    re.compile(r"""(?P<keep>\bcurl\b[^\n|;]{0,200}?\s(?:-[A-Za-z]{0,8}[uU]|--(?:proxy-)?user)(?:[ \t]+|=)"""
               r"""(?P<q>['"])?[^\s:'"]{1,100}:)(?(q)[^'"\n]+|[^\s'"]+)"""),
    # mysql 系的 -p<密码>（紧贴；-p 后有空格是「提示输入」，-P 是端口，都不算）。
    re.compile(r"(?P<keep>\b(?:mysql|mariadb)\w*\b[^\n|;&]{0,150}?\s-p)" + _VAL),
    re.compile(r"(?P<keep>\bsshpass[ \t]+-p[ \t]*)" + _VAL),
    re.compile(r"(?P<keep>\bdocker[ \t]+login\b[^\n|;&]{0,150}?\s-p[ \t]*)(?!-)" + _VAL),
    # echo <密码> | sudo -S …，以及管道喂给 --password-stdin 的写法。
    re.compile(r"(?P<keep>\b(?:echo|printf)[ \t]+(?:-[neE]+[ \t]+)?)" + _VAL +
               r"(?=[ \t]*\|[ \t]*(?:sudo\b[^\n|;&]{0,120}?\s-S\b|[^\n|;&]{0,120}?--password-stdin\b))"),
    # 以关键字结尾的选项，值用空白分隔：--password <值>、--with-token <值>、--db-password <值>。
    # 值只认引号串或 ASCII 可见字符：文档里「--token 兜底」这样的说明文字不是值。
    re.compile(r"(?i)(?P<keep>(?<![\w-])--?(?:[a-z0-9_]+-)*(?:password|passwd|passphrase|pwd|pass|secret"
               r"|token|api[_-]?key|access[_-]?key|private[_-]?key|secret[_-]?key|auth[_-]?token)"
               r"[ \t]+)(?!-)" + _VAL_ASCII),
    # 关键字后用制表符分隔的值（TSV、对齐的配置）。
    re.compile(r"(?i)(?P<keep>(?:" + _STRONG + "|" + _WEAK_SUFFIX + r")\t+)" + _VAL),
    # XML / HTML 元素：<password>值</password>。只认「标签名以关键字结尾、且有内容」的元素；
    # <input type="password">、<label for="password"> 这类把关键字当属性值的不算。
    re.compile(r"(?i)(?P<keep><(?:[\w.-]{1,40}:)?[\w.-]{0,40}?(?:" + _STRONG + "|" + _WEAK_SUFFIX + r")>)"
               r"[^<\n]+(?=</)"),
    # 两格的键值表格行：| password | 值 |。竖线当分隔符最容易误伤普通表格，所以条件收得很紧：
    # 整行恰好两格；键格里没有空格、以关键字结尾（可被 ** 或 ` 包住）；值格不是纯短横线；
    # 下一行不是表头的分隔行（那样的话这一行是表头，「值」其实是另一列的列名）。
    re.compile(r"(?im)(?P<keep>^[ \t]*\|[ \t]*[*_`]{0,2}[\w.\-]{0,60}?(?:" + _STRONG + "|" + _WEAK_SUFFIX + r")"
               r"[*_`]{0,2}[ \t]*\|[ \t]*)(?![ \t]*:?-+:?[ \t]*\|)(?![^\n]*\n[ \t]*\|?[ \t]*:?-{2,})"
               r"[^|\n]*[^|\s](?=[ \t]*\|[ \t]*$)"),
    # 同一行内的键值：键可被引号或 Markdown 标记包住（"password": "…"、**password**: …、`token`: …），
    # 分隔符 = : => := 与全角冒号，值不限长度。
    # 值以左括号开头且其后是空白或引号时不算（JSON 里 "set_token": {"a": 1} 这样的结构）。
    # 分隔符后面若先是加粗的结束标记（**password:** 值），标记连同值一起遮：只遮标记的话值就留下了。
    re.compile(r"(?i)(?P<keep>(?:" + _STRONG + "|" + _WEAK_SUFFIX + r""")(?:\\?['"]|\*{1,2}|_{1,2}|`){0,3}"""
               r"""[ \t]*(?:=>|:=|[:=：])[ \t]*)"""
               r"(?![\[{(](?:[\s'\"]|$))(?:(?:\*{1,2}|__|`)[ \t]+)?" + _VAL),
    re.compile(r"(?P<keep>" + _WEAK_BARE + r"[ \t]*(?:=>|:=|=)[ \t]*)" + _VAL),
    # 原有的键值规则原样保留（分隔符两侧可跨行，值至少 6 位）：新规则只在同一行内找值。
    re.compile(r"(?i)(?P<keep>(?:api[_-]?key|secret|token|passwd|password|private[_-]?key|access[_-]?key)"
               r"\s*[:=]\s*)(?P<lq>['\"]?)[^\s'\"]{6,}(?P=lq)"),
    re.compile(r"\bAKIA[0-9A-Z]{16}\b"),                      # AWS access key id
    re.compile(r"\bgh[pousr]_[A-Za-z0-9]{36,}\b"),             # GitHub token
    re.compile(r"\bsk-[A-Za-z0-9_\-]{20,}\b"),                 # 常见 sk- 前缀密钥
    re.compile(r"\beyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\b"),  # JWT
    re.compile(r"\bxox[abeoprs]-[A-Za-z0-9-]{10,}"),           # Slack
    re.compile(r"\b[spr]k_(?:live|test)_[A-Za-z0-9]{10,}"),    # Stripe
    re.compile(r"\bgithub_pat_[A-Za-z0-9_]{20,}"),             # GitHub fine-grained token
    re.compile(r"\bglpat-[A-Za-z0-9_\-]{20,}"),                # GitLab token
]


def redact(text: str) -> str:
    out = text
    for pat in SECRET_PATTERNS:
        if "keep" in pat.groupindex:
            out = pat.sub(lambda m: m.group("keep") + "«REDACTED»", out)
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


def _config_candidates() -> Tuple[Path, ...]:
    root = project_root()
    return (root / "arcforge.config.json", root / ".claude" / "arcforge.config.json")


def load_config() -> Dict[str, Any]:
    """读取 arcforge.config.json 的 "jev" 段；环境变量 ARCFORGE_JEV_MODE 可临时覆盖两个闸门的 mode。"""
    cfg = dict(DEFAULT_CONFIG)
    for candidate in _config_candidates():
        if candidate.exists():
            try:
                cfg = _deep_merge(cfg, json.loads(candidate.read_text()).get("jev", {}))
            except (OSError, json.JSONDecodeError) as e:
                log_error("config", f"{candidate}: {e}")
            break
    gate = cfg["quality_gate"]
    if not isinstance(gate, dict):
        # quality_gate 写成了字符串之类:出声后原样返回,不在这里抛——本函数落在失败放行的路径上,
        # 多一个抛出点等于把「配置写坏了」从放行变成 hook 崩溃。下面两段都要对它取键,故提前返回。
        log_error("config", f"jev.quality_gate={gate!r} 不是对象，mode 校验与 ARCFORGE_JEV_MODE 覆盖均已跳过")
        return cfg
    # 配置文件里的 mode 拼错不得静默:"Enforce" / "enfoce" 既非 off 也非 enforce,不校验就永久
    # 退化为 shadow,闸门看起来装好了却什么都不做。回落到 shadow 而不是 off——不阻断,但照常记录。
    mode = gate.get("mode")
    if mode not in ("off", "shadow", "enforce"):
        log_error("config", f"jev.quality_gate.mode={mode!r} 非法（可选 off / shadow / enforce），已按 shadow 处理")
        gate["mode"] = "shadow"
    override = os.environ.get("ARCFORGE_JEV_MODE")
    if override in ("off", "shadow", "enforce"):
        # 只覆盖 quality_gate：risk_gate 已从 DEFAULT_CONFIG 移除，留着这一行会在
        # **任何设了 ARCFORGE_JEV_MODE 的人**身上抛 KeyError。而只断言
        # `assertNotIn("risk_gate", DEFAULT_CONFIG)` 的测试根本不执行这条路径，
        # 删字典不删这行会全绿着把缺陷放过去（TASK-009 的 error_handling[0] 钉的就是这个）。
        cfg["quality_gate"]["mode"] = override
    return cfg


def _positive_int(v: Any) -> bool:
    # bool 必须单独排除：True 是 int 的子类，不排除的话 "max_rework": true 会被读成上限 1。
    return isinstance(v, int) and not isinstance(v, bool) and v > 0


def resolve_max_rework(thresholds: Dict[str, Any]) -> Optional[int]:
    """返工上限的唯一来源是 arcforge.config.json **顶层** max_rework。

    写通道与 Leader 用的是顶层那个值；闸门若另读 jev 段的值，两套上限会先后触发 ——
    下游 loom 顶层为 8、jev 段缺省 3，同一任务写通道认为还能返工，Jev 已转人类。
    优先级：顶层合法值 > jev.quality_gate.thresholds.max_rework 合法值 > None（调用方用默认）。
    非法值与两处不一致都记 errors 日志后继续 —— 配置问题不能卡住开发，但也不能静默。
    """
    top = None
    for candidate in _config_candidates():
        if candidate.exists():
            try:
                top = json.loads(candidate.read_text()).get("max_rework")
            except (OSError, json.JSONDecodeError, AttributeError):
                top = None   # 文件本身的问题已由 load_config() 记过
            break
    local = thresholds.get("max_rework")
    if top is not None and not _positive_int(top):
        log_error("config", f"顶层 max_rework={top!r} 非法（须为正整数），已忽略")
        top = None
    if local is not None and not _positive_int(local):
        log_error("config", f"jev.quality_gate.thresholds.max_rework={local!r} 非法（须为正整数），已忽略")
        local = None
    if top is not None:
        if local is not None and local != top:
            log_error("config", f"jev.quality_gate.thresholds.max_rework={local} 与顶层 "
                                f"max_rework={top} 不一致，以顶层为准")
        return top
    return local


def project_allowed(cfg: Dict[str, Any]) -> bool:
    """数据出境白名单（ADR-JEV-05）。空列表 = 不允许任何项目。"""
    allowed = cfg.get("allowed_projects") or []
    if "*" in allowed:
        return True
    root = project_root()
    names = {root.name} | _remote_urls(root)
    return any(a in n for a in allowed for n in names)


def _remote_urls(root: Path) -> Set[str]:
    """root 所在仓库的全部 remote URL。

    经 `git config` 读而不是直接读 `<root>/.git/config`:linked worktree 的 `.git` 是**文件**
    (`gitdir: …`),直接读必然 OSError,于是只剩目录名(如 wt-TASK-001)参与白名单匹配 ⇒
    判「不允许」⇒ Jev 静默返回、一行日志都不写(twine 2026-10-08 实测:主仓库 allowed=True、
    同一份配置在 worktree 里 allowed=False)。git 会自行解析 worktree 的 commondir。
    git 不可用 / 非 git 仓库时退回读文件,行为与改前一致。
    """
    try:
        r = subprocess.run(["git", "-C", str(root), "config", "--get-regexp", r"^remote\..*\.url$"],
                           capture_output=True, text=True, timeout=5)
        if r.returncode == 0:
            return {ln.split(None, 1)[1] for ln in r.stdout.splitlines() if " " in ln}
    except (OSError, subprocess.TimeoutExpired):
        pass
    try:
        return set(re.findall(r"url\s*=\s*(\S+)", (root / ".git" / "config").read_text()))
    except OSError:
        return set()


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
    body = json.dumps({"model": cfg.get("model", DEFAULT_MODEL),
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
