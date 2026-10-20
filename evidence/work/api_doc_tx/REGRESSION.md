# REGRESSION — xianyu-nai 全量回归 + 四角色复验

- 日期：2026-10-19
- 目标工程：`C:\Users\Administrator\Desktop\nai\xianyu-nai`（Go 模块 `xianyu-nai`，产物 `xianyu-nai.exe`）
- 数据目录：`C:\Users\Administrator\Desktop\nai\run`
- 环境：Windows（PowerShell 5.1）、go1.22.5 windows/amd64、无 git、无 sh、无 C 编译器、无 WSL
- 本次回归的原始日志目录（全部 UTF-8）：
  `C:\Users\Administrator\Desktop\nai\work\api_doc_tx\_reg\`
- 业务代码改动：**无**（未发现需要修的编译/测试笔误；下述 FINDINGS 均只记录、未修改）

## 结论汇总

| # | 检查项 | 结果 |
|---|---|---|
| 1a | `go version` | PASS（go1.22.5 windows/amd64） |
| 1b | `go build -p 1 -o xianyu-nai.exe .` | PASS（exit 0，无输出，产物 9,990,656 字节） |
| 1c | `go test ./nai/ -v` | PASS（120 个用例 + 4 个子测试，PASS 124 / FAIL 0 / SKIP 0） |
| 1d | `go vet ./...` | PASS（exit 0，无输出） |
| 1e | `gofmt -l .` | PASS（exit 0，无输出 = 无未格式化文件） |
| 2 | ini 节/键 与 `UISchema()` 对应关系 | PASS（ini 中无 schema 之外的节/键；schema 中 117 个键在 ini 中缺省，由默认值补齐，符合设计） |
| 2b | ini 值语义（select 取值域） | **FINDING F1**（`模式=中转` 不在 schema opts 内，见下） |
| 3 | 配置面板冒烟（4 个 API） | PASS（全部 200；cfg=0.28、strength=0.52、key 非空、nai3api 正确） |
| 4 | 两套四角色产物复验（路径/sha256/大小） | PASS |
| 4b | `key_api_tx/MODIFIED_FILE` 5 文件 == 仓库当前 5 文件 | PASS（逐文件 sha256 相同） |
| 5 | `ROLLBACK.ps1` 幂等（key_api_tx，连跑两次） | PASS（两次均 exit 0；两次后 5 文件 == baseline） |
| 5b | `ROLLBACK.ps1` 幂等（api_doc_tx，连跑两次） | PASS（第二次输出 `ROLLBACK_OK (already absent)`） |
| 6 | 自写 unified diff 应用器重建 MODIFIED_FILE | PASS（key: 5 文件/7 hunk；doc: 1 文件/1 hunk；`PATCH_APPLY_OK=True`） |
| 补 | 重建树可编译/可测/可 vet/可 gofmt | PASS（build/vet/test/gofmt 全 0） |
| 补 | 三态（BASELINE/MODIFIED/ROLLBACK）`promptGuidanceRescale` 复现 | PASS（5 / 0.28 / 5） |
| — | 发现的未修问题 | F1（select 取值）、F2（CLI `-strength` 默认值仍是 0.28） |

---

## 1. 工具链与构建/测试

所有命令均在 `C:\Users\Administrator\Desktop\nai\xianyu-nai` 下执行，stdout/stderr 重定向到 UTF-8 文件后读取。

### 1.1 go version

```
PS> go version
```
```
go version go1.22.5 windows/amd64
```
**EXIT 0**

### 1.2 go build

```
PS> go build -p 1 -o xianyu-nai.exe .
```
stdout: `<empty>`
stderr: `<empty>`
**EXIT 0**

产物：
```
C:\Users\Administrator\Desktop\nai\xianyu-nai\xianyu-nai.exe
size    = 9990656
mtime   = 2026-10-19 22:55:52
sha256  = e7ec4d98fe863aad52a06c7d95824eae2075eabf78cff591a8d77a9db83349e9
```

### 1.3 go test ./nai/ -v

```
PS> go test ./nai/ -v
```
末尾摘要（字面）：
```
--- PASS: TestWSPing (0.00s)
=== RUN   TestWSAcceptKey
--- PASS: TestWSAcceptKey (0.00s)
PASS
ok  	xianyu-nai/nai	2.009s
```
计数（对 `_reg\test.out` 统计）：
```
RUN 行总数        = 124   （120 个顶层测试函数 + 4 个子测试）
--- PASS 行总数   = 124
--- FAIL 行总数   = 0
--- SKIP 行总数   = 0
ok  	xianyu-nai/nai	2.009s
```
4 个子测试（均 PASS）：
```
TestUploadImageEndpointResolution/资源地址兜底
TestUploadImageEndpointResolution/配置项直用
TestUploadImageEndpointResolution/INI_默认_0_视为未配置
TestUploadImageEndpointResolution/都没配
```
**EXIT 0**

补充：`go test ./...` 也跑了一遍
```
?   	xianyu-nai	[no test files]
ok  	xianyu-nai/nai	(cached)
```
**EXIT 0**

> 注：PowerShell 默认代码页下 `-v` 输出的中文子测试名在控制台会乱码，日志文件本身是 UTF-8。
> 复核时用 `chcp 65001` 重跑一次，结果一致（`PASS / ok xianyu-nai/nai (cached)`，`--- FAIL` 0 行）。

### 1.4 go vet

```
PS> go vet ./...
```
stdout: `<empty>`
stderr: `<empty>`
**EXIT 0**

### 1.5 gofmt

```
PS> gofmt -l .
```
stdout: `<empty>`（没有任何文件被列出 = 全部已格式化）
stderr: `<empty>`
**EXIT 0**

---

## 2. `run\咸鱼ai画图.ini` 与 `UISchema()` 的键对应

被检 ini：`C:\Users\Administrator\Desktop\nai\run\咸鱼ai画图.ini`
- 600 字节，UTF-8 **无 BOM**，LF 换行
- sha256 `dbfda0cc2fa25f363063c21d6405f8bcf07502631655ff2551dac4d7ba20382c`
- 4 个节 / 18 个键

实测内容（字面）：
```ini
[nai3设置]
key=nai3-REDACTED-REPLACE-WITH-YOUR-OWN-KEY
模式=中转

[nai3参数]
全局默认模型=nai-diffusion-3
scale=5
steps=28
cfg=0.28
strength=0.52
默认种子=-1
宽度=832
高度=1216
采样=k_euler
噪声=native
sm=0
sm_dyn=0
负面词条=lowres, {bad}, error, fewer, extra, missing, worst quality, jpeg artifacts, bad quality, watermark, unfinished, displeasing, chromatic aberration, signature, extra digits, artistic error, username, scan, [abstract]

[nai3指令]
画图=nai画图

[重制版]
nai3api=https://nai.xianyuw.cn
naiofficialapi=https://image.novelai.net
```

schema（实跑 `/api/schema` 取得）：18 个节 / 135 个键。

### 2.1 ini 有、schema 没有的节 / 键

```
sections_in_ini_not_in_schema = []
keys_in_ini_not_in_schema     = (none)
```
**PASS**：ini 里没有任何 schema 之外的节或键。

### 2.2 schema 有、ini 里缺的节 / 键

缺 14 个整节（该 ini 只配置了 NAI 部分）：
```
['基本', '类型', '指令', '点数限制', '点数消耗', '图像审核', '黑名单功能',
 'sd设置', 'sd参数', 'sd指令', 'comfy设置', 'comfy参数', 'comfy指令', '浏览器UI']
```
缺 117 个键（总数），按节：
```
[基本]        9 : 主人, 机器人QQ, 发送模式, 是否引用, 官机互动, 上传图像, AI翻译令牌, 屏蔽词条, psw
[类型]        3 : 翻译接口, 今日少女接口, sd接口
[指令]       13 : 菜单, 开启群, 关闭群, 图片鉴赏, 清晰术, 去背景, ai鉴别, 图片混淆, 图片解混淆,
                 添加屏蔽, 添加黑名单, 取消黑名单, 查询点数
[点数限制]    5 : 开关, 基础点数, 点数处理类型, 点数不足提示, 充值点数指令
[点数消耗]    3 : sd画图, nai画图, comfy画图
[图像审核]    4 : 平台, 开关, 阈值, 处理
[黑名单功能]  6 : sd画图, nai画图, 清晰术, 去背景, 上传标签, 上传预设
[nai3设置]    1 : 官方key
[nai3参数]    2 : 完成后冷却, 完成冷却模式
[nai3指令]   17 : 修改种子, 修改负面, 查看负面, 情绪修改, 转线稿, 转草稿, 图像清理, 着色,
                 图像解析, 查看预设, 查看模型, 查询额度, 上传标签, 上传预设, 查看标签,
                 删除标签, 删除预设
[sd设置]      3 / [sd参数] 13 / [sd指令] 10 / [comfy设置] 2 / [comfy参数] 11 / [comfy指令] 5
[浏览器UI]    1 : 版本号
[重制版]      9 : nai3_plugin-api, 资源地址, upload-image, onebot_api, onebot_token,
                 onebot_listen, onebot_path, onebot_ws, onebot_ws_reverse
total_missing_keys = 117
```
说明：`ReadConfig()` 对缺失键用 schema 的 `def` 补齐（`ui.go:446`），`WriteConfig()` 只在有值时落盘（`ui.go:477`），所以“缺键”不是错误，是 ini 精简存储的正常形态。回归中已实测 `/api/config` 返回 18 节 135 键（全部带值）。

### 2.3 值的类型/取值域校验（仅对 ini 里出现的键）

```
bad_values = 1
BAD [nai3设置] 模式 type=select value='中转' -> 不是 schema 的 opts ['', '官方', '反代'] 之一 (def='')
```
其余 17 个值全部合法：`cfg='0.28'`、`strength='0.52'`、`scale='5'`、`steps='28'`、`sm/sm_dyn='0'` 等。

### 2.4 def vs ini（仅 ini 中存在的键）

```
DIFF [nai3设置] 模式         ini='中转'                     def=''
DIFF [nai3设置] key          ini='nai3-uwYZ...CssN'(53)     def=''
OK   [nai3参数] scale        ini='5'                        def='5'
OK   [nai3参数] steps        ini='28'                       def='28'
OK   [nai3参数] cfg          ini='0.28'                     def='0.28'     ← 关键项 PASS
OK   [nai3参数] strength     ini='0.52'                     def='0.52'     ← 关键项 PASS
OK   [nai3参数] 默认种子      ini='-1'                       def='-1'
DIFF [nai3参数] 宽度          ini='832'                      def='1024'     （用户调过，非缺陷）
DIFF [nai3参数] 高度          ini='1216'                     def='1024'     （用户调过，非缺陷）
OK   [nai3参数] 采样/噪声/sm/sm_dyn/全局默认模型/负面词条     与 def 一致
DIFF [nai3指令] 画图          ini='nai画图'                   def='画图'      （用户改过指令词，非缺陷）
DIFF [重制版] nai3api        ini='https://nai.xianyuw.cn'   def=''         （用户填写，正常）
OK   [重制版] naiofficialapi ini='https://image.novelai.net' def='https://image.novelai.net'
```

---

## 3. 配置面板冒烟（真跑进程）

启动命令（随机端口 55365，`Start-Process` + stdout/stderr 重定向到文件，结束后 taskkill 清场）：

```
xianyu-nai.exe -ui -ui-addr 127.0.0.1:55365 -data C:\Users\Administrator\Desktop\nai\run
```
进程启动日志（字面）：
```
面板已启动：http://127.0.0.1:55365/
数据目录：C:\Users\Administrator\Desktop\nai\run
按 Ctrl+C 退出
```
`/api/status` 探测成功（ready=true），随后 4 个 GET：

| 端点 | HTTP | 响应字节 | 关键字段结论 |
|---|---|---|---|
| `/api/schema` | **200** | 9742 | 18 节 / 135 键；`nai3参数/cfg` def=`"0.28"`（type=number）、`nai3参数/strength` def=`"0.52"`（type=number）；另有 `comfy参数/cfg` def=`"7"` |
| `/api/config` | **200** | 2107 | 18 节 / 135 键；`nai3参数/cfg="0.28"`、`nai3参数/strength="0.52"`、`nai3设置/key` 长度 53（前缀 `nai3-u`，非空）、`nai3设置/模式="中转"`、`重制版/nai3api="https://nai.xianyuw.cn"`、`重制版/naiofficialapi="https://image.novelai.net"` |
| `/api/status` | **200** | 200 | `enabled=true`、`inited=true`、`nai_mode="中转"`、`ini_path="...\run\咸鱼ai画图.ini"`、`data_dir="...\run"`、`sd_url=""`、`comfy_url=""` |
| `/api/presets` | **200** | 95 | `nai_presets=null`、`nai_tags=null`、`sd_presets=[]`、`comfy_presets=[]`、`comfy_workflows=null`（该目录未放预设文件，属正常） |

关键断言：
```
SCHEMA nai3参数/cfg       def = "0.28"   PASS
SCHEMA nai3参数/strength  def = "0.52"   PASS
CONFIG nai3参数/cfg       = "0.28"       PASS
CONFIG nai3参数/strength  = "0.52"       PASS
CONFIG nai3设置/key       长度 53，非空  PASS
CONFIG 重制版/nai3api     = "https://nai.xianyuw.cn"      PASS
CONFIG 重制版/naiofficialapi = "https://image.novelai.net" PASS
```
进程收尾：
```
taskkill /F /PID <pid> /T  -> 成功终止
Get-Process xianyu-nai -> 0
```
原始 JSON 存档：`_reg\ui_api_schema.json`、`_reg\ui_api_config.json`、`_reg\ui_api_status.json`、`_reg\ui_api_presets.json`、`_reg\ui_smoke_raw.json`、`_reg\ui_smoke_summary.json`。

---

## 4. 四角色产物复验（逐一 reopen）

### 4.1 集合 A：`work\key_api_tx`

**角色1 MODIFIED_FILE（目录，5 文件）**
```
C:\Users\Administrator\Desktop\nai\work\key_api_tx\MODIFIED_FILE\nai\client.go
C:\Users\Administrator\Desktop\nai\work\key_api_tx\MODIFIED_FILE\nai\ui.go
C:\Users\Administrator\Desktop\nai\work\key_api_tx\MODIFIED_FILE\nai\client_test.go
C:\Users\Administrator\Desktop\nai\work\key_api_tx\MODIFIED_FILE\nai\pipeline.go
C:\Users\Administrator\Desktop\nai\work\key_api_tx\MODIFIED_FILE\nai\sd_cmd.go
```
| 文件 | 字节 | sha256 |
|---|---:|---|
| nai/client.go | 16981 | `d6ca37e1bdd8749b8b7c9b7fdb7d22054469668fa3cba16007eee180d1b5a104` |
| nai/client_test.go | 3362 | `a3594eaf0ca5ca2cf4857cb3e81215cee5e58361da1bf976a14811ed0e42bcbb` |
| nai/pipeline.go | 9972 | `4cc2f198cc77544a0c5b62d26a373cb5963556a971c8a37f7d5ffcd4b3eb92fd` |
| nai/sd_cmd.go | 13316 | `ce04b336fa8b20177ca08b340bd0d31ff1a087fd91963b25e9ec6f9158374be4` |
| nai/ui.go | 31802 | `5d402c30963fd870ff82eb9c7164e3d0e34a85165b182a0fdce43b1b3754e3be` |

目录聚合：files=5, bytes=75433, aggregate_sha256=`d5b32b59d077eaba4a2e0ebda5091409c4022b85e4f358059cb73fb5345605b2`

**角色1 baseline（目录，5 文件）**
| 文件 | 字节 | sha256 |
|---|---:|---|
| nai/client.go | 16894 | `d00d96703bbdb5f4faa0934993c7bf92c84d3ff83fd2e02c74115a9a61e1e5e0` |
| nai/client_test.go | 3359 | `2407833c001d2bab97d11ff4f1ff5bf5c6425cc1a2b75057bba5f0f41dc232e0` |
| nai/pipeline.go | 9612 | `36c45137bc6a293c76709ba16b2d260dc3fa4deb6e75b69f8b3f3b09eeac8583` |
| nai/sd_cmd.go | 13775 | `516662c55037061c2127a2737a9e360d43d5636e627dcece5ba6b660970aba1c` |
| nai/ui.go | 31799 | `872976bf9f453e2fa08a3935e56d6c4114b75f75754bce0a670bff7e2d623221` |

目录聚合：files=5, bytes=75439, aggregate_sha256=`35e04646f5488566f3aba882a1eb5664a83a99df43933763066eb372838c6367`

**角色2 DIFF_FILE**
```
C:\Users\Administrator\Desktop\nai\work\key_api_tx\DIFF_FILE.patch   3495 字节
sha256 0d2c7449545611d522d6af7e0c77620592f6288d2f30c0a26b4814e12d15a84f
5 文件 / 7 hunk（client.go 2、ui.go 1、client_test.go 1、pipeline.go 1、sd_cmd.go 2）
```

**角色3 VERIFICATION.txt**
```
C:\Users\Administrator\Desktop\nai\work\key_api_tx\VERIFICATION.txt  6665 字节
sha256 417ebf59b79760542f1f15bade9da33204ba953c59a5da15157d7e0c7428274c
```

**角色4 ROLLBACK**
```
C:\Users\Administrator\Desktop\nai\work\key_api_tx\ROLLBACK.sh    983 字节  sha256 8428c98ee2a54e980bd673a00d8e9d1f19a018b17179fed193549f7a7a583c2f
C:\Users\Administrator\Desktop\nai\work\key_api_tx\ROLLBACK.ps1   942 字节  sha256 89b6e8ab413883501cb65a19ca8ef6378fc7fbff81adecab1e2816a2435eafbf
```

### 4.2 集合 B：`work\api_doc_tx`

| 角色 | 绝对路径 | 字节 | sha256 |
|---|---|---:|---|
| 1 MODIFIED_FILE | `C:\Users\Administrator\Desktop\nai\work\api_doc_tx\MODIFIED_FILE_API-doc.md` | 20814 | `83dfb94ad5f146b2999430b2ea367bd8b8e1db92f7c739fa6f2079bd6bfd06b7` |
| 2 DIFF_FILE | `C:\Users\Administrator\Desktop\nai\work\api_doc_tx\API-doc.diff` | 21358 | `6906f2bb4767e7c9e4a667bb5688d448873ede38180df0566a35765e5f2d2e2b` |
| 3 VERIFICATION | `C:\Users\Administrator\Desktop\nai\work\api_doc_tx\VERIFICATION.txt` | 4653 | `acfdc64bfddf57d43208556b182e2e77bbfea60b116fe5d129ccd8f5084335b2` |
| 4 ROLLBACK.sh | `C:\Users\Administrator\Desktop\nai\work\api_doc_tx\ROLLBACK.sh` | 479 | `e694c10b7d6aae228f9940fb111b3126a94f0bed886ddb2e6f7055ca7b872b20` |
| 4 ROLLBACK.ps1 | `C:\Users\Administrator\Desktop\nai\work\api_doc_tx\ROLLBACK.ps1` | 466 | `d56887d168fd1c60c2d20b898de6a4a85528e304bfaf445b0b7dafecf9c66f7d` |

配套目标树（同一目录下）：
```
target_baseline\  files=0  aggregate_sha256=e3b0c442...b855 （空，DOC_ABSENT）
target_modified\  1 文件 20814 字节 = 83dfb94a... （= MODIFIED_FILE，一致）
target_rollback\  files=0  aggregate_sha256=e3b0c442...b855 （空，已回滚）
```
文档探针（`probe.py`）复跑，字面：
```
CMD python work/api_doc_tx/probe.py ...\target_baseline -> EXIT 0  RESULT=DOC_ABSENT
CMD python work/api_doc_tx/probe.py ...\target_modified -> EXIT 0  RESULT=DOC_PRESENT
     BYTES=20814  SHA256=83dfb94a...6bfd06b7
     CHECKS={"has_endpoint":true,"has_official":true,"has_bearer":true,"has_key_note":true,
             "has_threshold":true,"has_zip":true,"has_subscription":true,"has_plugin_api":true}
     ALL_CHECKS_PASS=True
CMD python work/api_doc_tx/probe.py ...\target_rollback -> EXIT 0  RESULT=DOC_ABSENT
CMD python work/api_doc_tx/probe.py ...\xianyu-nai       -> EXIT 0  RESULT=DOC_PRESENT
     SHA256=83dfb94a...  ALL_CHECKS_PASS=True
```

### 4.3 MODIFIED_FILE 与仓库当前文件的逐一比对

```
file                   repo_sha256(16)   modified(16)      baseline(16)      repo==mod  repo==base
nai/client.go          d6ca37e1bdd8749b  d6ca37e1bdd8749b  d00d96703bbdb5f4  True       False
nai/ui.go              5d402c30963fd870  5d402c30963fd870  872976bf9f453e2f  True       False
nai/client_test.go     a3594eaf0ca5ca2c  a3594eaf0ca5ca2c  2407833c001d2bab  True       False
nai/pipeline.go        4cc2f198cc77544a  4cc2f198cc77544a  36c45137bc6a293c  True       False
nai/sd_cmd.go          ce04b336fa8b2017  ce04b336fa8b2017  516662c55037061c  True       False
ALL_REPO_EQ_MODIFIED = True
```
文档同样：仓库 `API文档-NAI画图接口.md`（20814 字节）sha256 = `83dfb94a...` = `MODIFIED_FILE_API-doc.md`，`repo_doc_eq_modified=True`。

结论：仓库当前处于 **MODIFIED 状态**（两笔事务都已落盘到仓库），且与角色1产物逐字节一致。

---

## 5. ROLLBACK.ps1 幂等性复验

### 5.1 key_api_tx（复制到独立新目录，连跑两次）

复制：`work\key_api_tx\MODIFIED_FILE` → `work\api_doc_tx\_reg\rb_key_copy`
复制后 5 文件 sha256 = 4.1 表中 MODIFIED 列（`d6ca…` / `5d40…` / `a359…` / `4cc2…` / `ce04…`）。

第 1 次：
```
CMD powershell -NoProfile -ExecutionPolicy Bypass -File C:\Users\Administrator\Desktop\nai\work\key_api_tx\ROLLBACK.ps1 -Target C:\Users\Administrator\Desktop\nai\work\api_doc_tx\_reg\rb_key_copy
EXIT 0
STDOUT
  restored nai\client.go
  restored nai\ui.go
  restored nai\client_test.go
  restored nai\pipeline.go
  restored nai\sd_cmd.go
  ROLLBACK_OK
restored==baseline -> True
  nai/client.go        EQUAL d00d96703bbdb5f4faa0934993c7bf92c84d3ff83fd2e02c74115a9a61e1e5e0
  nai/ui.go            EQUAL 872976bf9f453e2fa08a3935e56d6c4114b75f75754bce0a670bff7e2d623221
  nai/client_test.go   EQUAL 2407833c001d2bab97d11ff4f1ff5bf5c6425cc1a2b75057bba5f0f41dc232e0
  nai/pipeline.go      EQUAL 36c45137bc6a293c76709ba16b2d260dc3fa4deb6e75b69f8b3f3b09eeac8583
  nai/sd_cmd.go        EQUAL 516662c55037061c2127a2737a9e360d43d5636e627dcece5ba6b660970aba1c
```
第 2 次（重复执行）：
```
CMD powershell -NoProfile -ExecutionPolicy Bypass -File ...\ROLLBACK.ps1 -Target ...\rb_key_copy
EXIT 0
STDOUT  同上（restored ×5 + ROLLBACK_OK）
restored==baseline -> True   （5 个哈希与第 1 次完全相同）
```
**PASS：幂等。**（脚本是“无条件拷贝 baseline → target”，重复执行结果收敛且不报错）

### 5.2 附：把回滚后的整棵树当仓库用时仍可构建/测试

```
命令  powershell ... ROLLBACK.ps1 -Target ...\_reg\rb_full_tree   -> EXIT 0  ROLLBACK_OK
      restored==baseline -> True
      go build -p 1 -o xianyu-nai.exe .   -> EXIT 0
      go test ./nai/                      -> EXIT 0  ok  xianyu-nai/nai  1.979s
```

### 5.3 api_doc_tx（连跑两次）

复制：`MODIFIED_FILE_API-doc.md` → `_reg\rb_doc_copy\API文档-NAI画图接口.md`（20814 字节，`83dfb94a…`）

第 1 次：
```
CMD powershell -NoProfile -ExecutionPolicy Bypass -File C:\Users\Administrator\Desktop\nai\work\api_doc_tx\ROLLBACK.ps1 -Target C:\Users\Administrator\Desktop\nai\work\api_doc_tx\_reg\rb_doc_copy
EXIT 0
STDOUT  ROLLBACK_OK
after attempt 1: doc_exists=False
```
第 2 次（重复执行，文档已不存在）：
```
CMD powershell ... -File ...\ROLLBACK.ps1 -Target ...\rb_doc_copy
EXIT 0
STDOUT  ROLLBACK_OK (already absent)
after attempt 2: doc_exists=False
```
**PASS：幂等，且第二次命中 “already absent” 分支。**

---

## 6. DIFF 逐字节重建（自写 unified diff 应用器，不依赖 git/patch）

应用器：`%TEMP%\minidiff.py`（纯 Python，按字节读写，支持 `--- a/x` / `+++ b/x`、`@@ -a,b +c,d @@`、上下文 ` `/新增 `+`/删除 `-`、`\ No newline` 忽略行、`--- /dev/null` 新建文件；遇上下文/删除行不匹配即抛错）。

### 6.1 key_api_tx

```
CMD python minidiff.py work\key_api_tx\DIFF_FILE.patch work\key_api_tx\baseline \
        _reg\patch_out_key work\key_api_tx\MODIFIED_FILE _reg\patch_apply_key.txt
EXIT 0
PATCH_FILES=5 PATCH_HUNKS=7
nai/client.go            bytes=16981  sha=d6ca37e1bdd8749b8b7c9b7fdb7d22054469668fa3cba16007eee180d1b5a104  reconstructed==VERIFY(sha+bytes): True
nai/ui.go                bytes=31802  sha=5d402c30963fd870ff82eb9c7164e3d0e34a85165b182a0fdce43b1b3754e3be  reconstructed==VERIFY(sha+bytes): True
nai/client_test.go       bytes=3362   sha=a3594eaf0ca5ca2cf4857cb3e81215cee5e58361da1bf976a14811ed0e42bcbb  reconstructed==VERIFY(sha+bytes): True
nai/pipeline.go          bytes=9972   sha=4cc2f198cc77544a0c5b62d26a373cb5963556a971c8a37f7d5ffcd4b3eb92fd  reconstructed==VERIFY(sha+bytes): True
nai/sd_cmd.go            bytes=13316  sha=ce04b336fa8b20177ca08b340bd0d31ff1a087fd91963b25e9ec6f9158374be4  reconstructed==VERIFY(sha+bytes): True
PATCH_APPLY_OK=True
```

### 6.2 api_doc_tx（从 /dev/null 重建 477 行新文件）

```
CMD python minidiff.py work\api_doc_tx\API-doc.diff work\api_doc_tx\target_baseline \
        _reg\patch_out_doc work\api_doc_tx\target_modified _reg\patch_apply_doc.txt
EXIT 0
PATCH_FILES=1 PATCH_HUNKS=1
API文档-NAI画图接口.md  bytes=20814  sha=83dfb94ad5f146b2999430b2ea367bd8b8e1db92f7c739fa6f2079bd6bfd06b7  reconstructed==VERIFY(sha+bytes): True
PATCH_APPLY_OK=True
```
重建出的文档再跑 `probe.py`：`RESULT=DOC_PRESENT / BYTES=20814 / SHA256=83dfb94a… / ALL_CHECKS_PASS=True`。

### 6.3 附：用重建结果组装的整棵树必须可编译

```
tree = _reg\tree_from_patch   （仓库副本 + patch 重建的 5 个 .go 覆盖）
go build -p 1 -o xianyu-nai.exe .   -> EXIT 0
go vet ./...                        -> EXIT 0（无输出）
go test ./nai/                      -> EXIT 0  ok  xianyu-nai/nai  1.875s
gofmt -l .                          -> EXIT 0（0 行输出）
```

---

## 7. 三态行为复现（同一输入，独立进程）

三个可执行文件：`build_baseline\xianyu-nai.exe`（BASELINE）、`build_modified\xianyu-nai.exe`（MODIFIED）、`_reg\rb_full_tree\xianyu-nai.exe`（ROLLBACK 后的重建产物）。

输入（反代通道，`-dump` 只打印不发送）：
```
-dump -prompt 1girl -key nai3-test -proxy https://nai.xianyuw.cn -width 832 -height 1216
```
| 状态 | EXIT | 关键字段（字面） |
|---|---|---|
| BASELINE | 0 | `"scale": 5` / `"steps": 28` / `"promptGuidanceRescale": 5` |
| MODIFIED | 0 | `"scale": 5` / `"steps": 28` / `"promptGuidanceRescale": 0.28` |
| ROLLBACK | 0 | `"scale": 5` / `"steps": 28` / `"promptGuidanceRescale": 5` |

```
BASELINE : promptGuidanceRescale=5     （原样复现）
MODIFIED : promptGuidanceRescale=0.28  （GOAL 生效）
ROLLBACK : promptGuidanceRescale=5     （逐字节回滚后行为回到 baseline）
```
**PASS：可逆、确定性。**

同时验证仓库 HEAD 构建的 exe（步骤 1.2 产物）也是 `promptGuidanceRescale=0.28`，说明仓库当前处于 MODIFIED 状态，与 4.3 结论一致。

另一个观察（见 F2）：官方通道 `-dump`（`-mode 官方 -image probe.png`）在**三态下都输出 `"strength": 0.28`**，因为 CLI 的 `-strength` flag 默认值仍是 0.28 并总是覆盖 `cfg.Strength`，所以 `DefaultConfig().Strength=0.52` 这条修正在 CLI 路径上不可见（只有 ini/OneBot 路径能取到 0.52）。

---

## 8. 未修复的问题 / 观察

**F1（低，文档/UI 语义）** `run\咸鱼ai画图.ini` 的 `[nai3设置] 模式=中转` 不在 `UISchema()` 的 select 取值域内：
`ui.go:275` 声明 `sel("模式", "模式（官方 / 反代）", "", "", "官方", "反代")`，opts 只有 `""`/`官方`/`反代`。
- 运行期无影响：`Client.isOfficial()` 只判 `Mode == "官方"`，`中转` 走反代，与 DLL 默认值（`0x102B5C04 = "中转"`）一致；
- 但面板 `<select>` 没有 `中转` 这个 option，浏览器会把选中项落到空值 → 面板显示为空；`WriteConfig()` 对空值不落盘，所以保存不会破坏 ini。属“显示误导”，不是数据损坏。
- 未修（改 schema 属于业务代码改动）。

**F2（低，一致性）** `main.go:37` 的 `flag.Float64("strength", 0.28, ...)` 默认值仍是旧值 0.28，而 `nai.DefaultConfig().Strength` 已是 0.52、schema def 也是 0.52。
由于 `main.go:138` 无条件 `cfg.Strength = *strength`，CLI 路径永远用 0.28；`[nai3参数] strength=0.52` 只有走 `LoadNAIConfigFromINI`（OneBot / `-ui` 热重载）才生效。角色 A 的 VERIFICATION 里 `strength` 一项没有出现在 CLI dump 证据中，原因即此。未修（业务代码）。

**F3（提示，非缺陷）** 117 个 schema 键不在 ini 中。这是设计（`ReadConfig` 用 def 补齐、`WriteConfig` 只写非空值），`/api/config` 实测返回完整 135 键；不是缺配置。

**F4（提示）** `nai3参数/宽度=832`、`高度=1216` 与 schema def 1024×1024 不同，`nai3指令/画图=nai画图` 与 def `画图` 不同 —— 均为用户自定义，属正常覆盖，不是漂移。

**F5（提示）** 仓库当前为 MODIFIED 状态（5 个 .go + API 文档都已落盘），`baseline\` 是回滚源；本次未回滚仓库本体（按任务要求只验产物，不动仓库）。

---

## 9. 证据文件索引（全部在 `work\api_doc_tx\_reg\`）

| 文件 | 内容 |
|---|---|
| `build.code.txt` / `build.out` / `build.err` | go build 退出码与输出 |
| `test.out` / `test.err` / `test_utf8.out` | go test -v 全量日志（UTF-8） |
| `test_all.out` | `go test ./...` |
| `test_counts.txt` | RUN/PASS/FAIL/SKIP 计数 |
| `vet.out` / `vet.err` / `gofmt.out` / `gofmt.err` | vet / gofmt |
| `ini_copy.ini` | ini 原件字节副本 |
| `ini_vs_schema.txt` | 节/键差异全量清单 |
| `ini_value_validation.txt` | 值类型/取值域与 def 对比 |
| `ui_api_schema.json` / `ui_api_config.json` / `ui_api_status.json` / `ui_api_presets.json` | 4 个端点的完整响应 |
| `ui_smoke_raw.json` / `ui_smoke_summary.json` | 冒烟原始/汇总 |
| `ui_server.log` / `ui_server.err` | 面板进程 stdout/stderr |
| `hash_table.txt` | repo/MODIFIED/baseline 三方 sha256 全表 |
| `manifest.txt` | 两套角色 + 目标树的文件清单（大小 + sha256 + 聚合哈希） |
| `rollback_idempotency.txt` | 两套 ROLLBACK 各两次的字面输出与哈希比对 |
| `patch_apply_key.txt` / `patch_apply_doc.txt` | 自写应用器的逐文件重建结果 |
| `patch_out_key\` / `patch_out_doc\` | 重建产物 |
| `patch_build.*` / `patch_vet.*` / `patch_test.*` | 重建树的构建/静态检查/测试 |
| `rb_full_tree\` | 回滚后的完整仓库副本（含可构建产物） |
| `dump_baseline.json` / `dump_modified.json` / `dump_rollback.json` | 三态 `-dump` 请求体 |
| `dumpoff_*.json` | 三态官方通道 `-dump`（暴露 F2） |
| `doc_probe.txt` | probe.py 五个目标的字面输出 |
| `tree_identity.txt` | 仓库 vs build_baseline/build_modified 的树级比对 |
| `zhongzhuan.txt` | 全 work 目录"中转"字面出处（佐证 DLL 默认值） |
