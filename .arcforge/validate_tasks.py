#!/usr/bin/env python3
"""手工降级版 arcforge-validate：DAG 无环 / wave 序 / 依赖存在 / context_from 闭合 /
packages 非空 + 潜在并发任务 scope 互斥 / 状态-owner 不变量（拆分期基线）。"""
import json, sys, glob, os

tasks = {}
for p in sorted(glob.glob(os.path.join(sys.argv[1], "TASK-*.json"))):
    with open(p) as f:
        t = json.load(f)
    tasks[t["id"]] = t

errs = []

# 依赖存在 + context_from 闭合（context_from ⊆ 已定义任务）
for tid, t in tasks.items():
    for d in t["dependencies"]:
        if d not in tasks:
            errs.append(f"{tid}: 依赖 {d} 不存在")
    for c in t["context_from"]:
        if c not in tasks:
            errs.append(f"{tid}: context_from {c} 不存在")

# DAG 无环（DFS）
WHITE, GRAY, BLACK = 0, 1, 2
color = {tid: WHITE for tid in tasks}
def dfs(u, stack):
    color[u] = GRAY
    for v in tasks[u]["dependencies"]:
        if v not in tasks: continue
        if color[v] == GRAY:
            errs.append(f"环: {' -> '.join(stack + [u, v])}")
        elif color[v] == WHITE:
            dfs(v, stack + [u])
    color[u] = BLACK
for tid in tasks:
    if color[tid] == WHITE:
        dfs(tid, [])

# wave 序：本任务.wave > max(依赖.wave)
for tid, t in tasks.items():
    for d in t["dependencies"]:
        if d in tasks and t["wave"] <= tasks[d]["wave"]:
            errs.append(f"{tid}: wave {t['wave']} 未大于依赖 {d} 的 wave {tasks[d]['wave']}")

# packages 非空
for tid, t in tasks.items():
    if not t.get("packages"):
        errs.append(f"{tid}: packages 为空")

# 潜在并发对（无祖先-后代关系）scope 互斥
def ancestors(tid, seen=None):
    seen = seen or set()
    for d in tasks[tid]["dependencies"]:
        if d in tasks and d not in seen:
            seen.add(d); ancestors(d, seen)
    return seen
anc = {tid: ancestors(tid) for tid in tasks}
ids = sorted(tasks)
for i, a in enumerate(ids):
    for b in ids[i+1:]:
        if a in anc[b] or b in anc[a]:
            continue  # 有序关系，永不并发
        overlap = set(tasks[a]["packages"]) & set(tasks[b]["packages"])
        if overlap:
            errs.append(f"{a} 与 {b} 可能并发且 packages 交叠: {sorted(overlap)}")

# 状态机不变量（全阶段通用）
VALID = {"pending","assigned","in_progress","dev_done","verifying","verified",
         "rejected","review_fix","blocked_clarification","blocked_human","accepted","skipped"}
OWNED = {"assigned","in_progress","dev_done","verifying","verified","accepted"}  # 有归属者的状态
DONE = {"verified","accepted"}  # 完成态
for tid, t in tasks.items():
    st = t["status"]
    if st not in VALID:
        errs.append(f"{tid}: 非法 status {st}")
    if st in OWNED and not t["assigned_to"]:
        errs.append(f"{tid}: status={st} 但 assigned_to 为空")
    if st in OWNED and t["assignment_epoch"] < 1:
        errs.append(f"{tid}: status={st} 但 epoch<1")
    if st in ("verifying","verified","accepted") and not t.get("verifier"):
        errs.append(f"{tid}: status={st} 但 verifier 为空")
    if t["rework_count"] > 3:
        errs.append(f"{tid}: rework_count {t['rework_count']} 超过 max_rework=3 应转 blocked_human")
    if st == "blocked_clarification" and not t["questions"]:
        errs.append(f"{tid}: blocked_clarification 必须有 questions")
    # 完成必有产物：verified/accepted 须有 discovery 文件
    if st in DONE and not os.path.exists(t["discovery"]):
        errs.append(f"{tid}: status={st} 但 discovery 文件缺失 {t['discovery']}")
    # 失败必有原因
    if st == "rejected" and not t.get("reject_reason"):
        errs.append(f"{tid}: rejected 必须有 reject_reason")
    # 依赖序：非 pending/assigned 的任务，其依赖必须已 verified/accepted（skip 传播除外）
    if st in OWNED - {"assigned"}:
        for d in t["dependencies"]:
            if d in tasks and tasks[d]["status"] not in DONE:
                errs.append(f"{tid}: 已推进({st}) 但依赖 {d} 状态为 {tasks[d]['status']}")

if errs:
    print("VALIDATION FAILED:")
    for e in errs:
        print(" -", e)
    sys.exit(1)
print(f"VALIDATION OK: {len(tasks)} tasks, DAG 无环, wave 序正确, scope 互斥, packages 非空")
