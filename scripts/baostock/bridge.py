#!/usr/bin/env python3
"""Baostock HTTP 桥:GET /daily?code=sh.600519&start=2016-01-01&end=2026-08-02
→ JSON [{"date":"2026-08-01","close":1350.6}, ...]。仅绑 127.0.0.1:8181。
复权口径:adjustflag=3(不复权,与 PE 计算一致)。"""
import json
import urllib.parse
from http.server import BaseHTTPRequestHandler, HTTPServer

import baostock as bs

bs.login()  # baostock 匿名登录,进程生命周期内复用


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        u = urllib.parse.urlparse(self.path)
        if u.path != "/daily":
            self.send_response(404)
            self.end_headers()
            return
        q = urllib.parse.parse_qs(u.query)
        try:
            rs = bs.query_history_k_data_plus(
                q["code"][0], "date,close",
                start_date=q["start"][0], end_date=q["end"][0],
                frequency="d", adjustflag="3")
            # baostock 查询失败不抛异常,只把 error_code 挂在结果集上,rs.next() 直接为假。
            # 不显式检查就会把上游故障静默返回成 200 [] —— 下游会误判为「该期无数据」而
            # 不触发降级链下一跳(spec §2:临时错误必须往下跳)。
            if rs.error_code != "0":
                raise RuntimeError(f"baostock query {rs.error_code}: {rs.error_msg}")
            rows = []
            while rs.next():
                r = rs.get_row_data()
                if r[1]:
                    rows.append({"date": r[0], "close": float(r[1])})
            body = json.dumps(rows).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(body)
        except Exception as e:  # noqa: BLE001 — 桥的全部错误面都转 500 文本
            self.send_response(500)
            self.end_headers()
            self.wfile.write(str(e).encode())

    def log_message(self, *a):  # 静默默认访问日志,launchd 只收错误
        pass


if __name__ == "__main__":
    HTTPServer(("127.0.0.1", 8181), Handler).serve_forever()
