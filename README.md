# xianyu-nai

`咸鱼ai画图.dll` 的 Go 重制版。原插件是 32 位易语言 DLL（UPX 5.2.1 加壳），本项目完全依据静态反汇编 + 真机抓包还原其三条出图通道（NAI / SD / ComfyUI）与配置体系，产出单个纯标准库 Go 可执行文件，以 OneBot v11 与 QQ 框架通信。

- 原 DLL：1,803,264 → 脱壳后 5,390,336 字节，ImageBase `0x10000000`
- 产物：`xianyu-nai.exe`（Go 1.22，`CGO_ENABLED=0`，无第三方依赖）
- 形态：EXE + OneBot v11（不复刻 `edgeview.dll`/WebView2，配置面板改为本地 HTTP + 内嵌 HTML）

## 目录

```
main.go                  命令行入口：出图 / -dump / -ui / -onebot
nai/
  client.go              NAI 两通道（官方 / 反代）请求构造与解包
  pipeline.go            出图流水线（含 image_url 直链下载回填）
  nai_ini.go             [nai3设置] [nai3参数] [nai3指令] 等 18 个 ini 节
  ui.go                  配置面板 schema（18 分页，单一数据源）
  onebot.go              OneBot v11 适配层
  sd.go / comfy.go       SD(WebUI) / ComfyUI 两条通道
  audit.go               图像审核（api4ai / baidu / baiduai）
  publish.go             图库上传 / 直链
  prompt.go / gate.go    提示词处理、点数与频次闸门
config/
  咸鱼ai画图.example.ini 配置样例（key 已脱敏）
evidence/                反汇编与实测原始证据
  DLL-常量与RVA.md
  key_api_tx/            默认值修正 + image_url 回填 的四角色验证
  api_doc_tx/            接口文档审计与回归
API文档-NAI画图接口.md     NAI 画图接口完整文档（含 RVA 与实测标注）
还原度清单.md              与 DLL 的功能逐项对照
部署说明.md                部署与使用
```

## 快速开始

```powershell
# 单张出图（反代通道）
.\xianyu-nai.exe -prompt "1girl, solo" `
  -key "nai3-..." -proxy "https://nai.xianyuw.cn" `
  -steps 28 -width 832 -height 1216

# 配置面板
.\xianyu-nai.exe -ui -ui-addr 127.0.0.1:8088 -data .\run

# OneBot 机器人
.\xianyu-nai.exe -onebot -data .\run -ob-api http://127.0.0.1:3000 -ob-listen 0.0.0.0:8080
```

编译：

```powershell
go build -p 1 -o xianyu-nai.exe .
go test ./nai/
```

## key 与通道

`pst-` 前缀是 NovelAI 官方持久令牌（`https://image.novelai.net`）；`nai3-` 前缀是第三方反代（中转）平台的 key。插件按 `[nai3设置] 模式` 分流：只有字面等于 `官方` 才走官方通道，默认值 `中转` 走反代。详见 `API文档-NAI画图接口.md`。

## 已知限制

- 原 DLL 为 i386 + 易语言，DLL 形态无法重制（Go 的 `-buildmode=c-shared` 需要 C 工具链），故交付 EXE。
- 官方通道未做公网实连验证；`image.novelai.net` 连通性未确认。
- 配置面板下拉框的「反代」选项在 DLL 中无独立字面证据（DLL 只有 `中转` / `官方`），与 `中转` 行为等价。
- ini 统一按 UTF-8 读写，原插件为 ANSI/GBK，GBK 互操作未实现。

## 免责

本仓库为逆向学习与接口文档用途。二进制内容、接口、配置项均来自对自有样本的静态分析与自有账号的实测，未包含任何他人的密钥或私有数据。
