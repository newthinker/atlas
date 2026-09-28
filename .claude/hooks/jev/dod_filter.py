"""
dod_filter — DoD 条目的可判定性筛。

Jev 只判 verify_by: review 的条目，而归档数据显示这类条目里 18.2% 不是命题：
整段交付流程、用分号串起来的三四个断言、几百字的说明。把它们发给 Jev 只会
得到没有意义的概率值。这里先筛一道。

口径与语料（不带口径的百分比就是下一个没人能复算的数）：2026-09-20 用本模块的
judge() 实测 .arcforge/archive/ 下 107 个归档任务的 154 条 verify_by:review 条目，
28 条不可判定 = 18.2%，分布 compound 11 / too_long 9 / multiline 7 / empty 1，
too_long 那 9 条里最长 403 字，而不可判定条目中真正最长的一条有 2309 字
——它含换行，按判定顺序先命中 multiline。换分母会得到别的数：全部 DoD 条目 27.8%(205/737)、
仅对象形态条目 36.3%(190/524)、任务级 17.8%(19/107)或 29.7%(19/64)。复算见
discoveries/TASK-001.json 的 verification.recount。

**口径必须与 validator/dod_judgeability.go 逐字一致**，两侧由
tests/fixtures/dod-judgeability/cases.json 表驱动钉住。改判据时两边一起改。
"""
from __future__ import annotations

JUDGE_OK = "ok"
JUDGE_EMPTY = "empty"
JUDGE_MULTILINE = "multiline"
JUDGE_TOO_LONG = "too_long"
JUDGE_COMPOUND = "compound"

SEMICOLONS = ("；", ";")


def judge(desc: str, max_chars: int = 200, max_semicolons: int = 1) -> str:
    """返回 reason code。JUDGE_OK 表示这条可以发给 Jev。

    判定顺序固定为 empty → multiline → too_long → compound，先命中者返回。
    顺序是契约的一部分：一条既超长又复合的条目，两侧实现必须给出同一个 code，
    否则 validator 的告警文案会与闸门的 skip 理由对不上。
    """
    if not desc or not desc.strip():
        return JUDGE_EMPTY
    if "\n" in desc or "\r" in desc:
        return JUDGE_MULTILINE
    # 按 Unicode 码点计数。len(bytes) 会让中文按 3 倍算，200 字的中文条目全被误判。
    if len(desc) > max_chars:
        return JUDGE_TOO_LONG
    if sum(desc.count(s) for s in SEMICOLONS) > max_semicolons:
        return JUDGE_COMPOUND
    return JUDGE_OK
