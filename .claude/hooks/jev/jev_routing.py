"""
jev_routing — Arcforge 的 Jev 路由策略（纯函数，无 I/O）

两个决策函数：
  route_task(...)        质量闸门：TaskCompleted 时把任务路由到 qa / rework / human
  decide_permission(...) 风险闸门：PermissionRequest 时给出 allow / deny / pass（交给人）

设计约束（见 ADR-JEV-02、ADR-JEV-03）：
  * 纯函数：输入 = Jev 回答 + 任务状态 + 阈值；输出 = 决策 + 理由。便于单测与离线回放。
  * 永远不把"Jev 不确定"和"任务明确失败"混为一谈。
  * 完整性问题（测试篡改、危险代码）直接找人，不交给 agent 自修。
  * 风险闸门只能在"本来就要问人"的区间里代人批准；never_auto_allow 命中时一律交给人。
"""
from __future__ import annotations

from dataclasses import dataclass, field, asdict
from typing import Any, Dict, List, Optional

# ---------------------------------------------------------------------------
# 质量闸门
# ---------------------------------------------------------------------------

ROUTE_QA = "qa"              # 进入正常 QA（Reality Checker + 对抗评审）
ROUTE_REWORK = "rework"      # 明确失败：带失败条目退回原 Dev
ROUTE_HUMAN = "human"        # 人工介入

# 三个路由，不是五个。集成包另有 review / escalate 两级，等于自建一条与 arcforge 的
# code_review.max_iterations + 对抗评审平行的升级链；自建的前提是 Jev 判得准，
# 而那正是影子期尚未验证的。未验证的能力不该先建分支。
ALL_ROUTES = (ROUTE_QA, ROUTE_REWORK, ROUTE_HUMAN)


@dataclass
class QualityThresholds:
    pass_min: float = 0.90          # 每条 DoD 的 noul 都 >= 此值才算"看起来达成"
    fail_max: float = 0.70          # 任一 DoD 的 noul < 此值视为"该条未达成"
    verdict_pass_conf: float = 0.60  # verdict=pass 时要求的最低 confidence
    clear_fail_conf: float = 0.70   # verdict=needs_work 且 confidence >= 此值 → 明确失败
    integrity_max: float = 0.80     # test_tampering / unsafe 达到此值 → 直接找人
    # 返工上限复用 arcforge.config.json **顶层**的 max_rework（由 jev_common.resolve_max_rework
    # 解析后注入；3 只是两处都没配时的默认），读任务 JSON 的 rework_count 来比。
    # 不自建计数：两套阈值会先后触发，Leader 要面对两种「次数用尽」。
    max_rework: int = 3

    @classmethod
    def from_dict(cls, d: Optional[Dict[str, Any]]) -> "QualityThresholds":
        d = d or {}
        known = {k: d[k] for k in cls.__dataclass_fields__ if k in d}
        return cls(**known)


@dataclass
class QualitySignals:
    """从 Jev 响应中抽出的信号。缺失的问题用 None 表示（不做臆测）。"""
    dod: Dict[str, float]                 # dod_key -> P(已实现)
    test_tampering: Optional[float] = None
    unsafe: Optional[float] = None
    verdict: Optional[str] = None         # pass / needs_work / unclear
    verdict_conf: Optional[float] = None

    @property
    def min_dod(self) -> Optional[float]:
        return min(self.dod.values()) if self.dod else None

    def failing_items(self, below: float) -> List[str]:
        return sorted([k for k, p in self.dod.items() if p < below], key=lambda k: self.dod[k])


@dataclass
class Decision:
    route: str
    reason: str
    failing: List[str] = field(default_factory=list)
    flags: List[str] = field(default_factory=list)
    # 映射到 arcforge 既有的 reason_class 四枚举，不自造新词。hook 没有实例身份、
    # 走不了写通道，所以这只是**建议值**，由人在 transition 时自己填。
    reason_class: Optional[str] = None

    def to_dict(self) -> Dict[str, Any]:
        return asdict(self)


def parse_quality_answers(answers: Dict[str, Any]) -> QualitySignals:
    """把 Jev 的 answers 字典转成 QualitySignals。只认 dod_* / test_tampering / unsafe / verdict。"""
    def noul(answer: Any) -> Optional[float]:
        """noul 型答案的唯一识别口径：形如 {"noul": 0.9}，其余一律 None（不臆测）。

        值不是数字时同样返回 None（TASK-012）：`float()` 对 "high" 抛 ValueError、
        对 [1] 抛 TypeError，而**远端返回字符串而非数字是畸形响应里最典型的一种**。
        裸的 float() 会把它变成 main() 的崩溃——虽然 __main__ 的兜底 except 仍让进程
        fail-open，但那与 error_handling「parse_quality_answers 不抛异常」的契约相反。
        只捕这两个具体异常、不用裸 except：裸 except 会吞掉 KeyboardInterrupt 之类，
        也会让「别处坏了」伪装成「answer 格式不对」。
        ⚠ 降级后「畸形」与「没打分」在路由上不可区分（都落到 route_task 的 no_dod 分支），
        诊断性缺口见 TASK-012 的 discovery。
        """
        if isinstance(answer, dict) and answer.get("noul") is not None:
            try:
                return float(answer["noul"])
            except (TypeError, ValueError):
                return None
        return None

    def num(value: Any) -> Optional[float]:
        """裸数值的同口径降级：非数字（含 None、缺键）一律 None。

        为什么单独一个而不复用 noul()：两者拿到的形状不同——noul() 收的是
        `{"noul": x}` 这个信封，num() 收的是已经取出来的裸值。

        为什么不把 noul() 改成调用 num()：那会动到已经验收过的一行，而本次返工的
        范围是「只修 `verdict_conf=` 这一处」。两处各留一个 try/except 是刻意的，
        换来的是一个逐行可核的最小 diff。

        ⚠ 这一处曾是**同一个函数里的第二个裸 float()**，与上面那个相隔 23 行、
        同一种表达式，第一轮只修了一半（test-agent-2 端到端复现后退回）。
        教训是：修「某类表达式」时要把该函数**全文**扫一遍，而不是修到报错那一处为止。
        """
        try:
            return float(value)
        except (TypeError, ValueError):
            return None

    dod: Dict[str, float] = {}
    for k, a in answers.items():
        if not k.startswith("dod_"):
            continue
        p = noul(a)
        if p is not None:
            dod[k] = p

    verdict = answers.get("verdict")
    if not isinstance(verdict, dict):
        verdict = {}
    # 不再解析 scope_creep：scope 漂移已被 task-completed.sh 确定性阻断，
    # 能走到 Jev 说明已经通过，再概率性猜一遍只会产出与硬闸门矛盾的结论。
    return QualitySignals(
        dod=dod,
        test_tampering=noul(answers.get("test_tampering")),
        unsafe=noul(answers.get("unsafe")),
        verdict=verdict.get("choice"),
        verdict_conf=num(verdict.get("confidence")),
    )


def route_task(sig: QualitySignals, rework_count: int, th: QualityThresholds) -> Decision:
    """质量闸门的核心路由。顺序即优先级，先命中者生效。

    三个路由，不是五个。集成包在「Jev 判不准」时路由到 review/escalate，等于自建
    一条与 arcforge 的 code_review.max_iterations + 对抗评审平行的升级链；
    自建的前提是 Jev 判得准，而这正是影子期尚未验证的。未验证的能力不该先建分支。

    reason_class 只在通向 human/rework 的分支上给出，且只取 arcforge 既有的枚举值。
    """
    flags: List[str] = []

    # 1. 完整性问题：不能交给 agent 自己修。优先于其余全部分支 —— 包括「没有可判定
    #    条目」和「返工次数用尽」，因为那两者都会给出一个不指向作弊的结论。
    if sig.test_tampering is not None and sig.test_tampering >= th.integrity_max:
        flags.append("test_tampering")
    if sig.unsafe is not None and sig.unsafe >= th.integrity_max:
        flags.append("unsafe")
    if flags:
        return Decision(ROUTE_HUMAN, "疑似完整性问题：" + "、".join(flags) + "，需人工确认",
                        flags=flags, reason_class="dod_defect")

    # 2. 没有可判定的 DoD：没有判断依据，不拦。排在返工用尽之前 ——
    #    拿不出依据时，返工次数再多也不该由 Jev 把任务推给人。
    if not sig.dod:
        return Decision(ROUTE_QA, "无可判定的 review 条目，直接进入 QA", flags=["no_dod"])

    # 3. 返工次数用尽：返工机制本身没能解决问题，转人工。
    #    读任务 JSON 既有的 rework_count，不自建计数。
    if rework_count >= th.max_rework:
        return Decision(ROUTE_HUMAN, f"已返工 {rework_count} 次（上限 {th.max_rework}）仍未通过",
                        failing=sig.failing_items(th.pass_min), reason_class="dod_defect")

    min_dod = sig.min_dod
    # 4. 看起来通过：照常进 QA（闸门不替代 QA，只在它之前挡明确的失败）
    if (min_dod is not None and min_dod >= th.pass_min
            and sig.verdict == "pass"
            and (sig.verdict_conf or 0.0) >= th.verdict_pass_conf):
        return Decision(ROUTE_QA, f"全部条目 ≥ {th.pass_min}，verdict=pass")

    # 5. 明确失败：Jev 确定没做好，带失败条目退回原 Dev。
    #    三个条件缺一不可 —— 只有 verdict 说 needs_work 而没有一条条目低于 fail_max 时，
    #    退回去的会是一个没有具体条目的返工要求。
    failing = sig.failing_items(th.fail_max)
    if (failing and sig.verdict == "needs_work"
            and (sig.verdict_conf or 0.0) >= th.clear_fail_conf):
        return Decision(ROUTE_REWORK, f"{len(failing)} 条 review 条目明确未达成（< {th.fail_max}）",
                        failing=failing, reason_class="task_defect")

    # 6. 其余都是「Jev 判不准」→ 放行给 QA。
    #    noul 不带 confidence，「概率低」和「Jev 不确定」是两件事；把后者当成前者来处置，
    #    就是拿一个没有把握的信号去拦一个可能合格的任务。
    return Decision(ROUTE_QA,
                    f"判断不确定（min={min_dod:.2f}, verdict={sig.verdict}），交给 QA",
                    flags=["uncertain"])


# ---------------------------------------------------------------------------
# 风险闸门
# ---------------------------------------------------------------------------

PERM_ALLOW = "allow"   # 代人批准（仅限低风险）
PERM_DENY = "deny"     # 拒绝并把理由反馈给 agent
PERM_PASS = "pass"     # 不做决定，走原有流程（弹窗问人）


@dataclass
class RiskThresholds:
    allow_score_max: float = 0.6     # risk score（0~3）不高于此值才考虑自动放行
    allow_conf_min: float = 0.7      # 且 risk 的 confidence 不低于此值
    hazard_allow_max: float = 0.2    # 且所有危害 noul 都低于此值
    hazard_deny_min: float = 0.9     # destructive / exfiltration 任一 ≥ 此值 → 直接拒绝

    @classmethod
    def from_dict(cls, d: Optional[Dict[str, Any]]) -> "RiskThresholds":
        d = d or {}
        known = {k: d[k] for k in cls.__dataclass_fields__ if k in d}
        return cls(**known)


HAZARD_KEYS = ("destructive", "exfiltration", "outside_workspace", "privilege")
DENY_KEYS = ("destructive", "exfiltration")


@dataclass
class RiskSignals:
    score: Optional[float] = None
    score_conf: Optional[float] = None
    hazards: Dict[str, float] = field(default_factory=dict)


def parse_risk_answers(answers: Dict[str, Any]) -> RiskSignals:
    risk = answers.get("risk") if isinstance(answers.get("risk"), dict) else {}
    hazards = {}
    for k in HAZARD_KEYS:
        a = answers.get(k)
        if isinstance(a, dict) and a.get("noul") is not None:
            hazards[k] = float(a["noul"])
    return RiskSignals(
        score=float(risk["score"]) if risk.get("score") is not None else None,
        score_conf=float(risk["confidence"]) if risk.get("confidence") is not None else None,
        hazards=hazards,
    )


def decide_permission(sig: RiskSignals, th: RiskThresholds, *, event: str,
                      never_auto_allow_hit: bool = False) -> Decision:
    """
    event = "PermissionRequest"：可以 allow / deny / pass
    event = "PreToolUse"       ：只能 deny / pass（PreToolUse 的 allow 本来也绕不过 ask/deny 规则，
                                 这里干脆不用，避免语义混乱）
    """
    for k in DENY_KEYS:
        p = sig.hazards.get(k)
        if p is not None and p >= th.hazard_deny_min:
            return Decision(PERM_DENY, f"Jev 判定高风险：{k}={p:.2f}", flags=[k])

    if event != "PermissionRequest":
        return Decision(PERM_PASS, "PreToolUse 模式只拦截，不代批")

    if never_auto_allow_hit:
        return Decision(PERM_PASS, "命中 never_auto_allow，保留人工确认", flags=["never_auto_allow"])

    if sig.score is None or sig.score_conf is None:
        return Decision(PERM_PASS, "缺少 risk 评分，交给人")

    hazards_ok = all(sig.hazards.get(k, 1.0) < th.hazard_allow_max for k in HAZARD_KEYS)
    if sig.score <= th.allow_score_max and sig.score_conf >= th.allow_conf_min and hazards_ok:
        return Decision(PERM_ALLOW, f"低风险：score={sig.score:.2f}, conf={sig.score_conf:.2f}")

    return Decision(PERM_PASS, f"风险不够低（score={sig.score:.2f}, conf={sig.score_conf:.2f}），交给人")
