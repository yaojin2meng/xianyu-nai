# -*- coding: utf-8 -*-
"""api_doc_tx 最终重建：MODIFIED_FILE / DIFF / 三态契约检查 / ROLLBACK 实测。"""
import io, os, re, json, shutil, hashlib, difflib, subprocess

REPO = r"C:\Users\Administrator\Desktop\nai\xianyu-nai"
D = r"C:\Users\Administrator\Desktop\nai\work\api_doc_tx"
DOC = "API\u6587\u6863-NAI\u753b\u56fe\u63a5\u53e3.md"
MD = os.path.join(D, "MODIFIED_FILE_API-doc.md")
BL = os.path.join(D, "baseline_doc.md")
DIFF = os.path.join(D, "DIFF_FILE.diff")
CHK = os.path.join(D, "check_corrections.py")


def sha(p):
    return hashlib.sha256(io.open(p, "rb").read()).hexdigest()


def rd(p):
    return io.open(p, encoding="utf-8", newline="").read()


def wr(p, s):
    io.open(p, "w", encoding="utf-8", newline="").write(s)


# 1. MODIFIED := repo
shutil.copy2(os.path.join(REPO, DOC), MD)
cur_b = io.open(MD, "rb").read()
print("MODIFIED bytes=%d sha=%s lines=%d" % (
    len(cur_b), sha(MD), rd(MD).count("\n") + (0 if rd(MD).endswith("\n") else 1)))

# 2. DIFF
a = rd(BL).splitlines(True)
b = rd(MD).splitlines(True)
d = list(difflib.unified_diff(a, b, fromfile="baseline/" + DOC, tofile="modified/" + DOC, n=3))
wr(DIFF, "".join(d))
print("DIFF bytes=%d hunks=%d plus=%d minus=%d" % (
    os.path.getsize(DIFF), sum(1 for l in d if l.startswith("@@")),
    sum(1 for l in d if l.startswith("+") and not l.startswith("+++")),
    sum(1 for l in d if l.startswith("-") and not l.startswith("---"))))

# 3. patch apply
raw = rd(DIFF).splitlines(True)
i = 0
hunks = []
while i < len(raw):
    m = re.match(r"^@@ -(\d+)(?:,(\d+))? \+\d+(?:,\d+)? @@", raw[i])
    if m:
        hunks.append({"old": int(m.group(1)), "lines": []}); i += 1
        while i < len(raw) and not raw[i].startswith(("@@", "--- ")):
            if raw[i][:1] in (" ", "+", "-"): hunks[-1]["lines"].append(raw[i])
            i += 1
        continue
    i += 1
src = rd(BL).splitlines(True)
out = []; pos = 0; ok = True
for h in hunks:
    st = h["old"] - 1
    out.extend(src[pos:st]); pos = st
    for ln in h["lines"]:
        tag, txt = ln[:1], ln[1:]
        if tag == " ":
            if src[pos] != txt: ok = False
            out.append(txt); pos += 1
        elif tag == "-":
            if src[pos] != txt: ok = False
            pos += 1
        else:
            out.append(txt)
out.extend(src[pos:])
REC = os.path.join(D, "reconstructed_new.md")
wr(REC, "".join(out))
print("PATCH_APPLY_OK=%s equal=%s" % (ok and sha(REC) == sha(MD), sha(REC) == sha(MD)))

# 4. 三态目录
def mk(name, srcp):
    t = os.path.join(D, name)
    if os.path.isdir(t): shutil.rmtree(t)
    os.makedirs(t)
    if srcp: shutil.copy2(srcp, os.path.join(t, DOC))
    return t

tb = mk("target_baseline_doc", BL)
tm = mk("target_modified_doc", MD)
tr = mk("target_rollback_doc", MD)

# 5. ROLLBACK 实测
pre = sha(os.path.join(tr, DOC))
p = subprocess.run(["powershell", "-ExecutionPolicy", "Bypass", "-File",
                    os.path.join(D, "ROLLBACK.ps1"), "-Target", tr], capture_output=True)
post = sha(os.path.join(tr, DOC))
print("ROLLBACK exit=%d stdout=%r" % (p.returncode, p.stdout.decode("utf-8", "replace").strip()))
print("  pre=%s post=%s post==baseline:%s" % (pre[:16], post[:16], post == sha(BL)))

# 6. 三态契约检查
def run(t):
    q = subprocess.run(["python", CHK, t], capture_output=True)
    return q.returncode, q.stdout.decode("utf-8", "replace").strip()

L = []
for tag, t in (("BASELINE", tb), ("MODIFIED", tm), ("ROLLBACK", tr)):
    rc, o = run(t)
    L.append("== %s  exit=%d" % (tag, rc)); L.append(o); L.append("")
wr(os.path.join(D, "PROBE_matrix.txt"), "\n".join(L))
for blk in "\n".join(L).split("== ")[1:]:
    lines = blk.splitlines()
    print("%-34s %s" % (lines[0], [x for x in lines if x.startswith(("PASS=", "ALL_CHECKS_PASS="))]))

json.dump({"pre": pre, "post": post, "rollback_exit": p.returncode,
           "post_eq_baseline": post == sha(BL),
           "patch_apply_ok": ok and sha(REC) == sha(MD),
           "modified_sha": sha(MD), "modified_bytes": len(cur_b)},
          io.open(os.path.join(D, "_final_gates.json"), "w", encoding="utf-8", newline="\n"),
          ensure_ascii=False, indent=2)
print("DONE")
