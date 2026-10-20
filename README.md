# xianyu-nai

`咸鱼ai画图.dll` 的 Go 重制版。原插件是 32 位易语言 DLL（UPX 5.2.1 加壳），本项目依据静态反汇编 + 真机抓包还原其三条出图通道（NAI / SD / ComfyUI）与整套配置体系，产出单个纯标准库 Go 可执行文件，用 OneBot v11 与 QQ 框架通信。

- 原 DLL：1,803,264 字节 → 脱壳后 5,390,336 字节，ImageBase `0x10000000`
- 产物：`xianyu-nai.exe`（Go 1.22，`CGO_ENABLED=0`，无第三方依赖）
- 形态：EXE + OneBot v11。不复刻 `edgeview.dll` / WebView2，配置面板改为本地 HTTP + 内嵌 HTML（原插件本身也是 WebView2 加载网页）

## 仓库结构

```
xianyu-nai/            Go 模块（module xianyu-nai）
  go.mod
  main.go              命令行入口：出图 / -dump / -ui / -onebot
  nai/
    client.go          NAI 两通道（官方 / 反代）请求构造与解包
    pipeline.go        出图流水线（含 image_url 直链下载回填）
    nai_ini.go         [nai3设置] [nai3参数] [nai3指令] 等配置节
    ui.go              配置面板 schema（18 分页，单一数据源驱动）
    onebot.go          OneBot v11 适配层
    sd.go / sd_*.go    Stable Diffusion (WebUI) 通道
    comfy*.go          ComfyUI 通道
    audit.go           图像审核（api4ai / baidu / baiduai）
    publish.go         图库上传 / 直链
    prompt.go / gate.go 提示词处理、点数与频次闸门
    *_test.go          104 个用例
  API文档-NAI画图接口.md   NAI 画图接口完整文档（每条结论带 RVA 或实测标注）
  还原度清单.md            与 DLL 的功能逐项对照
  部署说明.md              部署与使用

config/
  咸鱼ai画图.example.ini   配置样例（key 已脱敏为占位串）

evidence/
  README.md                证据索引
  DLL-常量与RVA.md          关键常量 VA/RVA 对照
  work/allini.txt          全量「读配置项」调用点（324 处）
  work/defaults.txt        各配置项默认值
  work/api_wire/           假服务器抓包 + 字段矩阵 + 鉴权矩阵
  work/key_api_tx/         默认值修正 + image_url 回填 的四角色验证
  work/api_doc_tx/         接口文档审计、回归与四角色验证
  tools/                   事务重建脚本
```

## 编译

```powershell
cd xianyu-nai
go build -p 1 -o xianyu-nai.exe .
go test ./nai/
go vet ./...
```

纯标准库，不需要联网拉依赖。`-p 1` 是在 4 GB 内存的机器上避免并行编译 OOM；内存充足可去掉。

## 快速开始

```powershell
# 单张出图（反代通道）
.\xianyu-nai.exe -prompt "1girl, solo" `
  -key "nai3-..." -proxy "https://nai.xianyuw.cn" `
  -steps 28 -width 832 -height 1216

# 配置面板（浏览器打开 http://127.0.0.1:8088/）
.\xianyu-nai.exe -ui -ui-addr 127.0.0.1:8088 -data .\run

# OneBot 机器人（正向 WebSocket / HTTP 上报）
.\xianyu-nai.exe -onebot -data .\run `
  -ob-api http://127.0.0.1:3000 `
  -ob-listen 0.0.0.0:8080
```

配置样例见 `config/咸鱼ai画图.example.ini`，把 `key` 换成自己的即可。

## key 与通道

- `pst-` 前缀 = NovelAI 官方持久令牌，走 `https://image.novelai.net`
- `nai3-` 前缀 = 第三方反代（中转）平台的 key，走反代端点

插件按 `[nai3设置] 模式` 分流：**只有字面等于 `官方`** 才走官方通道，默认值 `中转`，其余一切取值（含空串）都走反代。这个判定在 DLL 里是一条 `strcmp` + `sete`，见 `evidence/DLL-常量与RVA.md` 第四节。

两条通道的请求体异构，不能互换：

| 字段 | 官方 | 反代 |
|---|---|---|
| `width` / `height` / `steps` | 字符串 | 数字 |
| `seed` | 数字 | 字符串 |
| `promptGuidanceRescale` | ≤ 1 | ≤ 1 |

`promptGuidanceRescale` 超过 1 会被服务端以 400 拒绝（`promptGuidanceRescale不能大于1`）。插件默认 `cfg=0.28`，本项目对齐该默认值。

## 已知限制

- 原 DLL 是 i386 + 易语言，DLL 形态无法重制：Go 的 `-buildmode=c-shared` 强制外部链接，`CGO_ENABLED=0` 直接报错，`CGO_ENABLED=1` 需要 C 工具链。故交付 EXE。
- 官方通道未做公网实连验证，`image.novelai.net` 连通性未确认（抓包里的 key 是 `pst-xxxxxxxx` 占位符）。
- 配置面板下拉框的「反代」选项在 DLL 中无独立字面证据（DLL 只有 `中转` / `官方`），与 `中转` 行为等价。
- ini 统一按 UTF-8 读写，原插件为 ANSI/GBK，GBK 互操作未实现。

## 免责

本仓库为逆向学习与接口文档用途。二进制分析对象为自有样本，实测使用自有账号凭据；仓库内不包含任何真实密钥（已统一替换为 `nai3-REDACTED-REPLACE-WITH-YOUR-OWN-KEY`），也不分发原版 DLL 及其脱壳产物。

## License

MIT，见 `LICENSE`。
