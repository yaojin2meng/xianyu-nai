# DLL 关键常量与 RVA 对照

样本：`咸鱼ai画图.dll`，原始 1,803,264 字节，UPX 5.2.1 加壳；脱壳后 5,390,336 字节。
ImageBase `0x10000000`，本样本 **RVA == 文件偏移**（各节 `VirtualAddress == PointerToRawData`），所以下文可直接按 VA − 0x10000000 定位文件偏移。

> 提取方式：`pefile` 解析节表 + `capstone` 反汇编 + 字符串扫描。
> 注意：capstone/pefile 只接受 **RVA**，传 VA 会触发访问冲突。

## 一、NAI 端点

| VA | 字面量 | 含义 |
|---|---|---|
| `0x102BA8AC` | `/api/generate_image` | 反代通道出图 |
| `0x102BA9C0` | `/ai/generate-image` | 官方通道出图 |
| `0x102BA9A4` | `/ai/augment-image` | 官方通道导演工具 |
| `0x102BB014` | `/user/subscription` | 额度查询 |
| `0x102B8DEC` | `nai3_plugin-api` | 辅助接口 base |
| `0x102BA9A0` | `/` | 官方通道拼 URL 时的尾部斜杠判断串 |

## 二、辅助接口（`nai3_plugin-api`）

`/api/nsfw_check`、`/api/superResolution_image`、`/api/remove_background`、`/api/tagger`、`/api/image-metadata`、`/api/v1/tag-translate`、`/api/v1/youdao-translate`

图库上传：
- JSON：`{资源地址}/plug/AI/v1/upload_image/upload.php`（`0x102B94F4`）
- 直传：`{资源地址}/plug/AI/upload-image`，multipart 字段 `file`（`0x102B8D34`）
- boundary 写死 `WebKitFormBoundary`（`0x102B8D3C` 是整条 Content-Type，`0x103D0EBC` 是分界符 `--WebKitFormBoundary`）

## 三、配置节名 / 键名

| VA | 字面量 |
|---|---|
| `0x102B5C14` | `nai3设置` |
| `0x102B5B78` | `nai3参数` |
| `0x102B5D6C` | `nai3指令` |
| `0x102B5C04` | `中转`（`[nai3设置] 模式` 的默认值） |
| `0x102B5C20` | `官方`（`[nai3设置] 模式` 的官方分支判据） |
| `0x102BA770` | `cfg`（写入 `promptGuidanceRescale` 的键） |
| `0x102BA768` | `scale` |
| `0x102B8334` | `steps` |
| `0x102B832C` | `默认种子` |
| `0x102B94E0` | `采样` 默认 `k_euler` |
| `0x102BA788` | `噪声` 默认 `native` |
| `0x102B8E64` | `阈值`（鉴黄） |

默认值串：

| VA | 值 | 归属 |
|---|---|---|
| `0x1039AC60` | `0.28` | `[nai3参数] cfg` |
| `0x1039AB54` | `0.52` | `[nai3参数] strength` |
| `0x1039AB5C` | `28` | `[nai3参数] steps` |
| `0x102B5AA0` | `5` | `[nai3参数] scale` |
| `0x102B7E7C` | `1024` | `[nai3参数] 宽度` / `高度` |
| `0x102B8DD0` | `-1` | `[nai3参数] 默认种子` |
| `0x102B8E60` | `0.5` | `[图像审核] 阈值` |

## 四、通道选择逻辑

`0x1005E3E0`–`0x1005E3FE`：

```
0x1005E3E0  push   0x102B5C20              ; "官方"
0x1005E3E5  push   dword ptr [ebp-0xD0]   ; 读出来的 [nai3设置] 模式
0x1005E3EB  call   0x10001C40              ; 字符串比较
0x1005E3F0  add    esp, 8
0x1005E3F3  cmp    eax, 0
0x1005E3F6  mov    eax, 0
0x1005E3FB  sete   al                     ; al = (比较结果 == 0)
0x1005E3FE  mov    dword ptr [ebp-0xD4], eax
```

结论：`模式 == "官方"` 才走官方通道，其余一切取值（含默认 `中转`、含空串）都走反代。

## 五、URL 拼接差异

- 官方：`0x100637C5` 起，先 `push 0x102BA9A0`（`"/"`）做尾部比较，判断 base 是否已以 `/` 结尾。
- 反代：`0x1005E4F3` 起，**裸拼**，直接把 base 与路径两元素拼接（`ecx=2`），不判断斜杠。

## 六、鉴黄判定

`0x1002E4CB` 起：

```
0x1002E4CB  fld    qword ptr [ebp - 0x70]      ; nsfw 分数
0x1002E4CE  fsub   qword ptr [ebp - 0x80]      ; 减去 threshold
0x1002E4D4  fcomp  qword ptr [0x102B7FA8]      ; 与 -1e-07（实测该 double）比较
0x1002E4DA  fnstsw ax
0x1002E4DC  test   ah, 1                       ; C0 = 结果"小于"
0x1002E4DF  jne    0x1002E4F1                  ; 小于 → 合规
0x1002E4E5  mov    dword ptr [ebp - 0x44], 1   ; 否则 → 违规
```

等价语义：`nsfw - threshold >= -1e-7` 判违规，即 `nsfw >= threshold` 的浮点容差版。
阈值默认串 `0.5` 在 `0x102B8E60`；失败文案 VA `0x102B8E38`。

## 七、其他常量

| VA | 字面量 |
|---|---|
| `0x102B8178` | `Authorization: Bearer ` |
| `0x103D2F1C` | `res.xianyuw.cn` |
| `0x1039B1AC` | `xianyu.xianyuw.cn` |
| `0x102BA5F8` | 反代请求模板串 |

## 八、未找到 / 未确认

| 项 | 结论 |
|---|---|
| `重制版` | NOT FOUND（该节是 Go 重制版新增） |
| `/plug/AI/upload-image` | NOT FOUND（仅存在于 Go 侧重制） |
| `bg-removal` / `superresolution` 作为 `req_type` | NOT FOUND（它们走独立端点） |
| 内嵌 HTML / Vue 前端 | NOT FOUND（`<!DOCTYPE`/`<html`/`<div`/`createApp` 命中 0，仅 `Vue` 1 处 @ `0x2a0832`） |
| `nai3api` / `naiofficialapi` / `nai3_plugin-api` 的默认值 | 不硬编码，由授权服务器 `aiurl.php` 下发 |
