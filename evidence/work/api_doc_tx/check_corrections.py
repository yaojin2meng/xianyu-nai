# -*- coding: utf-8 -*-
"""8 条「本次修正点」契约检查：基线版必有多条 FAIL，修正版应全 PASS。"""
import io, os, sys, hashlib

d = sys.argv[1]
p = os.path.join(d, "API文档-NAI画图接口.md")
if not os.path.exists(p):
    print("RESULT=DOC_ABSENT")
    sys.exit(3)

s = io.open(p, encoding="utf-8").read()

checks = [
    ("C1_baidu_field_imgUrl", '{"type":"baidu","imgUrl"' in s),
    ("C2_no_wrong_six_float", '"seed" : 1833993695' not in s),
    ("C3_timeout_line_129", "client.go:129" in s and "client.go:128" not in s),
    ("C4_latency_range", "6.0–16.4 秒" in s),
    ("C5_two_hardcoded_domains", "硬编码的两个 `xianyuw.cn` 域名" in s),
    ("C6_augment_six_fields", '"defry":0' in s),
    ("C7_pseudo_exit0", "exit=0" in s and "假服务器那 6 个用例的 HTTP 状态码" in s),
    ("C8_reprint_note_src", "nai_ini.go:48-50" in s and "client.go:48-50" not in s),
    ("C9_status_code_wording", "`200` 请求成功" in s and "所有API接口都不可用" in s and "所有上游不可用" not in s),
]

print("RESULT=DOC_PRESENT")
print("BYTES=%d" % len(s.encode("utf-8")))
print("SHA256=%s" % hashlib.sha256(s.encode("utf-8")).hexdigest())
npass = 0
for name, ok in checks:
    print("%s=%s" % (name, "PASS" if ok else "FAIL"))
    npass += 1 if ok else 0
print("PASS=%d/%d" % (npass, len(checks)))
print("ALL_CHECKS_PASS=%s" % (npass == len(checks)))
sys.exit(0 if npass == len(checks) else 5)
