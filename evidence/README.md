# 逆向证据索引

本目录保存全部结论的原始证据，便于逐条复核。所有路径均相对本目录。

## DLL 二进制取证

分析对象：`咸鱼ai画图.dll`，原始 1,803,264 字节（UPX 5.2.1 加壳），脱壳后 5,390,336 字节，ImageBase `0x10000000`。
本样本 RVA == 文件偏移（各节 `VirtualAddress == PointerToRawData`），可直接 VA − 0x10000000 定位。
提取方式：`pefile` 解析节表 + `capstone` 反汇编 + 字符串扫描。

> 原版 DLL 与脱壳产物**不在仓库内**：那是第三方的商业二进制，不随仓库分发。
> 文中出现的 `unpacked.dll` 路径均指本地分析副本。

| 文件 | 内容 |
|---|---|
| `DLL-常量与RVA.md` | 关键常量 VA/RVA 对照表（端点、节名、键名、错误文案、鉴黄判定） |

## ini 配置体系

| 文件 | 内容 |
|---|---|
| `work/allini.txt` | 全量「读配置项」调用点与节名/键名（324 处） |
| `work/allsections.txt` | 全部节名清单 |
| `work/sections_str.txt` | 节名字符串表 |
| `work/defaults.txt` | 各配置项默认值 |
| `work/inaireads.txt` | 逐条读取记录 |
| `work/ui_consts.txt` | 面板相关常量 |
| `work/uischema.txt` | 面板 schema 提取结果 |
| `work/naisec.txt` | `[nai3设置]` 节实测内容 |

## NAI 接口

| 文件 | 内容 |
|---|---|
| `work/api_evidence.txt` | 端点 / 配置项 RVA 对照 |
| `work/api_urls.txt` | 全部 URL 常量 |
| `work/api_aiurl.txt` / `work/api_aiurl_probe.txt` / `work/api_aiurl_post.txt` | 授权服务器 `aiurl.php` 下发 `nai3api` 的取证 |
| `work/api_host_probe.txt` | 主机连通性探测 |
| `work/tpl_proxy.txt` / `work/tpl_after.txt` | 反代请求模板串拼接过程 |
| `work/api_wire/wire.txt` | 本地假服务器抓到的 6 组真实出站请求 |
| `work/api_wire/field_matrix.txt` | 对真实服务端的字段矩阵（字符串/数字类型差异） |
| `work/api_wire/auth_matrix.txt` | 鉴权矩阵 |
| `work/api_wire/docs_strings.txt` / `docs_component.txt` | 平台自带文档原文串 |
| `work/api_wire/platform_index.html` | 平台首页快照 |
| `work/api_wire/probe_endpoints.txt` | 端点探测结果 |
| `work/api_wire/onebot_e2e_result.json` | OneBot 真机结果 |
| `work/api_doc_tx/rva_recheck.txt` | RVA 逐条复核 |
| `work/api_doc_tx/va_strings.txt` | 关键字符串 VA 清单 |
| `work/api_doc_tx/addr_deep.txt` | 深挖地址结果（含 NOT FOUND 项） |
| `work/api_doc_tx/augment_probe.txt` | `augment-image` 探测 |

## 四角色验证

### key_api_tx —— 默认值修正 + image_url 回填

| 角色 | 文件 |
|---|---|
| MODIFIED_FILE | `work/key_api_tx/MODIFIED_FILE/`（6 个 .go 文件） |
| DIFF_FILE | `work/key_api_tx/DIFF_FILE.patch`（6 文件 / 9 hunks / +17 −28） |
| VERIFICATION | `work/key_api_tx/VERIFICATION.txt` |
| ROLLBACK | `work/key_api_tx/ROLLBACK.sh` / `ROLLBACK.ps1` |
| baseline | `work/key_api_tx/baseline/` |
| patch 重建 | `work/key_api_tx/patch_verify/` |

三态矩阵：`ui_schema_matrix.json`、`onebot_matrix.json`、`dump_matrix.txt`、`_live_literal.txt`、`stability_5x.json`、`rollback_hashes.json`、`_gates.json`、`_final_gates.json`。

结论摘要：
- `cfg` 5 → 0.28、`strength` 0.28 → 0.52（与 DLL 默认值串 `0x1039AC60` / `0x1039AB54` 对齐）
- `[nai3设置] 模式` 默认 `""` → `中转`，面板 select 取值域补 `中转`
- 反代只回 `image_url` 时，`pipeline.go` 新增 `FetchImageBytes` 回填

### api_doc_tx —— 交付文档

| 角色 | 文件 |
|---|---|
| MODIFIED_FILE | `work/api_doc_tx/MODIFIED_FILE_API-doc.md` |
| DIFF_FILE | `work/api_doc_tx/DIFF_FILE.diff`（19 hunks / +67 −52） |
| VERIFICATION | `work/api_doc_tx/VERIFICATION.txt` |
| ROLLBACK | `work/api_doc_tx/ROLLBACK.sh` / `ROLLBACK.ps1`（内嵌 gzip+base64 pristine） |
| baseline | `work/api_doc_tx/baseline_doc.md` |
| 反向重建 | `work/api_doc_tx/reconstructed_new.md` |
| 审计报告 | `work/api_doc_tx/AUDIT.md`（155 条断言逐条核对） |
| 回归全表 | `work/api_doc_tx/REGRESSION.md` |
| 三态契约输出 | `work/api_doc_tx/PROBE_matrix.txt` |
| 面板实测 | `work/api_doc_tx/ui_final_probe.txt` |
| 真机 e2e | `work/api_doc_tx/ship_e2e_result.json` |
| 契约检查器 | `work/api_doc_tx/check_corrections.py` |

审计结论：155 条断言中 131 一致、19 处不一致已就地改正、4 条降级「未确认」，合计 27 条修正。

## 复现脚本

| 文件 | 作用 |
|---|---|
| `tools/rebuild_key_tx.py` | 重建 key_api_tx 的 baseline / MODIFIED / DIFF，跑三态与回滚 |
| `tools/rebuild_doc_tx.py` | 重建文档 baseline / MODIFIED / DIFF，跑 9 条契约与回滚 |

> `rebuild_key_tx.py` 里原本写死了真实 key 用于真机调用，仓库版本已替换为
> `nai3-REDACTED-REPLACE-WITH-YOUR-OWN-KEY`。要重新跑真机部分，请替换成自己的 key。
> 其余纯静态部分（DIFF 重建、三态契约检查）不需要 key，可直接运行。
