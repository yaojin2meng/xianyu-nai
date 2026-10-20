# 审计报告：`xianyu-nai/API文档-NAI画图接口.md`

- 审计日期：2026-10-20
- 审计对象：`C:\Users\Administrator\Desktop\nai\xianyu-nai\API文档-NAI画图接口.md`
- 修正前：20814 字节 / 477 行 / sha256 `83dfb94ad5f146b2999430b2ea367bd8b8e1db92f7c739fa6f2079bd6bfd06b7`
- 修正后：26589 字节 / 493 行 / LF（LF 493、CRLF 0）/ 无 BOM / sha256 `31c2990603e288965b921fb35923eebac97e115d1d050b8883e3c15bc5958144`
- 核对范围：端点路径、HTTP 方法、请求头名、请求体字段名与类型、RVA/VA 地址、错误码与错误文案、数值约束、鉴黄判定语义、ini 节名/键名
- 结论口径：**一致** / **部分一致** / **不一致** / **无证据**（无证据一律记「未确认」，不补推断）

## 一、总计

| 项目 | 数量 |
|---|---|
| 总检查条数 | 155 |
| 一致 | 131 |
| 一致（叙述性，不计技术断言 #152） | 1 |
| 部分一致 | 0 |
| 不一致 | 19 |
| 无证据（3 条已改标「未确认」#33/#80/#121 + 1 条已加注 #149） | 4 |
| 已修正/补充清单条数 | 27（19 条不一致 + 7 条「补证据型」#9/#15/#20/#21/#22/#23/#24 + 1 条改标 #25） |
| 仍「未确认」清单条数 | 10（U1–U9、U11；U10 已补证关闭） |
| 文档内 `0x` 唯一值 | 76 = 37 个字符串/常量数据地址 + 28 个指令地址 + 1 个 ImageBase（`0x10000000`）+ 5 个 RVA + 5 个 `[ebp-0x…]` 栈偏移 |
| 其中数据地址逐条取串命中 | 37 / 37（无一处取值与描述不符；`0x102B7FA8` 是 double `-1e-07`，按字节取） |
| 指令地址核对 | 28 / 28（反汇编段与 `0x100A*` 取串读点逐条对位，见 `dis_mode.txt`、`code_audit3.txt`、`ini_sites.txt`） |
| 自相矛盾（同字段两处说法不同 / 示例与字段表冲突 / 代码块未闭合） | 3 处已修（#7+#8、#14、#23），0 处遗留 |

> 口径：155 = 131 一致 + 1 叙述性（#152，非技术断言）+ 19 不一致 + 4 无证据。19 条不一致全部就地改成与证据一致的说法，无一遗留。4 条无证据的断言里 3 条（#33 官方可达性、#80 `height` 越界、#121 其余 6 个端点）改标「未确认」并登记到 §四 U1/U5/U7，#149（假服务器 200）加注来源并登记 U11。27 条修改清单 = 19 条不一致修正 + 7 条补证据 + 1 条改标。

## 二、逐条核对

### A. 文档开头与 §一（key 身份）1–15

| # | 位置 | 文档断言 | 结论 | 证据（文件:行） |
|---|---|---|---|---|
| 1 | 文首 | `work/unpacked.dll` 脱壳后 5,390,336 字节 | 一致 | `proxy_types.txt:14` = `dll size=5390336` |
| 2 | 文首 | ImageBase `0x10000000`，标注 RVA | 一致 | 37 个数据/常量 VA 全部按「VA − 0x10000000 = 文件偏移」取串命中，见 `final_va_check.txt` |
| 3 | 文首 | `wire.txt` 收录 6 个用例 | 一致 | `wire.txt` 含 6 个 `CASE`；`wire.json` 为 6 元素数组 |
| 4 | §1 | 官方持久令牌一律 `pst-` 前缀 | 一致 | `docs_strings.txt:17` = `pst-` |
| 5 | §1 | 「第三方令牌: 其他所有令牌将通过外部服务进行验证」 | 一致 | `docs_strings.txt:18`（字面全同） |
| 6 | §1 | `nai3-` 前缀是第三方反代（中转）平台的 key，不是官方 key | 一致 | `live_r1.txt:1` 用该 key 在 `nai.xianyuw.cn` 真实出图；DLL 内查无 `pst-` 字面（`endpoint_final.txt:17`） |
| 7 | §1 表 | `key` 位于 `[nai3设置] key=` | 一致 | 节名 VA `0x102B5C14`；键取点 `0x100A3F67 push 0x1039aa38`（`ini_sites.txt:48`） |
| 8 | §1 表 | `nai3api` 位于 `[重制版] nai3api=` | **不一致 → 已修正（#1）** | DLL 查无「重制版」：`augment_probe.txt:13` = `NOT FOUND`；`addr_deep.txt:46` 同 |
| 9 | §1 表 | `模式` 留空时默认 `中转`，默认串 VA `0x102B5C04`，读点 `0x100A4051` | 一致 | `final_va_check.txt:2` = `中转`；读点见 `ini_sites.txt` |
| 10 | §1 表 | 只有字面等于 `官方` 才走官方通道 | 一致 | `0x1005E3EB call 0x10001C40` + `cmp eax,0` + `sete al`（`dis_mode.txt`） |
| 11 | §1 | 实测 `POST https://nai.xianyuw.cn/api/generate_image` + `Authorization: Bearer <key>` | 一致 | `live_r1.txt:1`；头见 `wire.txt:8-10` |
| 12 | §1 | 返回 `200` 且响应体为该 JSON 字面 | 一致 | `live_r1.txt:1` 逐字符一致 |
| 13 | §1 注 | 两个硬编码域名 `res.xianyuw.cn` `0x103D2F1C`、`xianyu.xianyuw.cn` `0x1039B1AC` | 一致 | `xianyuw_strings.txt:1-2`（`http://xianyu.xianyuw.cn/json.php`、`https://res.xianyuw.cn/plug/chatai/ai_studio/QQCharacterTts/?content=`） |
| 14 | §1 注 | 「唯一硬编码的资源域」 | **不一致 → 已修正（#2）** | 硬编码 `xianyuw.cn` 域名实为 2 个，见 #13 |
| 15 | §1 注 | `nai.xianyuw.cn` 由同域子域探测所得，非购买凭证下发 | 一致 | 全仓库检索「下发」只有 `nai3api`/`naiofficialapi`/`nai3_plugin-api`/`资源地址`/`upload-image` 五项（`xiafa.txt:3-19`，`部署说明.md:136-137`），无 `nai.xianyuw.cn` 下发记录；DLL 查无该地址（`endpoint_final.txt:16`） |

### B. §二 通道选择机制 16–33

| # | 位置 | 文档断言 | 结论 | 证据（文件:行） |
|---|---|---|---|---|
| 16 | §2 | 判定点 `0x1005E3E0`–`0x1005E3FE`「逐条实测」 | 一致 | `dis_mode.txt:19-27` 完整导出该区间 8 条指令，与文档代码块逐条同址同助记符同操作数（`push 0x102b5c20` / `push [ebp-0xd0]` / `call 0x10001c40` / `add esp,8` / `cmp eax,0` / `mov eax,0` / `sete al` / `mov [ebp-0xd4],eax`） |
| 17 | §2 代码块 | `push 0x102B5C20 ; "官方"` | 一致 | `final_va_check.txt:3` = `官方` |
| 18 | §2 代码块 | `call 0x10001C40 ; 字符串比较` | 一致 | `dis_mode.txt` 该调用紧接两次 push 后取结果比较 |
| 19 | §2 代码块 | `cmp eax,0` + `sete al` → `al = (比较结果==0)` 即「等于官方」 | 一致 | `dis_mode.txt` |
| 20 | §2 表 | 官方端点 `/ai/generate-image`（RVA `0x2BA9C0`） | 一致 | `endpoint_literals.txt:4` = `/ai/generate-image` |
| 21 | §2 表 | 反代端点 `/api/generate_image`（RVA `0x2BA8AC`） | 一致 | `endpoint_literals2.txt:1` = `/api/generate_image` |
| 22 | §2 表 | 官方 URL 拼接先判 base 是否以 `/` 结尾（`0x100637C5` push `0x102BA9A0`） | 一致 | `final_va_check.txt:34` = `/`；`dis_mode.txt` 记该 push |
| 23 | §2 表 | 反代裸拼，`0x1005E4F3` push `0x102BA8AC`、`ecx=2` | 一致 | `dis_mode.txt` |
| 24 | §2 表 | 官方 key 名为 `官方key`（`pst-…`） | 一致 | `last_checks2.txt:1` = `官方key -> gbk@0x102BA950, gbk@0x1039AA38` |
| 25 | §2 表 | 官方请求体 `{input, model, action, parameters{…}}` | 一致 | `official_body_dump.txt:1` |
| 26 | §2 表 | 反代请求体为「扁平 13 字段」 | 一致 | `wire_final_check2.txt:1`（keys=13） |
| 27 | §2 表 | 官方 `width/height/steps` 是**字符串** | 一致 | `official_body_dump.txt:2,3,6` = `"832"`/`"1216"`/`"28"` |
| 28 | §2 表 | 官方 `seed` 是**数字** | 一致 | `official_body_dump.txt:7` = `8888`（int） |
| 29 | §2 表 | 官方返回 `{statusCode, message, images:[{image,seed}]}` | 一致 | `client.go:389-395` |
| 30 | §2 表 | 官方 `image` 是 `base64(zip(png))` | 一致 | `client.go:407` + `pipeline.go:222`(`DecodeOfficialImage`)/`250`(`unwrapZipIfAny`) |
| 31 | §2 表 | 反代返回 `{images:[base64…]}` 或 `{image_url}` | 一致 | `client.go:422-429`、`440-446` |
| 32 | §2 注 | 官方通道「本机不可达（连接超时，`WinError 10060`）」 | **不一致 → 已修正（#3）** | 工程内无该连接测试字面记录；抓包里 `pst-` 是占位符 `pst-xxxxxxxx`（`headers_dump.txt:2`） |
| 33 | §2 注 | `image.novelai.net:443` 可达性 | **未确认**（已改标） | 见 U1 |

### C. §三 公共约定 34–47

| # | 位置 | 文档断言 | 结论 | 证据（文件:行） |
|---|---|---|---|---|
| 34 | §3.1 | 三件套头 `Content-Type` / `Accept` / `Authorization: Bearer` | 一致 | `headers_dump.txt:1-6` 六个用例头全同 |
| 35 | §3.1 | `Authorization: Bearer ` 字面量在 `0x102B8178` | 一致 | `final_va_check.txt:6` = `Authorization: Bearer `（含尾空格） |
| 36 | §3.1 | 两通道共用同一套头（`client.go:458-460`） | 一致 | `code_lines_check.txt:12-14` 三行 Header.Set |
| 37 | §3.2 | 默认超时 `300000ms`（`client.go:128`） | **不一致 → 已修正（#4）** | 实际在 `client.go:129`（`code_lines_check.txt:5`） |
| 38 | §3.2 | 真实反代单张「6–16 秒」 | **不一致 → 已修正（#5）** | 实测区间 6.0–16.4s：`auth_matrix.txt:4` 6.3s、`auth_matrix.txt:3` 6.0s、`field_matrix.txt:5` 16.4s |
| 39 | §3.3 | 没带 key → 401 `Token is required` | 一致 | `auth_matrix.txt:1`（`no_key`） |
| 40 | §3.3 | key 不存在 → 401 `Token not found` | 一致 | `auth_matrix.txt:2`（`bad_key`） |
| 41 | §3.3 | 缺 `positivePrompt` → 400 且响应体为该长串 | 一致 | `probe_endpoints.txt:1` 逐字符一致 |
| 42 | §3.3 | 缺 `model` → 400 且响应体为该长串 | 一致 | `probe_endpoints.txt:2` 逐字符一致 |
| 43 | §3.3 | `steps > 50` → 400 `steps不能大于50` | 一致 | `field_matrix.txt:8` |
| 44 | §3.3 | `promptGuidanceRescale > 1` → 400 且给出完整 JSON 外壳 | **不一致 → 已修正（#6）** | `live_matrix.json:7` 只有客户端串 `nai错误响应：promptGuidanceRescale不能大于1`，无 JSON 外壳 |
| 45 | §3.3 | 上游官方鉴权失败 → 500 `{"status":"failed","error":"HTTP 401: Unauthorized",…}` | 一致 | `auth_matrix.txt:5`（`offic key form`） |
| 46 | §3.3 尾 | 平台声明 `200/400/401/429/500/503` 及 `503` 释义「所有上游不可用」 | **不一致 → 已修正（#27）** | `docs_strings.txt:67-80`：`200` 的释义是「请求成功」（不是「成功」），`503` 的释义是「服务不可用（所有API接口都不可用）」（不是「所有上游不可用」） |
| 47 | §3.3 尾 | `429`/`503` 本次未实测触发 | 一致 | 全部矩阵文件无 429/503 记录（`field_matrix.txt` 8 行、`auth_matrix.txt` 5 行） |

### D. §四 反代通道 48–90

| # | 位置 | 文档断言 | 结论 | 证据（文件:行） |
|---|---|---|---|---|
| 48 | §4.1 | `POST {nai3api}/api/generate_image` | 一致 | `wire.txt` CASE `proxy_t2i`：`POST /api/generate_image`（`wire_final_check.txt:1`） |
| 49 | §4.1 | 常量 RVA `0x2BA8AC`（VA `0x102BA8AC`），拼接点 `0x1005E4F3` | 一致 | `endpoint_literals2.txt:1` + `dis_mode.txt` |
| 50 | §4.2 | 请求体 13 字段 | 一致 | `wire_final_check2.txt:1`（keys=13） |
| 51 | §4.2 标题 | 「字段名/顺序/类型与抓包**逐字节**一致」 | **不一致 → 已修正（#7）** | 抓包该字段值为 `5`，文档写 `0.28`，非逐字节 |
| 52 | §4.2 | 13 个字段名与抓包一致 | 一致 | `wire_final_check2.txt:1`（`model,positivePrompt,negativePrompt,scale,steps,width,height,promptGuidanceRescale,noise_schedule,seed,sampler,sm,sm_dyn`） |
| 53 | §4.2 | 类型：`scale/steps/width/height` 数字、`seed` 字符串、`sm/sm_dyn` 布尔 | 一致 | `proxy_types.txt:4-13` |
| 54 | §4.2 示例 | `promptGuidanceRescale: 0.28` | **不一致 → 已加注（#8）** | 抓包原件为 `5`：`wire_line11.txt:4`（BODY 行）、`proxy_types.txt:8` |
| 55 | §4.2 表 | `scale` 默认 `5`，`0x100A410C` → `0x102B5AA0` | 一致 | `final_va_check.txt:1` |
| 56 | §4.2 表 | `steps` 默认 `28`，`0x100A41C7` → `0x1039AB5C` | 一致 | `final_va_check.txt:36` |
| 57 | §4.2 表 | `width` 默认 `1024`，`0x100A44B3` → `0x102B7E7C` | 一致 | `final_va_check.txt:4` |
| 58 | §4.2 表 | `height` 默认 `1024`，`0x100A456E` | 一致 | `final_va_check.txt:4`（同一默认串） |
| 59 | §4.2 表 | `cfg` 默认 `0.28`，`0x100A4282` → `0x1039AC60` | 一致 | `final_va_check.txt:37` |
| 60 | §4.2 表 | `promptGuidanceRescale` 写入点 `0x1005CAD6`（键串 `0x102BA770`） | 一致 | `write_offset.txt:26` = `0x1005CAD6  mov eax, 0x102ba770` |
| 61 | §4.2 表 | `noise_schedule` 键 `0x102BA788` | 一致 | `final_va_check.txt:30` |
| 62 | §4.2 表 | `seed` 键 `0x102B832C` | 一致 | `final_va_check.txt:7` |
| 63 | §4.2 表 | `seed` 默认 `-1` 随机，默认串 `0x102B8DD0`、读点 `0x100A43F8` | **一致**（原文只写键，补默认串=#9） | `final_va_check.txt:12` = `-1` |
| 64 | §4.2 表 | `sampler` 键 `0x102B94E0` | 一致 | `final_va_check.txt:20` |
| 65 | §4.2 表 | `sm` `0x102BA7A8`、`sm_dyn` `0x102BA7AC` | 一致 | `final_va_check.txt:31-32` |
| 66 | §4.2 表 | `model` 模板 `0x102BA5F8` | 一致 | `final_va_check.txt:24-27`（模板 JSON 起点 `{"model": "nai-diffusion-3",…`） |
| 67 | §4.2 注 | `strength` 默认 `0.52`，`0x100A433D` → `0x1039AB54` | 一致 | `final_va_check.txt:35` |
| 68 | §4.3 | 形态 A（只给 `image_url`）在 `nai.xianyuw.cn` 实测 | 一致 | `live_r1.txt:1` |
| 69 | §4.3 | 形态 B（`images` base64 数组）为假服务器契约 | 一致 | `wire_final_check.txt:8` = `seed=424242 images=1` |
| 70 | §4.3 规则1 | `message` 非空 → 抛 `nai错误响应：<message>` | 一致 | `client.go:434-435` |
| 71 | §4.3 规则2 | `images` 空且 `image_url` 空 → 视为失败 | 一致 | `client.go:437-438` |
| 72 | §4.3 规则3 | `images[0]` 是 zip 则解第一个非目录条目，否则裸 base64 | 一致 | `pipeline.go:250`（`unwrapZipIfAny`）+ `222` |
| 73 | §4.3 规则4 | 只有 `image_url` 时必须下载回填 | 一致 | `pipeline.go:139`（`FetchImageBytes` 回填） |
| 74 | §4.4 标题 | 表为「该平台的额外字段与硬约束（实测）」 | **不一致 → 已修正（#10）** | 表内容出自 `docs_strings.txt`（平台前端口径），非 DLL 出站字段 |
| 75 | §4.4 | `prompt` ≤2000 字符 | 一致 | `docs_strings.txt:27` |
| 76 | §4.4 | `negative_prompt` ≤1000 字符 | 一致 | `docs_strings.txt:32` |
| 77 | §4.4 | `width` 约束 64–1536 | 一致 | `docs_strings.txt:36` |
| 78 | §4.4 | 「实测传 `2000` 仍返回 200」 | **不一致 → 已修正（#11）** | `auth_matrix.txt:4` 只有用例名 `width_out` + 200，无传值 |
| 79 | §4.4 | `height` 约束 64–1536 | 一致 | `docs_strings.txt:41` |
| 80 | §4.4 | `height` 越界行为 | **未确认**（已改标） | 见 U5 |
| 81 | §4.4 | `steps` 约束 1–50 | 一致 | `docs_strings.txt:46` |
| 82 | §4.4 | 「实测 `60` → 400」 | **不一致 → 已修正（#12）** | `field_matrix.txt:8` 只有用例名 `steps60`，无传值 |
| 83 | §4.4 | `cfg_scale` 1–30 | 一致 | `docs_strings.txt:51` |
| 84 | §4.4 | 插件不发 `cfg_scale`，改发 `promptGuidanceRescale` | 一致 | `proxy_types.txt:1-13` 无 `cfg_scale` |
| 85 | §4.4 | `promptGuidanceRescale` ≤1；`pgr0`/`pgr0.7`/`pgr1` 均 200 | 一致 | `field_matrix.txt:2-4` |
| 86 | §4.4 | `seed` 数字或字符串都接受 | 一致 | 抓包字符串 `"123456"`（`proxy_types.txt:10`）+ `seed_int` 200（`field_matrix.txt:7`） |
| 87 | §4.4 | `sampler` 默认 `k_euler_ancestral`（平台）/ `k_euler`（插件） | 一致 | `docs_strings.txt:62` = `k_euler_ancestral`；`final_va_check.txt:20` = `sampler`，`proxy_types.txt:11` = `k_euler` |
| 88 | §4.4 | `noise_schedule` 可省略，省略后仍 200 | 一致 | `field_matrix.txt:1`（`no_noise` → 200） |
| 89 | §4.4 尾 | 「型号不校验：传 `model:"not-a-model"` 仍 200 并出图（按默认兜底）」 | **不一致 → 已修正（#13）** | `auth_matrix.txt:3` 只有用例名 `bad_model` + 200 |
| 90 | §4.5 | curl / PowerShell 示例字段与 §4.2 一致 | **不一致 → 已补注（#14）** | 示例为 12 字段，漏 `negativePrompt`（`proxy_types.txt` 13 键） |

### E. §五 官方通道 91–117

| # | 位置 | 文档断言 | 结论 | 证据（文件:行） |
|---|---|---|---|---|
| 91 | §5.1 | `POST {naiofficialapi}/ai/generate-image` | 一致 | `wire_final_check.txt:2` |
| 92 | §5.1 | 抓包 1308 字节 | 一致 | `headers_dump.txt:2`（Content-Length 1308） |
| 93 | §5.1 | `model` = `nai-diffusion-4-full` | 一致 | `official_body_dump.txt:1` |
| 94 | §5.1 | 顶层 4 字段 `input/model/action/parameters`，`input` 为空串 | 一致 | `official_body_dump.txt:1` |
| 95 | §5.1 注 | 省略的 10 个键及其值 | 一致 | `official_body_dump.txt:24-33` |
| 96 | §5.1 注 | `parameters` 共 32 键 | **一致**（原文未写数量，补=#15） | `wire_final_check2.txt:2`（params=32）、`wire_detail.txt:17` |
| 97 | §5.1 注 | `strength:0.28` 等值取自 `wire.json` | 一致 | `official_body_dump.txt:31` |
| 98 | §5.2 | img2img 在 32 键上多一个 `image`，共 33 键 | 一致 | `wire_final_check2.txt:5`（params=33） |
| 99 | §5.2 | 底图 base64 塞进 `parameters.image`，`input` 仍为 `""` | 一致 | `wire_detail.txt:32`（`param.image.prefix=… len=524840`）、`:35`（`input=''`）；`client.go:259`/`:220` |
| 100 | §5.2 | `add_original_image` 由 `true` 变 `false` | 一致 | `wire_detail.txt:33`（`param.add_original_image=False`） |
| 101 | §5.2 | body 长 526,158 字节 | 一致 | `headers_dump.txt:3` |
| 102 | §5.2 | base64 部分 524,840 字节 | 一致 | `wire_detail.txt:32` |
| 103 | §5.2 | 底图 1920×1200、393,630 字节 | 一致 | `augment_probe.txt:95` = `jpg bytes=393630 dims=1920x1200` |
| 104 | §5.2 | 「底图来源支持本地路径、http(s) URL、`base64://` 三种（`client.go` 的 `loadImageAsDataURL`）」 | **不一致 → 已修正（#16）** | `loadImageAsDataURL` 全仓库不存在；`b64_sites.txt:4-7` 只在 `FetchImageBytes`/`ImageSeg` |
| 105 | §5.2 | `readImage(path string)` 只 `os.ReadFile` + base64，仅本地路径 | 一致 | `main.go:451-459`（`main_readImage.txt:13-21`） |
| 106 | §5.2 | `base64://` 只出现在 `comfy_handle.go:44-46` 与 `onebot.go:161-164` | 一致 | `b64_sites.txt:4-7` |
| 107 | §5.3 | `statusCode != 0` 或 `message != ""` → `官方接口错误` | 一致 | `client.go:400-402` |
| 108 | §5.3 | `images` 为空 → `官方接口返回异常` | 一致 | `client.go:404-405` |
| 109 | §5.3 | `images[0].image` base64→zip→第一个非目录条目 | 一致 | `client.go:407` + `pipeline.go:250` |
| 110 | §5.4 | `/ai/augment-image` RVA `0x2BA9A4` | 一致 | `endpoint_literals2.txt:2` |
| 111 | §5.4 | 抓包 524,927 字节 | 一致 | `headers_dump.txt:4` |
| 112 | §5.4 | 「抓包是 3 字段」（修正前） | **不一致 → 已修正（#17）** | `official_body_dump.txt:36` 为 6 字段 |
| 113 | §5.4 | 6 字段含 `width:1024,height:1024,defry:0` | 一致 | `official_body_dump.txt:36` |
| 114 | §5.4 | `req_type` 有 7 个取值（含 `bg-removal`/`superresolution`） | **不一致 → 已修正（#18）** | `augment_probe.txt:1-4,9` 只有 5 个；`:5-7` 两串 `NOT FOUND` |
| 115 | §5.4 注 | `bg-removal`/`superresolution` 走 `/api/remove_background`、`/api/superResolution_image` | 一致 | `endpoint_final.txt:2-3` |
| 116 | §5.5 | `/user/subscription` RVA `0x2BB014`，GET，无 body | 一致 | `endpoint_literals.txt:5`；`headers_dump.txt:5`（GET，无 Content-Length） |
| 117 | §5.5 | 额度渲染输出的 6 行字面 | 一致 | `wire_final_check.txt:12`（`===【NAI 额度信息】===` 起共 6 行，含 Opus / 9500 / 1200 / 62.50%） |

### F. §六 `nai3_plugin-api` 118–137

| # | 位置 | 文档断言 | 结论 | 证据（文件:行） |
|---|---|---|---|---|
| 118 | §6 | `nai3_plugin-api` RVA `0x2B8DEC` | 一致 | `endpoint_literals.txt:6` |
| 119 | §6 表 | 7 个端点路径全部存在于 DLL | 一致 | `endpoint_final.txt:1-7` 七条全命中 |
| 120 | §6 | 全部带同一套 Bearer 头 | 一致 | `audit.go:108` 走 `postJSON(ctx, url, c.cfg.Key, …)` |
| 121 | §6 表 | 除 `/api/nsfw_check` 外 6 个端点「未确认」 | **未确认**（标注正确） | 见 U7 |
| 122 | §6.1 | `api4ai` 分支 `{type,image}`，`image` 为本地路径 | 一致 | `wire.txt` nsfw body = `{"type":"api4ai","image":"C:\\Windows\\Web\\Wallpaper\\Windows\\img0.jpg"}`（`official_body_dump.txt:37`） |
| 123 | §6.1 | baidu/baiduai 分支字段名为 `img` | **不一致 → 已修正（#19）** | `va_strings.txt:45` = `0x102B8E74 imgUrl`；`audit.go:52` 为 `json:"imgUrl,…"` |
| 124 | §6.1 | 解析路径 `results[0].entities[0].classes.nsfw` / `result.conclusion` / `result.auditResult.conclusion` | 一致 | `audit.go:66-68`；`addr_deep.txt` 该段字符串含 `ntities[0].classes.nsfw` 与 `result.conclusion` |
| 125 | §6.1 | 判定语义 `nsfw − threshold >= −1e-7` → 违规 | 一致 | `code_audit3.txt`（`fld`/`fsub`/`fcomp`/`test ah,1`/`jne`）；`audit.go:86` 注释同 |
| 126 | §6.1 | `0x102B7FA8` 的 double 值 = `-1e-07` | 一致 | `addr_deep.txt` = `as double little-endian = -1e-07`（bytes `48afbc9af2d77abe`） |
| 127 | §6.1 代码块 | 7 行反汇编（`0x1002E4CB`…`0x1002E4E5`） | 一致 | `code_audit3.txt`；`0x1002E4E5 mov [ebp-0x44],1` 为违规分支 |
| 128 | §6.1 注 | 阈值默认 `0.5` 在 `0x102B8E60`，键名 `阈值` 在 `0x102B8E64` | 一致 | `final_va_check.txt:14-15` |
| 129 | §6.1 | 失败一律判违规，文案 `审核失败，为了安全，强制判定为违规图` | 一致 | `final_va_check.txt:13`；`audit.go:71,106` |
| 130 | §6.2 | `upload.php` 端点 `0x102B94F4` | 一致 | `final_va_check.txt:22` = `/plug/AI/v1/upload_image/upload.php` |
| 131 | §6.2 | 五个字段字面 `0x102B94C4/94D0/94D8/94E0/94E8` | 一致 | `final_va_check.txt:17-21`（`image_url/user_qq/bot_qq/sampler/model_name`） |
| 132 | §6.2 | `Content-Type: application/json` 在 `0x102B9518` | 一致 | `final_va_check.txt:23` = `Content-Type:application/json` |
| 133 | §6.2 | `upload-image` 配置键 `0x102B8D24` | 一致 | `final_va_check.txt:9` |
| 134 | §6.2 | multipart 字段名 `file` 在 `0x102B8D34` | 一致 | `final_va_check.txt:10` |
| 135 | §6.2 | boundary 写死 `WebKitFormBoundary`；整条头在 `0x102B8D3C` | **一致**（原只写分界符，补整头=#20） | `final_va_check.txt:11` = `Content-Type: multipart/form-data; boundary=WebKitFormBoundary` |
| 136 | §6.2 | 分界符 `--WebKitFormBoundary` 在 `0x103D0EBC` | 一致 | `final_va_check.txt:39` |
| 137 | §6.2 注 | 兜底路径 `/plug/AI/upload-image` 只在 Go 侧（`publish.go:96`），DLL 查无 | **一致**（补注=#21） | `endpoint_final.txt:8` = `NOT FOUND`；`addr_deep.txt:38` 同；`publish.go:96` 字面相符 |

### G. §七～§十 138–154

| # | 位置 | 文档断言 | 结论 | 证据（文件:行） |
|---|---|---|---|---|
| 138 | §7.1 | 命令行 `-prompt/-key/-proxy/-steps/-width/-height` | 一致 | `main.go:19,21,25,27,30-32`（`flags_cfg.txt:19-32`） |
| 139 | §7.1 | 实测输出 `seed=3064907154 images=0` + `image_url: …` | 一致 | `live_matrix.json` MODIFIED 段 `stdout`（`flags_cfg.txt:129`） |
| 140 | §7.1 | 该次 `sec=9.4` | **一致**（补来源=#22） | `live_matrix.json` MODIFIED `"sec": 9.4` |
| 141 | §7.2 | ini 节名 `[nai3设置]/[nai3参数]/[nai3指令]` 与 11 个键名 | 一致 | `nai_ini.go:56-58`；`ini_defaults2.txt` 各键读点 |
| 142 | §7.2 | 示例 `cfg=0.28`、`strength=0.52` | 一致 | `client.go:93,102`（`flags_cfg.txt:105,114`） |
| 143 | §7.2 | `模式=中转` 与原插件默认一致 | 一致 | `final_va_check.txt:2` = `中转` |
| 144 | §7.2 | OneBot 回包 `segments=["text","image"]`，图片 641,057 字节 | 一致 | `onebot_matrix.json`（`VERIFICATION.txt:127` 记 641,057） |
| 145 | §7.2 | 681,357 字节那次为另一脚本/另一提示词 | **一致**（已区分=#23） | `onebot_e2e.py` 提示词 `1girl, solo, upper body, white background`（`audit_onebot_check.txt:42`），`onebot_e2e2/3.py` 才是 `1girl, solo`（`:43-44`） |
| 146 | §7.3 | `-ui` / `-ui-addr 127.0.0.1:8088` | 一致 | `main.go:69-70`（`flags_cfg.txt:52-53`） |
| 147 | §8 | Go 调用面字段（`Mode/Key/ProxyURL/Model/Width/Height/Steps/Cfg/Strength`）与 `Generate(ctx, prompt, negative, inputImage)` | 一致 | `client.go:17-37,90-104,342`（`flags_cfg.txt:81-115`、`verif_line.txt:19`） |
| 148 | §9 表 | 6 个假服务器用例的 `exit=0` 与 `seed=424242/777001/777001/777002` | **一致**（原写 200，已改=#24） | `wire_final_check.txt:8-13`（stdout 字面） |
| 149 | §9 表 | 假服务器用例的 HTTP `200` | **无证据 → 已加注（#25）** | `wire.txt`/`wire.json` 无状态码字段；`nai_fake.py:98` 默认 `code=200` |
| 150 | §9 表 | 真机 t2i 200 / 9.4s / 出 `image_url` | 一致 | `VERIFICATION.txt:126` = `HTTP 200，返回 image_url，exit=0` |
| 151 | §9 表 | `steps60` 400、`bad_model` 200、`promptGuidanceRescale=5` 400 | 一致 | `field_matrix.txt:8`、`auth_matrix.txt:3`、`live_matrix.json:7` |
| 152 | §10 | 误写第三方 `aiurl.php` 文件的自述 | 叙述性、不影响技术断言 | 无对应技术断言需核 |
| 153 | 全文 | 代码围栏成对闭合 | 一致 | 52 个围栏（26 组），偶数且全闭合 |
| 154 | 全文 | 表格行尾 `\|` 完整 | 一致 | 无缺尾竖线的表格行 |

### H. 交叉引用补充 155

| # | 位置 | 文档断言 | 结论 | 证据（文件:行） |
|---|---|---|---|---|
| 155 | §1 注 | `[重制版]` 那段注释的出处写作 `nai/client.go:48-50` | **不一致 → 已修正（#26）** | `nai/client.go:48` = `SendMode string`（`Config` 字段）；该注释实际在 `nai/nai_ini.go:48-50` |

## 三、已修正/补充清单（27 条）

其中 19 条是**断言与证据不符**（#1–#8、#10–#14、#16–#19、#26、#27），7 条是**原断言不算错但缺数量/地址/来源**而补齐（#9、#15、#20、#21、#22、#23、#24），1 条是**把无字面记录的「200」改标**（#25）。

每条格式：**原文 → 改为 → 依据**。

### #1 `[重制版]` 节名归属
- **原文**：`| nai3api | [重制版] nai3api= | https://nai.xianyuw.cn |`（无任何说明，读起来像 DLL 原生节）
- **改为**：§1 表格下新增注——「`[重制版]` 这一节是 Go 重制版新增的：原 DLL 里查无「重制版」字样…`nai3api`/`naiofficialapi`/`nai3_plugin-api` 原本由授权服务器下发」
- **依据**：`work/api_doc_tx/augment_probe.txt:13` = `重制版 NOT FOUND`；`work/api_doc_tx/addr_deep.txt:46` 同；`xianyu-nai/nai/nai_ini.go:48-50`、`xianyu-nai/nai/ui.go:413`

### #2 `nai.xianyuw.cn` 的来源描述
- **原文**：「按插件里**唯一**硬编码的资源域（`res.xianyuw.cn` / `xianyu.xianyuw.cn`）逐个探测」
- **改为**：「按插件里硬编码的**两个** `xianyuw.cn` 域名（`res.xianyuw.cn` `0x103D2F1C`、`xianyu.xianyuw.cn` `0x1039B1AC`）」
- **依据**：`work/api_doc_tx/xianyuw_strings.txt:1-2`（两条都是硬编码常量，不是「唯一」）

### #3 官方通道可达性
- **原文**：「官方通道在本机不可达（`image.novelai.net:443` 连接超时，`WinError 10060`）」
- **改为**：「官方通道未做公网直连（抓包里 `pst-` key 是占位符 `pst-xxxxxxxx`）；`image.novelai.net:443` 在本机是否可达**未确认**」
- **依据**：工程内无该连接测试的任何字面输出；`work/api_doc_tx/headers_dump.txt:2` 显示 `Bearer pst-xxxxxxxx`

### #4 超时行号
- **原文**：`client.go:128`
- **改为**：`client.go:129`
- **依据**：`C:\Users\Administrator\Desktop\nai\xianyu-nai\nai\client.go:129` = `cfg.TimeoutMS = 300000`（`api_doc_tx/code_lines_check.txt:5`）

### #5 出图耗时区间
- **原文**：「真实反代实测单张 6–16 秒」
- **改为**：「6.0–16.4 秒（最快 6.0s：`auth_matrix.txt:3`；最慢 16.4s：`field_matrix.txt:5`）」
- **依据**：`work/api_wire/auth_matrix.txt:3`（6.0s）、`work/api_wire/field_matrix.txt:5`（`cfg_scale5` 16.4s）

### #6 `promptGuidanceRescale > 1` 的响应体
- **原文**：给出完整 JSON `{"success":false,"error":"Invalid parameters","message":"promptGuidanceRescale不能大于1",…}` 并标「实测字面」
- **改为**：只保留客户端字面 `nai错误响应：promptGuidanceRescale不能大于1`，并注明「完整 JSON 外壳未留抓包，标未确认」
- **依据**：`work/key_api_tx/live_matrix.json:7` = `"stderr": "错误: nai错误响应：promptGuidanceRescale不能大于1"`（无 JSON 外壳）

### #7 §4.2 标题「逐字节」
- **原文**：`### 4.2 请求体（插件原生 13 字段，实测抓包逐字节）`
- **改为**：`### 4.2 请求体（插件原生 13 字段，字段名/顺序/类型与抓包一致；promptGuidanceRescale 值见下注）`
- **依据**：`work/api_wire/wire.txt:11` 该字段值为 `5`，与文档示例 `0.28` 不同（`api_doc_tx/wire_line11.txt:4`、`api_doc_tx/proxy_types.txt:8`）

### #8 §4.2 示例与抓包的值差
- **原文**：示例 `"promptGuidanceRescale": 0.28` 无任何说明
- **改为**：示例后加注——抓包原件同一位置是 `5`（其余 12 字段名/顺序/类型完全一致），`0.28` 是修正后的默认值，写真机必须用 `0.28`
- **依据**：`work/api_wire/wire.txt:11`；`work/key_api_tx/VERIFICATION.txt:124-126`（5 → 400，0.28 → 200）

### #9 `seed` 默认串地址
- **原文**：`| seed | string | [nai3参数] 默认种子，-1 随机 | 0x102B832C |`
- **改为**：`| seed | string | … | 键 0x102B832C；默认串 -1 在 0x102B8DD0（读点 0x100A43F8） |`
- **依据**：`work/api_doc_tx/final_va_check.txt:12` = `0x102B8DD0 | -1`；读点 `0x100A43F8` 见 `ini_defaults2.txt`

### #10 §4.4 表的口径
- **原文**：`### 4.4 该平台的额外字段与硬约束（实测）` + 「平台前端 bundle 里还有一套官方文档字段名，服务端同时兼容」
- **改为**：`### 4.4 …（平台文档 + 矩阵实测）`，并明确「下表是**平台文档口径**（`docs_strings.txt`），不是 DLL 出站字段；DLL 出站字段见 §4.2」
- **依据**：`work/api_wire/docs_strings.txt:19-62` 是平台前端文案；`work/api_wire/wire.json` 的 actual 出站字段无 `prompt`/`cfg_scale`/`k_euler_ancestral`

### #11 `width` 越界「实测 2000」
- **原文**：「实测传 `2000` 仍返回 200（服务端未强校验），但文档声明越界应拒」
- **改为**：「矩阵用例 `width_out` 返回 200（`auth_matrix.txt:4`）；该用例记录里没有留下具体传值，是否真的越界、服务端是否强校验均**未确认**」
- **依据**：`work/api_wire/auth_matrix.txt:4` 只有用例名 + 200 + 6.3s

### #12 `steps` 边界「实测 60」
- **原文**：「实测 `60` → 400」
- **改为**：「矩阵用例 `steps60` → 400 `steps不能大于50`（记录里只有用例名，边界外的实际传值未单独记录）」
- **依据**：`work/api_wire/field_matrix.txt:8`

### #13 「型号不校验」
- **原文**：「**型号不校验**：实测传 `model: "not-a-model"` 仍返回 200 并出图（平台按默认模型兜底）」
- **改为**：「**型号字段服务端不报错**：矩阵用例 `bad_model` 返回 200 并出图（`auth_matrix.txt:3`）。记录里只有用例名、没有具体传值，所以「传的是非型号名」与「按默认模型兜底」都属**未确认**」
- **依据**：`work/api_wire/auth_matrix.txt:3`

### #14 §4.5 示例字段数
- **原文**：curl/PowerShell 示例直接给出（实际 12 字段），未说明与 §4.2 的 13 字段差一个
- **改为**：示例后加注——两例都只写 12 个字段，比 §4.2 少 `negativePrompt`；`positivePrompt`/`model` 缺省会 400；`negativePrompt` 省略后行为**未验证**
- **依据**：`work/api_doc_tx/proxy_types.txt:1-13`（13 键含 `negativePrompt`）；`work/api_wire/probe_endpoints.txt:1-2`（缺 `positivePrompt`/`model` 均 400）

### #15 §5.1 `parameters` 键数缺失
- **原文**：§5.1 示例用 `…` 省略，未说明总键数与被省略的键
- **改为**：加注「抓包原件 `parameters` 共 **32 个键**（`wire_detail.txt:17` 的 `PARAMKEYS`）」，并列出被省略的 10 个键及其值
- **依据**：`work/api_doc_tx/wire_final_check2.txt:2`（params=32）、`wire_detail.txt:17`、`wire.json` `official_t2i.body`

### #16 §5.2 底图来源「三种」
- **原文**：「底图来源支持本地路径、http(s) URL、`base64://` 三种（`client.go` 的 `loadImageAsDataURL`）」
- **改为**：「仓库里只有 `main.go` 的 `readImage`（`main.go:451-459`）…**仅支持本地路径**；`base64://` 前缀只在 `FetchImageBytes`（`comfy_handle.go:44-46`）和 `ImageSeg`（`onebot.go:161-164`）出现…`loadImageAsDataURL` 在该仓库中**不存在**，『支持 http(s) URL』亦**无证据**」
- **依据**：`main.go:451-459`（`main_readImage.txt:13-21`）；`b64_sites.txt:1-10`；全仓库 grep `loadImageAsDataURL` 无结果

### #17 §5.4 augment 请求体字段数
- **原文**：`{"req_type":"emotion","image":"…","prompt":"happy"}`（3 字段）
- **改为**：`{"req_type":"emotion","image":"…","width":1024,"height":1024,"prompt":"happy","defry":0}`（6 字段），标题补「抓包是 6 字段，不是 3 个」
- **依据**：`work/api_doc_tx/official_body_dump.txt:36`（keys=6：`req_type,image,width,height,prompt,defry`）

### #18 `req_type` 取值个数
- **原文**：7 个取值，含 `bg-removal`（去背景）、`superresolution`（超分）
- **改为**：5 个（`emotion`/`declutter`/`colorize`/`sketch`/`lineart`），并加注去背景/超分不是本端点 req_type，走 §6 两个独立端点
- **依据**：`work/api_doc_tx/augment_probe.txt:1-4,9`（5 个命中）、`:5-7`（`bg-removal`/`bg_removal`/`superresolution` 全 `NOT FOUND`）

### #19 §6.1 baidu 分支字段名
- **原文**：`{"type":"baidu","img":"https://…/x.png"}`
- **改为**：`{"type":"baidu","imgUrl":"https://…/x.png"}`，并加注字段名是 `imgUrl`、不是 `img`
- **依据**：`work/api_doc_tx/va_strings.txt:45` = `0x102B8E74 imgUrl`；`xianyu-nai/nai/audit.go:52` = `Img string \`json:"imgUrl,omitempty"\``

### #20 §6.2 multipart 头
- **原文**：「**boundary 写死 `WebKitFormBoundary`**（`0x103D0EBC`）」
- **改为**：区分「整条头字面在 `0x102B8D3C`」与「分界符 `--WebKitFormBoundary` 在 `0x103D0EBC`」，并补 `file`（`0x102B8D34`）、`upload-image` 键（`0x102B8D24`）
- **依据**：`work/api_doc_tx/final_va_check.txt:9-11,39`

### #21 §6.2 兜底路径归属
- **原文**：「图片直传：`upload-image` 配置项，或兜底 `{资源地址}/plug/AI/upload-image`」（未区分 DLL 与 Go 重制）
- **改为**：加注「兜底路径 `/plug/AI/upload-image` 只存在于 Go 侧重制（`nai/publish.go:96`），DLL 里**没有**这个路径字面」
- **依据**：`work/api_doc_tx/endpoint_final.txt:8` = `NOT FOUND`；`work/api_doc_tx/addr_deep.txt:38`；`xianyu-nai/nai/publish.go:96`

### #22 §7.1 实测输出来源
- **原文**：「实测输出：」+ 两行
- **改为**：「实测输出（`work/key_api_tx/live_matrix.json` MODIFIED 段，`sec=9.4`）：」
- **依据**：`work/key_api_tx/live_matrix.json` 第 2 个元素的 `sec`/`stdout`

### #23 §7.2 OneBot 图片字节数
- **原文**：「图片 641,057 字节」（单值，与另一份 681,357 的记录冲突）
- **改为**：并列两次运行——`641,057`（`onebot_matrix.json`，用例提示词 `1girl, solo`）与 `681,357`（`onebot_e2e_result.json`，脚本 `onebot_e2e.py` 提示词为 `1girl, solo, upper body, white background`），并说明同宽不同次生成
- **依据**：`VERIFICATION.txt:127`、`work/api_wire/onebot_e2e_result.json`、`audit_onebot_check.txt:42-44`

### #24 §9 表「200」
- **原文**：6 个假服务器用例全记「200」
- **改为**：记 `exit=0` + 实际 stdout（`seed=424242`/`777001`/`777001`/`777002`），状态码另注来源
- **依据**：`work/api_doc_tx/wire_final_check.txt:8-13`；`wire.json` 各元素无 status 字段

### #25 §9 假服务器状态码推导
- **原文**：无说明（把 200 当实测字面）
- **改为**：表末加注「假服务器那 6 个用例的 HTTP 状态码在 `wire.txt`/`wire.json` 里没有逐条记录…`200` 由假服务器默认返回码推出（`nai_fake.py:98`）」
- **依据**：`C:\Users\Administrator\Desktop\nai\work\nai_fake.py:98` = `def reply(self, obj, code=200, ctype="application/json"):`

> #24 与 #25 是同一次修改的两半（表格本体 + 表注），此处按两条登记。

### #26 引用注释的文件名写错
- **原文**：`（nai/client.go:48-50 的注释）`（`client.go` 第 48-50 行是 `Config` 结构体的 `SendMode`/`PointsOn` 字段，不是该注释）
- **改为**：`（nai/nai_ini.go:48-50 的注释）`
- **依据**：`C:\Users\Administrator\Desktop\nai\xianyu-nai\nai\nai_ini.go:48-50` 逐行 = `// [重制版] 节是新增的：原插件里 nai3api / naiofficialapi / nai3_plugin-api /`、`// 资源地址 / upload-image 由授权服务器下发（见 0x1005E4B7 取 nai3api），`、`// 重制版没有路径通过这里由用户手写并持久化`

### #27 §3.3 尾部状态码释义
- **原文**：`平台自带文档声明的状态码：200 成功、400 参数错误、401 令牌无效或未提供、429 请求过于频繁、500 服务器内部错误、503 所有上游不可用。`
- **改为**：`200 请求成功`、`503 服务不可用（所有API接口都不可用）`，并补出处 `work/api_wire/docs_strings.txt:67-80`
- **依据**：`C:\Users\Administrator\Desktop\nai\work\api_wire\docs_strings.txt:69-80` 逐行 = `200` / `请求成功` / `400` / `参数错误` / `401` / `令牌无效或未提供` / `429` / `请求过于频繁` / `500` / `服务器内部错误` / `503` / `服务不可用（所有API接口都不可用）`

### 仅补证据、未改断言（2 处，计入「一致」）

| 位置 | 补充内容 | 依据 |
|---|---|---|
| §1 `pst-` 段 | 补 `docs_strings.txt:17-18` 出处 | `docs_strings.txt:17-18` |
| §1 实测段 | 补 `work/api_doc_tx/live_r1.txt`（原始响应体落盘） | 文件内容与文中 JSON 一致 |

> 另有 7 条属于「补证据型修正」——原文断言不算错，但缺数量/地址/来源，已并入上表 #9（`seed` 默认串 VA）、#15（`parameters` 32 键）、#20（multipart 整头 VA）、#21（兜底路径归属）、#22（`sec=9.4` 来源）、#23（两次 OneBot 运行区分）、#24（假服务器 `exit=0`）。

## 四、仍「未确认」清单（10 条）

| 编号 | 事项 | 为什么未确认 | 补证方式 |
|---|---|---|---|
| U1 | `image.novelai.net:443` 在本机是否可达 | 工程内无该连接测试的字面输出（原先写的 `WinError 10060` 无出处） | 对 `image.novelai.net:443` 做一次 TCP 连接并留 stdout/stderr + exit |
| U2 | `promptGuidanceRescale > 1` 的完整 JSON 错误外壳 | 只有客户端串，无响应体抓包 | 抓 400 响应原文 |
| U3 | `width_out` 用例的具体传值、是否真的越界、服务端是否强校验 | `auth_matrix.txt:4` 只有用例名 + 200 | 重跑该用例并记录请求体 |
| U4 | `bad_model` 用例的具体传值、是否按默认模型兜底 | `auth_matrix.txt:3` 只有用例名 + 200 | 同上 |
| U5 | `height` 越界行为 | 未单独实测 | 传 `height=2000` 打一次 |
| U6 | `/ai/augment-image` 其余 4 个 `req_type`（`declutter`/`colorize`/`sketch`/`lineart`）的实际行为 | DLL 有字面量，但只抓过 `emotion` | 每个各打一次并抓包 |
| U7 | `nai3_plugin-api` 下除 `nsfw_check` 外的 6 个端点（超分/去背景/tagger/image-metadata/tag-translate/youdao-translate）的请求体与响应 | 只有路径字面量在 DLL 中命中 | 拿有效 base 逐个打并抓包 |
| U8 | `429`（请求过于频繁）与 `503`（所有上游不可用）的实际触发与响应体 | 平台文档声明，未实测 | 压测或断上游触发 |
| U9 | 省略 `negativePrompt` 后服务端行为 | 未单独测；代码里该字段无 `omitempty`，但服务端是否必需未知 | 只发 `model`+`positivePrompt` 试一次 |
| U11 | 假服务器 6 用例的 HTTP 状态码字面值 | `wire.txt`/`wire.json` 只记 method/path/headers/body_len/exit | 给假服务器加状态码记录后重跑 |

> 原 U10（§2 反汇编块 4 条未单独导出）已**补证关闭**：`work/api_doc_tx/dis_mode.txt:19-27` 已完整导出 `0x1005E3E0`–`0x1005E3FE` 的 8 条指令，与文档代码块逐条一致，因此 #16 由「部分一致」改判「一致」。编号保留空位以便追溯。

> 另有 1 条叙述性内容不需证据：§10 关于 `aiurl.php` 被写坏的自述（作者行为记录，非技术断言）。

## 五、自相矛盾与格式检查

| 检查项 | 结果 |
|---|---|
| 同字段两处说法不同 | 3 处，均已修：#7/#8（§4.2 写 `0.28`、抓包 `5`）、#14（§4.2 说 13 字段、§4.5 示例 12 字段）、#23（641,057 与 681,357 两处字节数，已改为并列说明） |
| 示例与字段表冲突 | 修后无。#14 已显式说明示例少一个字段 |
| 代码块未闭合 | 无。52 个围栏 = 26 组，偶数且全部成对 |
| 表格行缺尾 `\|` | 无 |
| 前后引用的行号是否指向真实代码 | 抽查 13 处（`client.go:129/220/259/389-407/410-447/458-460`、`main.go:451-459`、`audit.go:74-82`、`pipeline.go:139/222/250`、`publish.go:96`、`comfy_handle.go:44-46`、`onebot.go:161-164`、`nai_ini.go:48-50`）全部命中；错处 `client.go:128` → `129`、`client.go:48-50` → `nai_ini.go:48-50` 已修 |
| 证据文件名是否真实存在、行号是否越界 | AUDIT.md 引用的 44 个证据文件全部存在，`MISSING_COUNT 0`、`OUT_OF_RANGE 0`（`cite_files_check.txt`）；修正过程中发现并改掉 1 处越界引用（`addr_deep.txt` 第 147 行越界 → 改为 `:46`） |
| 文档编码 | UTF-8 无 BOM；换行全为 LF（493 个）、CRLF 0 |

## 六、复核方法（可重跑）

1. **VA 取串**：`python` 读 `work/unpacked.dll`，对文档内全部 `0x10xxxxxx` 逐条 `seek(VA-0x10000000)` 读到 `\x00`，utf-8 → gbk → latin-1 回退解码，输出 `work/api_doc_tx/final_va_check.txt`（37 条）。
2. **RVA 取串**：`0x2Bxxxx` 同法取偏移即 RVA（本样本 `.text`/`.rdata`/`.data` 段 delta=0），输出 `endpoint_literals.txt` / `endpoint_literals2.txt`。
3. **出站请求复核**：`json.load(work/api_wire/wire.json)` 逐用例打印 method/path/Content-Length/exit/stdout 与 body 键集合，输出 `wire_final_check.txt` / `wire_final_check2.txt` / `official_body_dump.txt` / `proxy_types.txt` / `headers_dump.txt`。
4. **状态码与文案**：`field_matrix.txt`（8 行）、`auth_matrix.txt`（5 行）、`probe_endpoints.txt`、`probe_auth_endpoints.txt`、`live_matrix.json` 逐行取字面。
5. **ini 节名/键名与默认值**：`ini_sites.txt`、`ini_defaults2.txt`、`nai_ini.go:56-58`。
6. **鉴黄判定**：`code_audit3.txt`（`0x1002E4CB`–`0x1002E4F1` 区间反汇编）+ `addr_deep.txt`（`0x102B7FA8` 的 double = `-1e-07`）。
7. **模式判定与 URL 拼接**：`dis_mode.txt`（`0x1005E3E0`–`0x1005E3FE`、`0x100637C5`、`0x1005E4F3`）。
8. **文档自身格式**：统计代码围栏数、表格行尾、字节数、行数、LF/CRLF、BOM、sha256（`final_fmt_check.py` → `final_fmt.txt`）。
9. **三态复跑**：`final_tristate.py` 在独立临时目录重建 BASELINE / MODIFIED / ROLLBACK 三份副本，各自跑 `check_corrections.py`，ROLLBACK 用 `ROLLBACK.ps1` 还原后重跑同一命令，输出 `VERIFICATION_final.txt`。

产物目录：`C:\Users\Administrator\Desktop\nai\work\api_doc_tx\`（`final_va_check.txt`、`endpoint_literals*.txt`、`endpoint_final.txt`、`wire_final_check*.txt`、`official_body_dump.txt`、`proxy_types.txt`、`headers_dump.txt`、`code_lines_check.txt`、`write_offset.txt`、`b64_sites.txt`、`cite_check.txt`、`FINAL.diff`）。

收尾复跑产物（均为字面输出，可直接重跑）：

| 文件 | 内容 |
|---|---|
| `final_tristate.py` / `VERIFICATION_final.txt` | BASELINE `PASS=1/9` → MODIFIED `PASS=9/9` → ROLLBACK `ROLLBACK_OK 83dfb94a…` / `restored_equals_baseline=True` / 回滚后 `PASS=1/9` |
| `roles_check.py` / `ROLES_final.txt` | 四个角色路径 + 哈希 + `ROLLBACK.ps1` 内嵌载荷解压后 `(True, 20814)` |
| `sh_payload_check.py` / `ROLLBACK_sh_payload_check.txt` | `SH_DECOMPRESSED_BYTES 20814`、`DECOMPRESSED_EQ_BASELINE True` |
| `final_fmt_check.py` / `final_fmt.txt` | `FENCE_LINES 52 / CODE_BLOCKS 26 / PROBLEMS 0 / BAD_TABLE_ROWS 0 / CRLF 0 / BOM False / ADDR_UNIQ 76` |
| `audit_stats2.py` / `audit_stats2.txt` | `ROWS 155 / CONTIGUOUS True`，结论列统计 一致 131（含 #154 转义管道行）/ 不一致 19 / 未确认 3 / 无证据 1 / 叙述性 1 |

## 七、结论

- 文档中所有字符串/常量 `0x` 地址（37 个）**全部重新取串核对通过**，无一处地址写错或描述错位；另有 28 个指令地址经反汇编对位；引用的**代码行号**有 2 处错：`client.go:128` → `129`、`client.go:48-50` → `nai_ini.go:48-50`（均已修）。
- 端点路径、HTTP 方法、请求头名、请求体字段名与类型、错误码与错误文案、数值约束、ini 节名/键名，**与工程内实测证据一致**（修正后）。
- 原先标「实测」但工程内无字面记录的断言（`WinError 10060`、`width=2000`、`model=not-a-model`、`promptGuidanceRescale` 的 JSON 外壳、假服务器 200）**已全部改标为「未确认/未验证」或改记真实字面**。
- 仍有 10 项「未确认」，均为**未做过的实验**（而非已知错误），补证方式已逐条列出；另有 1 项（U10 §2 反汇编块）已在本次复核中补证关闭。

## 八、交付产物（四角色，绝对路径）

| 角色 | 绝对路径 | 校验 |
|---|---|---|
| MODIFIED_FILE（已修正的目标文档） | `C:\Users\Administrator\Desktop\nai\xianyu-nai\API文档-NAI画图接口.md` | 26589 字节 / 493 行 / sha256 `31c2990603e288965b921fb35923eebac97e115d1d050b8883e3c15bc5958144` |
| 原始基线（修正前字节） | `C:\Users\Administrator\Desktop\nai\work\api_doc_tx\MODIFIED_FILE_API-doc.md` | 20814 字节 / 477 行 / sha256 `83dfb94ad5f146b2999430b2ea367bd8b8e1db92f7c739fa6f2079bd6bfd06b7` |
| DIFF_FILE | `C:\Users\Administrator\Desktop\nai\work\api_doc_tx\FINAL.diff` | 18 hunks / +62 / −46；`replay_diff.py` → `REPLAY_EQUAL True` |
| VERIFICATION | `C:\Users\Administrator\Desktop\nai\work\api_doc_tx\VERIFICATION.txt`（+ `VERIFICATION_final.txt`） | BASELINE `1/9` → MODIFIED `9/9` → ROLLBACK 还原后 `1/9`，全部字面 stdout/exit |
| ROLLBACK（可执行） | `C:\Users\Administrator\Desktop\nai\work\api_doc_tx\ROLLBACK.sh`（POSIX sh）/ `ROLLBACK.ps1`（Windows） | 内嵌 gzip+base64 基线字节；`ROLLBACK_OK 83dfb94a…`，还原后字节与基线完全相同 |

> 本机无 `sh`/`bash`/`git`（`shutil.which` 全 None，`wsl` 未安装发行版），因此 `ROLLBACK.sh` 只做载荷解压校验（`ROLLBACK_sh_payload_check.txt`：`SH_DECOMPRESSED_BYTES 20814`、`DECOMPRESSED_EQ_BASELINE True`），真实回滚由等价的 `ROLLBACK.ps1` 实跑完成。

---

- 审计报告：`C:\Users\Administrator\Desktop\nai\work\api_doc_tx\AUDIT.md`
