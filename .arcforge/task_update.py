#!/usr/bin/env python3
"""在 with-task-lock.sh 临界区内原子更新 task JSON 字段。
用法: task_update.py <tasks_dir> <TASK-ID> field=json_value ...
值先按 JSON 解析，失败则按字符串。写临时文件后 rename。"""
import json, os, sys

tasks_dir, tid = sys.argv[1], sys.argv[2]
path = os.path.join(tasks_dir, tid + ".json")
with open(path) as f:
    t = json.load(f)
for kv in sys.argv[3:]:
    k, v = kv.split("=", 1)
    try:
        t[k] = json.loads(v)
    except json.JSONDecodeError:
        t[k] = v
tmp = path + ".tmp"
with open(tmp, "w") as f:
    json.dump(t, f, ensure_ascii=False, indent=2)
    f.write("\n")
os.rename(tmp, path)
print(f"{tid}: " + ", ".join(sys.argv[3:]))
