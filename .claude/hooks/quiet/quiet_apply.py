"""
quiet_apply — 执行器:把一条已记录的正面决策经写通道落地。

用法: python3 .claude/hooks/quiet/quiet_apply.py <decision_id> [--assign-to <实例名>]
退出码: 0 已落地 / 2 拒绝(不落地)。

Leader 拿到正面 action 后**只能**经这里落地。落地前重新核对全部前提:记录本身、时间、档位与
放权指纹、决策模型、事实与硬规则。本文件的落地段是全系统唯一「只在放权档下才执行」的代码。

执行器不问决策模型、不拼题目,也不取键锁:它只复用决策入口读事实、算键、跑硬规则的四个函数。
「核对」与「写入」之间的并发改动交给写通道自己的锁去拦(熔断落地带 --expect-epoch)。
"""
from __future__ import annotations

import sys

sys.dont_write_bytecode = True

import json  # noqa: E402
import os  # noqa: E402
import re  # noqa: E402
import signal  # noqa: E402
import subprocess  # noqa: E402
import tempfile  # noqa: E402
import time  # noqa: E402
from datetime import datetime, timezone  # noqa: E402
from pathlib import Path  # noqa: E402
from typing import Any, Dict, List, Optional, Tuple  # noqa: E402

_HERE = str(Path(__file__).resolve().parent)
if _HERE not in sys.path:
    sys.path.insert(0, _HERE)
import quiet_common as qc  # noqa: E402
import quiet_decide as qd  # noqa: E402

deciders = qc.deciders

# 白名单传递:ARCFORGE_SKIP_VALIDATE 等会关掉写通道刹车的变量不得被继承。
WRITE_ENV = ("PATH", "HOME", "LANG", "LC_ALL", "TMPDIR", "ARCFORGE_TOKEN")
# 只查形状(须配 fullmatch:match 加 `$` 会放过串尾换行);「是不是在册实例」由写通道把关。
# 首字符须是字母或数字:以 - 开头的值长得像一个选项。
INSTANCE_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$")
WRITE_TIMEOUT_S = 120
WRITE_REAP_S = 5            # 超时后先 SIGTERM(让写通道的 EXIT trap 放掉自己的锁),等这么久再 SIGKILL
WRITE_FAILED = 125          # 写通道没跑起来或超时:不是它自己的退出码,但同样不是 0


class Refused(Exception):
    """核对未通过:不落地。"""


def _stop_group(proc: "subprocess.Popen[bytes]") -> None:
    """超时后收拾整个进程组:先 SIGTERM——写通道靠 EXIT trap 清它自己的锁目录,直接 SIGKILL 会把锁留下。"""
    for sig in (signal.SIGTERM, signal.SIGKILL):
        try:
            os.killpg(proc.pid, sig)
        except (ProcessLookupError, PermissionError):
            return
        try:
            proc.wait(timeout=WRITE_REAP_S)
            return
        except subprocess.TimeoutExpired:
            continue


def run_write(root: Path, args: List[str], stdin: bytes) -> int:
    """以 leader 身份调写通道,返回它的退出码;跑不起来或超时返回 WRITE_FAILED。**不抛**。

    工作目录固定为主仓库根(写通道用相对路径,在 linked worktree 内会 DENY);环境只含 WRITE_ENV。
    stdin 与 stderr 都走临时文件而不是管道:结果以写通道自己的退出为准,它拉起的进程还占着
    输出不该让这里等满超时、再把一次已经完成的写入报成失败。
    """
    script = root / ".claude" / "hooks" / "arcforge-write.sh"
    env = {k: os.environ[k] for k in WRITE_ENV if k in os.environ}
    try:
        with tempfile.TemporaryFile() as fin, tempfile.TemporaryFile() as ferr:
            fin.write(stdin)
            fin.seek(0)
            if not script.is_file():
                raise OSError("写通道脚本不存在")
            proc = subprocess.Popen(["bash", str(script), "--as", "leader"] + list(args), cwd=str(root), env=env,
                                    stdin=fin, stdout=subprocess.DEVNULL, stderr=ferr, start_new_session=True)
            try:
                rc = int(proc.wait(timeout=WRITE_TIMEOUT_S))
            except subprocess.TimeoutExpired:
                _stop_group(proc)
                rc = WRITE_FAILED
            if rc != 0:
                # 写通道的 DENY 文案只打在 stderr;不留痕的话 Leader 只看得到一个 exit 2。
                ferr.seek(max(0, ferr.seek(0, os.SEEK_END) - 4096))
                tail = ferr.read().decode("utf-8", errors="replace")[-260:]
                qc.log_error(root, "quiet_write", f"exit {rc}: {tail}")
            return rc
    except Exception as e:  # noqa: BLE001 —— 拉不起来:OSError;参数里有 NUL 或孤立代理:ValueError 一族
        qc.log_error(root, "quiet_write", f"{type(e).__name__}: {e}")
        return WRITE_FAILED


# ---------------------------------------------------------------------------
# 核对:任一项不满足即抛 Refused
# ---------------------------------------------------------------------------

def _record(root: Path, decision_id: Any) -> Dict[str, Any]:
    """日志里那条记录本身:恰好一行、正面取值、放权档、真实时钟、带着合法且正面的模型意见、尚未落地。"""
    if not isinstance(decision_id, str) or not decision_id:
        raise Refused("decision_id 须为非空字符串")
    rows = qc.read(root, qd.LOG)
    hits = [r for r in rows if r.get("type") == "decision" and r.get("id") == decision_id]
    if len(hits) != 1:
        raise Refused(f"日志中该 decision_id 命中 {len(hits)} 行(须恰好 1 行)")
    rec = hits[0]
    kind = rec.get("kind")
    if not isinstance(kind, str) or kind not in qd.ACTIONS:
        raise Refused("该记录的 kind 不是可落地的类别")
    if rec.get("action") != qd.ACTIONS[kind]:
        raise Refused("该记录不是正面决策")
    if rec.get("mode") != "enforce":
        raise Refused("该记录不是在放权档下产生的")
    if "clock" in rec:
        raise Refused("该记录是在被覆写的时钟下产生的")
    # action 写着正面取值不够——那只是一个字段。记录里的模型意见过一遍与决策入口复用缓存行时相同的校验
    # (题目表只随类别变,不含事实;这里不拼题目、不问模型):没有意见、意见不合法、或选的不是正面选项的行不落地。
    qid, positive = qd.QUESTION[kind]
    try:
        opinion = deciders.validate_answers(qd._questions(kind), {qid: rec.get("opinion")}).answers.get(qid)
    except Exception:  # noqa: BLE001 —— 例:超大整数的置信度让校验自己溢出
        opinion = None
    if opinion is None:
        raise Refused("该记录没有合法的模型意见")
    if opinion["choice"] != positive:
        raise Refused("该记录的模型意见不是正面选项")
    if not isinstance(rec.get("night"), str) or not isinstance(rec.get("key"), str):
        raise Refused("该记录缺少归属的夜晚或键")
    if any(r.get("type") == "applied" and r.get("decision_id") == decision_id for r in rows):
        raise Refused("该决策已落地过")
    return rec


def _same_night(st: qc.Settings, rec: Dict[str, Any]) -> None:
    """此刻仍在该记录归属的那一晚的静默期内,且时钟没有被覆写。核对开始时查一次,写入前再查一次。"""
    if os.environ.get("ARCFORGE_QUIET_NOW"):
        raise Refused("设置了 ARCFORGE_QUIET_NOW(测试用时钟覆写),执行器一律拒绝")
    at, overridden = qc.now()
    if overridden:
        raise Refused("时钟被覆写,执行器一律拒绝")
    if qc.window(st, at) != rec["night"]:
        raise Refused("此刻已不在该决策归属的那一晚的静默期内")


def _check(root: Path, decision_id: Any) -> Tuple[Dict[str, Any], qc.Settings, Dict[str, Any]]:
    """返回 (记录, 当前配置, 此刻的事实);任一项不满足抛 Refused。"""
    if os.environ.get("ARCFORGE_QUIET_NOW"):
        raise Refused("设置了 ARCFORGE_QUIET_NOW(测试用时钟覆写),执行器一律拒绝")
    rec = _record(root, decision_id)
    kind = rec["kind"]
    try:
        st = qc.load(root)
    except qc.ConfigError as e:
        raise Refused(f"配置非法: {e}") from e
    if st is None or not st.enabled:
        raise Refused("功能已关闭")
    # 门槛取**此刻**配置的:同一晚里人调高了门槛并重新授权之后,按旧门槛产生、尚未落地的正面决策不再够格
    # (决策模型的指纹没变,下面的指纹与档位核对都拦不住它)。意见的形状已在 _record 里校验过。
    if rec["opinion"]["confidence"] < st.min_confidence:
        raise Refused("该记录的置信度低于当前配置的门槛")
    _same_night(st, rec)
    fp = deciders.fingerprint(st.decider, st.config)
    if rec.get("decider") != {"name": st.decider, "fp": fp}:
        raise Refused("当前决策模型(名称或指纹)与记录不一致")
    if qc.effective_mode(st, kind, fp) != ("enforce", False):
        raise Refused("该类当前不在放权档,或放权指纹不符")
    if deciders.available(st.decider, st.config, root) is not None:
        raise Refused("决策模型当前不可用")
    task_id = rec.get("task")
    try:
        if qd.key_of(kind, root, task_id) != rec["key"]:
            raise Refused("任务或 DoD 在决策之后变了")
        facts = qd.read_facts(kind, root, task_id, st)
    except qc.FactsError as e:
        raise Refused(f"事实读不到: {e}") from e
    if qd.facts_key(kind, facts) != rec["key"]:
        raise Refused("任务或 DoD 在决策之后变了")
    rule = qd.hit_rule(kind, facts)
    if rule is not None:
        raise Refused(f"硬规则此刻命中: {rule}")
    return rec, st, facts


def _landing(rec: Dict[str, Any], st: qc.Settings, facts: Dict[str, Any], decision_id: str,
             assign_to: Any) -> Tuple[List[str], bytes]:
    """写通道的参数与 stdin。本函数与随后的那一次 run_write 合起来就是落地段。"""
    if rec["kind"] == "blocked_human":
        if not isinstance(assign_to, str) or not INSTANCE_RE.fullmatch(assign_to):
            raise Refused("--assign-to 必填,且须为合法实例名")
        marker = json.dumps({"night": rec["night"], "decision_id": decision_id, "decider": st.decider},
                            ensure_ascii=False, separators=(",", ":"))
        # --expect-epoch:核对与写入之间任务若被并发重派(epoch 已变),写通道在自己的锁里 DENY。
        # 不带的话这次写入走的是 assigned → assigned——leader 的合法边,会把别人刚选的人盖掉。
        return (["task", rec["task"], "transition", "assigned",
                 "--field", f"assigned_to={assign_to}",
                 "--json-field", f"auto_redispatch={marker}",
                 "--expect-epoch", str(facts["assignment_epoch"])], b"")
    release = {"by": f"decider:{st.decider}", "decision_id": decision_id, "dod_sha256": rec["key"],
               "at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")}
    return ["doc", "docs/" + qd.RELEASE], json.dumps(release, ensure_ascii=False, indent=2).encode("utf-8")


def _marker_on_disk(root: Path, rec: Dict[str, Any], decision_id: str) -> bool:
    """磁盘上是否已有**这条决策**的落地标记。**只用于给拒绝原因添一句话,不参与任何判定**:
    标记说明这条决策落过地,说明不了是不是这一次调用落的(同一个 decision_id 被调两次时,
    后到者看到的正是先到者的标记)。把「写通道没给出 0」解释成成功是简报(只读)交叉核对的事。"""
    try:
        if rec["kind"] == "blocked_human":
            path, field = root / ".arcforge" / "tasks" / f"{rec['task']}.json", "auto_redispatch"
        else:
            path, field = root / ".arcforge" / "docs" / qd.RELEASE, None
        if path.is_symlink() or not path.is_file():
            return False
        doc = json.loads(path.read_bytes())
        mark = doc.get(field) if field else doc
        return isinstance(mark, dict) and mark.get("decision_id") == decision_id
    except Exception:  # noqa: BLE001 —— 读不出来 = 没有证据表明已落地
        return False


def apply(decision_id: str, assign_to: Optional[str] = None,
          cwd: Optional[Path] = None) -> Tuple[int, str]:
    """(0, "applied") 或 (2, 拒绝原因)。对可预见的输入不抛。"""
    try:
        root = qc.main_root(cwd)
    except (qc.RootError, OSError, ValueError, TypeError) as e:    # 当前目录已被删除;cwd 不是可用的路径
        return 2, f"主仓库根不可决断: {e}"

    def refuse(why: str) -> Tuple[int, str]:
        qc.log_error(root, "quiet_apply", f"{str(decision_id)[:80]}: {why}")
        return 2, why

    try:
        rec, st, facts = _check(root, decision_id)
        args, stdin = _landing(rec, st, facts, decision_id, assign_to)
        # 核对要花时间(读事实、前置,确认门还要跑一遍校验器):紧挨着写入再看一次窗口。
        _same_night(st, rec)
    except Refused as e:
        return refuse(str(e))
    except Exception as e:  # noqa: BLE001 —— 核对过程里任何一步出错都是「未落地」
        return refuse(f"核对出错: {type(e).__name__}: {e}")

    # ---- 落地:全系统唯一只在放权档下执行的一次写入 ----
    rc = run_write(root, args, stdin)
    if rc != 0:
        # 写通道没给出 0 = 没有落地确认:一律拒绝、不写 applied 行。磁盘上若已是本决策的标记
        # (别的调用先落了地,或这一次写完之后才超时),只把这一事实记下来供人排查。
        seen = ";磁盘上已见本决策的标记,是否落地以简报的交叉核对为准" if _marker_on_disk(root, rec, decision_id) else ""
        return refuse(f"写通道 exit {rc},未确认落地{seen}")

    try:
        qc.append(root, qd.LOG, {"type": "applied", "decision_id": decision_id, "night": rec["night"],
                                 "kind": rec["kind"], "task": rec.get("task"), "ts": round(time.time(), 1)})
    except qc.LogError as e:
        # 状态已经迁移,不能回头说没落地;简报对照 transitions.jsonl / release 文件会标红这次缺行。
        qc.log_error(root, "quiet_apply", f"{decision_id}: 已落地但 applied 行写入失败: {e}")
    return 0, "applied"


def _say(stream: Any, message: str) -> None:
    """输出不得改变退出码:调用方不读输出时写与收尾的 flush 都会失败,解释器会把退出码改成 120。"""
    try:
        print(message, file=stream, flush=True)
    except Exception:  # noqa: BLE001
        try:
            os.dup2(os.open(os.devnull, os.O_WRONLY), stream.fileno())     # 让收尾时的 flush 有处可去
        except Exception:  # noqa: BLE001
            pass


def main(argv: List[str]) -> int:
    try:
        if len(argv) == 1:
            decision_id, assign_to = argv[0], None
        elif len(argv) == 3 and argv[1] == "--assign-to":
            decision_id, assign_to = argv[0], argv[2]
        else:
            _say(sys.stderr, "用法: quiet_apply.py <decision_id> [--assign-to <实例名>]")
            return 2
        rc, message = apply(decision_id, assign_to)
    except Exception as e:  # noqa: BLE001 —— 任何异常都是「未落地」
        _say(sys.stderr, f"quiet_apply 异常,未落地: {type(e).__name__}: {e}")
        return 2
    if rc != 0:
        _say(sys.stderr, message)
        return 2
    _say(sys.stdout, message)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
