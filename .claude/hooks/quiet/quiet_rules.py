"""
quiet_rules — 静默期决策的硬规则层。

纯函数:输入是决策入口**自己读到的**事实,输出是命中的规则名或 None。不做 I/O,不问决策模型。
命中任何一条都意味着「留到早上」,而且此时不得调用决策模型。
规则名只用 ASCII、不超过 24 个字符(日志单行有字节上限)。
"""
from __future__ import annotations

from typing import Any, Dict, Optional

VALIDATOR_UNAVAILABLE = "validator_unavailable"
# validator-run.sh 的自产码:124 装了但跑不了 / 125 版本陈旧 / 127 压根没装。
_VALIDATOR_UNAVAILABLE_CODES = (124, 125, 127)

EVIDENCE_FIELDS = ("dod_sha256", "orphan_requirements", "phantom_dod",
                   "reviewer_verdict", "reviewer_gaps", "requirements_path")


def _count(v: Any) -> bool:
    return isinstance(v, int) and not isinstance(v, bool) and v >= 0


def blocked_human(task: Any) -> Optional[str]:
    if not isinstance(task, dict) or task.get("status") != "blocked_human":
        return "not_blocked"
    blocked = task.get("blocked")
    # 只有原因明确写着返工超限的任务才可能交给决策模型(是不是本轮写的,见下面那道检查及其限度)。
    if not isinstance(blocked, dict) or blocked.get("reason") != "rework_limit":
        return "reason_not_rework_limit"
    # 自定义字段落盘后永不清除:第二次熔断时若 Leader 漏写,留着的是上一次的原因。
    # 两次熔断之间必经一次重派(assignment_epoch += 1),所以 Leader **上一次**留下的值对不上。
    # 这道检查只防陈旧值,不防别的写者在**当前** epoch 留下的值:写通道对这个字段不设防,任务 owner
    # 可以先经 update 预写一个对得上的值,此后 Leader 不带该字段转入熔断时,那个值照样通过本条。
    epoch = task.get("assignment_epoch")
    if not _count(epoch) or not _count(blocked.get("epoch")) or blocked["epoch"] != epoch:
        return "blocked_epoch_stale"
    last = task.get("last_transition")
    if not isinstance(last, dict) or last.get("from") != "rejected":
        return "not_from_rejected"
    if task.get("reason_class") == "dod_defect":
        return "dod_defect"
    # 按「键存在」而非「值为真」判:写通道不能删键、只能改值。
    if "auto_redispatch" in task:
        return "already_redispatched"
    return None


def _evidence_ok(e: Any) -> bool:
    if not isinstance(e, dict) or any(f not in e for f in EVIDENCE_FIELDS):
        return False
    return (isinstance(e["dod_sha256"], str) and bool(e["dod_sha256"])
            and _count(e["orphan_requirements"]) and _count(e["phantom_dod"])
            and _count(e["reviewer_gaps"])
            and isinstance(e["reviewer_verdict"], str)
            and isinstance(e["requirements_path"], str) and bool(e["requirements_path"]))


def dod_gate(facts: Dict[str, Any]) -> Optional[str]:
    if facts.get("autonomy") == "full-auto":
        return "full_auto"
    evidence = facts.get("evidence")
    if not _evidence_ok(evidence):
        return "evidence_invalid"
    if evidence["dod_sha256"] != facts.get("dod_sha256"):
        return "evidence_stale"
    if evidence["orphan_requirements"] > 0 or evidence["phantom_dod"] > 0:
        return "orphan_or_phantom"
    if evidence["reviewer_verdict"] != "sufficient" or evidence["reviewer_gaps"] > 0:
        return "reviewer_insufficient"
    rc = facts.get("validator_rc")
    if rc is None or rc in _VALIDATOR_UNAVAILABLE_CODES:
        return VALIDATOR_UNAVAILABLE
    if rc != 0:
        return "validator_failed"
    return None
