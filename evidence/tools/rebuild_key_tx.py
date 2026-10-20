# -*- coding: utf-8 -*-
"""key_api_tx 事务最终重建：MODIFIED_FILE / baseline / DIFF / 三态实测矩阵。"""
import io, os, re, json, shutil, hashlib, difflib, subprocess, time, socket, threading
import http.server, urllib.request

TX = r"C:\Users\Administrator\Desktop\nai\work\key_api_tx"
REPO = r"C:\Users\Administrator\Desktop\nai\xianyu-nai"
DATA = r"C:\Users\Administrator\Desktop\nai\run"
KEY = "nai3-REDACTED-REPLACE-WITH-YOUR-OWN-KEY"
PROXY = "https://nai.xianyuw.cn"
PROMPT = "1girl, solo, upper body, white background"
FILES = ["main.go", "nai/client.go", "nai/ui.go", "nai/client_test.go",
         "nai/pipeline.go", "nai/sd_cmd.go"]


def sha(p):
    return hashlib.sha256(io.open(p, "rb").read()).hexdigest()


def rd(p):
    return io.open(p, encoding="utf-8", newline="").read()


def wr(p, s):
    os.makedirs(os.path.dirname(p), exist_ok=True)
    io.open(p, "w", encoding="utf-8", newline="").write(s)


def sub1(s, old, new, tag):
    n = s.count(old)
    assert n == 1, "REV %s match=%d" % (tag, n)
    return s.replace(old, new)


BASE = os.path.join(TX, "baseline")
MOD = os.path.join(TX, "MODIFIED_FILE")

# ---------- 1. MODIFIED_FILE := 当前仓库 ----------
for f in FILES:
    wr(os.path.join(MOD, f), rd(os.path.join(REPO, f)))

# ---------- 2. baseline := 从 MODIFIED 反向还原 ----------
cur = {f: rd(os.path.join(MOD, f)) for f in FILES}
base = dict(cur)

base["nai/client.go"] = sub1(cur["nai/client.go"],
    '\t\tMode:      "\u4e2d\u8f6c",              // 0x100A4051 \u8bfb [nai3\u8bbe\u7f6e] \u6a21\u5f0f\uff0c\u9ed8\u8ba4\u4e32 0x102B5C04 = "\u4e2d\u8f6c"',
    '\t\tMode:      "",', "mode")
base["nai/client.go"] = sub1(base["nai/client.go"],
    '\t\tCfg:       0.28,              // nai3\u53c2\u6570/cfg \u9ed8\u8ba4 "0.28"\uff080x100A4282 \u2192 0x1039AC60\uff09',
    '\t\tCfg:       5,                 // naiCfg \u9ed8\u8ba4 5', "cfg")
base["nai/client.go"] = sub1(base["nai/client.go"],
    '\t\tStrength:  0.52, // nai3\u53c2\u6570/strength \u9ed8\u8ba4 "0.52"\uff080x100A433D \u2192 0x1039AB54\uff09',
    '\t\tStrength:  0.28, // naiStrength \u9ed8\u8ba4\u6bb5', "strength")

base["nai/ui.go"] = sub1(cur["nai/ui.go"],
    '\t\t\t\tnum("cfg", "CFG", "0.28"),\n\t\t\t\tnum("strength", "\u91cd\u7ed8\u5f3a\u5ea6", "0.52"),',
    '\t\t\t\tnum("cfg", "CFG", "5"),\n\t\t\t\tnum("strength", "\u91cd\u7ed8\u5f3a\u5ea6", "0.28"),', "ui-num")
base["nai/ui.go"] = sub1(base["nai/ui.go"],
    '\t\t\t\tsel("\u6a21\u5f0f", "\u6a21\u5f0f\uff08\u5b98\u65b9 / \u53cd\u4ee3\uff09", "\u4e2d\u8f6c", "", "\u4e2d\u8f6c", "\u5b98\u65b9", "\u53cd\u4ee3"),',
    '\t\t\t\tsel("\u6a21\u5f0f", "\u6a21\u5f0f\uff08\u5b98\u65b9 / \u53cd\u4ee3\uff09", "", "", "\u5b98\u65b9", "\u53cd\u4ee3"),', "ui-sel")

base["nai/client_test.go"] = sub1(cur["nai/client_test.go"],
    '"promptGuidanceRescale":0.28,"noise_schedule"',
    '"promptGuidanceRescale":5,"noise_schedule"', "test")

base["main.go"] = sub1(cur["main.go"],
    'flag.Float64("strength", 0.52, "img2img \u91cd\u7ed8\u5e45\u5ea6")',
    'flag.Float64("strength", 0.28, "img2img \u91cd\u7ed8\u5e45\u5ea6")', "main-strength")

blk = ('\t// 8.1 \u53cd\u4ee3\u53ea\u7ed9 image_url\u3001\u4e0d\u7ed9 base64 \u65f6\uff08\u5982 nai.xianyuww.cn / nai.xianyuw.cn\uff09\uff0c\n'
       '\t// \u9700\u8981\u628a\u76f4\u94fe\u4e0b\u8f7d\u56de\u6765\uff0c\u5426\u5219\u4e0b\u6e38\u6309"\u6ca1\u51fa\u56fe"\u5904\u7406\u3002\n'
       '\tif len(out.Images) == 0 && strings.TrimSpace(out.ImageURL) != "" {\n'
       '\t\tif b, err := c2.FetchImageBytes(ctx, out.ImageURL); err == nil && len(b) > 0 {\n'
       '\t\t\tout.Images = append(out.Images, b)\n'
       '\t\t}\n'
       '\t}\n\n')
base["nai/pipeline.go"] = sub1(cur["nai/pipeline.go"], blk, "", "pipeline")

s = cur["nai/sd_cmd.go"]
new_fb = ('// FetchImageBase64 \u4e0b\u8f7d\u4e00\u5f20\u56fe\u5e76\u8f6c\u6210 base64\uff0c\u4f9b img2img \u7684 init_images[0]\u3002\n'
          'func (c *Client) FetchImageBase64(ctx context.Context, u string) (string, error) {\n'
          '\tb, err := c.FetchImageBytes(ctx, u)\n'
          '\tif err != nil {\n\t\treturn "", err\n\t}\n'
          '\treturn base64.StdEncoding.EncodeToString(b), nil\n}\n')
old_fb = ('// FetchImageBase64 \u4e0b\u8f7d\u4e00\u5f20\u56fe\u5e76\u8f6c\u6210 base64\uff0c\u4f9b img2img \u7684 init_images[0]\u3002\n'
          'func (c *Client) FetchImageBase64(ctx context.Context, u string) (string, error) {\n'
          '\tu = strings.TrimSpace(u)\n'
          '\tif u == "" {\n\t\treturn "", fmt.Errorf("\u56fe\u7247\u5730\u5740\u4e3a\u7a7a")\n\t}\n'
          '\tif strings.HasPrefix(u, "base64://") {\n\t\treturn strings.TrimPrefix(u, "base64://"), nil\n\t}\n'
          '\treq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)\n'
          '\tif err != nil {\n\t\treturn "", err\n\t}\n'
          '\tresp, err := c.HTTP.Do(req)\n'
          '\tif err != nil {\n\t\treturn "", fmt.Errorf("%s", MsgSDLinkFailed)\n\t}\n'
          '\tdefer resp.Body.Close()\n'
          '\tb, err := io.ReadAll(resp.Body)\n'
          '\tif err != nil {\n\t\treturn "", fmt.Errorf("%s", MsgSDLinkFailed)\n\t}\n'
          '\treturn base64.StdEncoding.EncodeToString(b), nil\n}\n')
s = sub1(s, new_fb, old_fb, "sd-fetch")
s = sub1(s, 'import (\n\t"context"\n\t"encoding/base64"\n\t"encoding/json"\n\t"os"\n',
         'import (\n\t"context"\n\t"encoding/base64"\n\t"encoding/json"\n\t"fmt"\n\t"io"\n\t"net/http"\n\t"os"\n', "sd-imp")
base["nai/sd_cmd.go"] = s

for f in FILES:
    wr(os.path.join(BASE, f), base[f])

# ---------- 3. DIFF_FILE.patch ----------
chunks = []
for f in FILES:
    d = list(difflib.unified_diff(base[f].splitlines(True), cur[f].splitlines(True),
                                  fromfile="a/" + f, tofile="b/" + f, n=3))
    chunks.append("".join(d))
full = "".join(chunks)
wr(os.path.join(TX, "DIFF_FILE.patch"), full)
print("DIFF bytes=%d files=%d hunks=%d plus=%d minus=%d" % (
    len(full.encode("utf-8")), len(FILES), full.count("\n@@ ") + (1 if full.startswith("@@ ") else 0),
    sum(1 for l in full.splitlines() if l.startswith("+") and not l.startswith("+++")),
    sum(1 for l in full.splitlines() if l.startswith("-") and not l.startswith("---"))))

# ---------- 4. patch 反向重建验证 ----------
raw = rd(os.path.join(TX, "DIFF_FILE.patch")).splitlines(True)
i = 0; pfiles = []; curf = None
while i < len(raw):
    m = re.match(r"^--- a/(.+)$", raw[i].rstrip("\n"))
    if m:
        curf = {"path": m.group(1), "hunks": []}; pfiles.append(curf); i += 1; continue
    if raw[i].startswith("+++ b/"): i += 1; continue
    m = re.match(r"^@@ -(\d+)(?:,(\d+))? \+\d+(?:,\d+)? @@", raw[i])
    if m and curf is not None:
        curf["hunks"].append({"old": int(m.group(1)), "lines": []}); i += 1
        while i < len(raw) and not raw[i].startswith(("@@", "--- a/")):
            if raw[i][:1] in (" ", "+", "-"): curf["hunks"][-1]["lines"].append(raw[i])
            i += 1
        continue
    i += 1

PV = os.path.join(TX, "patch_verify")
if os.path.isdir(PV): shutil.rmtree(PV)
shutil.copytree(BASE, PV)
patch_ok = True
for f in pfiles:
    src = rd(os.path.join(BASE, f["path"])).splitlines(True)
    out = []; pos = 0
    for h in f["hunks"]:
        st = h["old"] - 1
        out.extend(src[pos:st]); pos = st
        for ln in h["lines"]:
            tag, txt = ln[:1], ln[1:]
            if tag == " ":
                assert src[pos] == txt, "CTX %s@%d" % (f["path"], pos); out.append(txt); pos += 1
            elif tag == "-":
                assert src[pos] == txt, "DEL %s@%d" % (f["path"], pos); pos += 1
            else:
                out.append(txt)
    out.extend(src[pos:])
    wr(os.path.join(PV, f["path"]), "".join(out))
    patch_ok = patch_ok and sha(os.path.join(PV, f["path"])) == sha(os.path.join(MOD, f["path"]))
print("PATCH_APPLY_OK=%s files=%d hunks=%d" % (patch_ok,
      len(pfiles), sum(len(f["hunks"]) for f in pfiles)))

# ---------- 5. 建三棵树 ----------
def make_tree(name, srcdir):
    d = os.path.join(TX, name)
    if os.path.isdir(d): shutil.rmtree(d)
    shutil.copytree(REPO, d, ignore=shutil.ignore_patterns("*.exe", "*.test"))
    for f in FILES:
        shutil.copy2(os.path.join(srcdir, f), os.path.join(d, f))
    return d

trees = {}
trees["BASELINE"] = (make_tree("build_baseline", BASE), "build_baseline")
trees["MODIFIED"] = (make_tree("build_modified", MOD), "build_modified")
RB = os.path.join(TX, "rollback_tree")
if os.path.isdir(RB): shutil.rmtree(RB)
shutil.copytree(REPO, RB, ignore=shutil.ignore_patterns("*.exe", "*.test"))
for f in FILES:
    shutil.copy2(os.path.join(MOD, f), os.path.join(RB, f))

# ---------- 6. ROLLBACK ----------
before = {f: sha(os.path.join(RB, f)) for f in FILES}
p = subprocess.run(["powershell", "-ExecutionPolicy", "Bypass", "-File",
                    os.path.join(TX, "ROLLBACK.ps1"), "-Target", RB], capture_output=True)
rb_out = p.stdout.decode("utf-8", "replace")
after = {f: sha(os.path.join(RB, f)) for f in FILES}
basesha = {f: sha(os.path.join(BASE, f)) for f in FILES}
rb_ok = all(after[f] == basesha[f] for f in FILES)
json.dump({"before": before, "after": after, "baseline": basesha,
           "exit": p.returncode, "all_equal": rb_ok, "stdout": rb_out},
          io.open(os.path.join(TX, "rollback_hashes.json"), "w", encoding="utf-8", newline="\n"),
          ensure_ascii=False, indent=2)
trees["ROLLBACK"] = (RB, "rollback_tree")
print("ROLLBACK exit=%d all_equal=%s" % (p.returncode, rb_ok))

# ---------- 7. 构建三个 exe ----------
build = {}
for tag, (d, _) in trees.items():
    r = subprocess.run(["go", "build", "-p", "1", "-o", "xianyu-nai.exe", "."], cwd=d, capture_output=True)
    build[tag] = {"exit": r.returncode, "err": r.stderr.decode("utf-8", "replace")[:300],
                  "bytes": os.path.getsize(os.path.join(d, "xianyu-nai.exe")) if r.returncode == 0 else 0}
    print("%-9s BUILD exit=%d bytes=%d" % (tag, build[tag]["exit"], build[tag]["bytes"]))

# ---------- 8. 三态：dump / live / UI schema ----------
def dec(b):
    for enc in ("utf-8", "gbk"):
        try: return b.decode(enc).strip()
        except Exception: pass
    return b.decode("latin-1").strip()

def freeport():
    s = socket.socket(); s.bind(("127.0.0.1", 0)); q = s.getsockname()[1]; s.close(); return q

def probe(tag):
    d, _ = trees[tag]
    exe = os.path.join(d, "xianyu-nai.exe")
    row = {"tag": tag}
    q = subprocess.run([exe, "-prompt", PROMPT, "-key", KEY, "-proxy", PROXY,
                        "-steps", "28", "-width", "832", "-height", "1216", "-dump"],
                       capture_output=True, timeout=120)
    body = dec(q.stdout)
    row["dump_exit"] = q.returncode
    try: row["pgr"] = json.loads(body).get("promptGuidanceRescale")
    except Exception: row["pgr"] = None
    wr(os.path.join(TX, "dump_body_%s.json" % tag.lower()), body + "\n")
    o = subprocess.run([exe, "-prompt", PROMPT, "-official", "-official-key", "pst-dummy",
                        "-official-url", "http://127.0.0.1:1", "-model", "nai-diffusion-4-full",
                        "-image", r"C:\Windows\Web\Wallpaper\Windows\img0.jpg", "-dump"],
                       capture_output=True, timeout=120)
    try: row["strength"] = json.loads(dec(o.stdout)).get("parameters", {}).get("strength")
    except Exception: row["strength"] = None
    t0 = time.time()
    l = subprocess.run([exe, "-prompt", PROMPT, "-key", KEY, "-proxy", PROXY,
                        "-steps", "28", "-width", "832", "-height", "1216"],
                       capture_output=True, timeout=400)
    row.update({"live_exit": l.returncode, "live_sec": round(time.time() - t0, 1),
                "live_out": dec(l.stdout), "live_err": dec(l.stderr)})
    port = freeport()
    nf = io.open(os.path.join(TX, "ui_%s.log" % tag.lower()), "wb")
    pr = subprocess.Popen([exe, "-ui", "-ui-addr", "127.0.0.1:%d" % port, "-data", DATA],
                          stdout=nf, stderr=nf)
    time.sleep(2.5)
    try:
        base_u = "http://127.0.0.1:%d" % port
        sch = json.loads(urllib.request.urlopen(base_u + "/api/schema", timeout=20).read())
        cfg = json.loads(urllib.request.urlopen(base_u + "/api/config", timeout=20).read())
        row["http"] = [(ep, urllib.request.urlopen(base_u + ep, timeout=20).status)
                       for ep in ("/", "/api/schema", "/api/config", "/api/status", "/api/presets")]
        for sec in sch:
            if sec.get("name") == "nai3设置":
                for f in sec.get("fields", []):
                    if f.get("key") == "模式":
                        row["mode_def"] = f.get("def"); row["mode_opts"] = f.get("opts")
            if sec.get("name") == "nai3参数":
                for f in sec.get("fields", []):
                    if f.get("key") in ("cfg", "strength"):
                        row["schema_" + f["key"]] = f.get("def")
        ini = cfg.get("nai3参数") or {}
        row["ini_cfg"] = ini.get("cfg"); row["ini_strength"] = ini.get("strength")
        row["ini_mode"] = (cfg.get("nai3设置") or {}).get("模式")
    finally:
        pr.terminate()
        try: pr.wait(5)
        except Exception: pr.kill()
        nf.close()
    return row

rows = [probe(t) for t in ("BASELINE", "MODIFIED", "ROLLBACK")]
json.dump(rows, io.open(os.path.join(TX, "ui_schema_matrix.json"), "w", encoding="utf-8", newline="\n"),
          ensure_ascii=False, indent=2)
for r in rows:
    print("%-9s pgr=%-6s strength=%-5s live_exit=%s %ss mode_def=%r opts=%r" % (
        r["tag"], r["pgr"], r["strength"], r["live_exit"], r["live_sec"], r["mode_def"], r["mode_opts"]))

json.dump({"diff_ok": patch_ok, "rollback": {"exit": p.returncode, "all_equal": rb_ok},
           "build": build}, io.open(os.path.join(TX, "_final_gates.json"), "w", encoding="utf-8", newline="\n"),
          ensure_ascii=False, indent=2)
print("DONE")
