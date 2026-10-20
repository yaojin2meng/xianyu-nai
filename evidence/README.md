# 逆向证据索引

本目录保存全部结论的原始证据，便于逐条复核。所有路径均相对本目录。

## DLL 二进制取证

`work/unpacked.dll`（脱壳后 5,390,336 字节，ImageBase `0x10000000`）的反汇编与字符串常量提取脚本与输出。RVA == 文件偏移（本样本特性）。

| 文件 | 内容 |
|---|---|
| `DLL-常量与RVA.md` | 关键常量 VA/RVA 对照表（端点、节名、键名、错误文案） |

## ini 配置体系

| 文件 | 内容 |
|---|---|
| `work/allini.txt` | 全量 `读配置项` 调用点与节名/键名（324 处） |
| `work/allsections.txt` | 全部节名清单 |
| `work/defaults.txt` | 各配置项默认值 |
| `work/ui_consts.txt` | 面板相关常量 |

## NAI 接口

| 文件 | 内容 |
|---|---|
| `work/api_evidence.txt` | 端点 / 配置项 RVA 对照 |
| `work/api_wire/wire.txt` | 本地假服务器抓到的 6 组真实出站请求 |
| `work/api_wire/field_matrix.txt` | 对真实服务端的字段矩阵 |
| `work/api_wire/auth_matrix.txt` | 鉴权矩阵 |
| `work/api_wire/docs_strings.txt` | 平台自带文档原文串 |
| `work/api_wire/probe_endpoints.txt` | 端点探测结果 |
| `work/api_doc_tx/rva_recheck.txt` | RVA 逐条复核 |
| `work/api_doc_tx/va_strings.txt` | 关键字符串 VA 清单 |
| `work/api_doc_tx/addr_deep.txt` | 深挖地址结果（含 NOT FOUND 项） |

## 四角色验证

### key_api_tx —— 默认值修正 + image_url 回填

| 角色 | 文件 |
|---|---|
| MODIFIED_FILE | `work/key_api_tx/MODIFIED_FILE/`（6 个 .go 文件） |
| DIFF_FILE | `work/key_api_tx/DIFF_FILE.patch` |
| VERIFICATION | `work/key_api_tx/VERIFICATION.txt` |
| ROLLBACK | `work/key_api_tx/ROLLBACK.sh` / `ROLLBACK.ps1` |
| baseline | `work/key_api_tx/baseline/` |
| patch 重建 | `work/key_api_tx/patch_verify/` |

三态矩阵：`ui_schema_matrix.json`、`onebot_matrix.json`、`dump_matrix.txt`、`_live_literal.txt`、`stability_5x.json`、`rollback_hashes.json`。

### api_doc_tx —— 交付文档

| 角色 | 文件 |
|---|---|
| MODIFIED_FILE | `work/api_doc_tx/MODIFIED_FILE_API-doc.md` |
| DIFF_FILE | `work/api_doc_tx/DIFF_FILE.diff` |
| VERIFICATION | `work/api_doc_tx/VERIFICATION.txt` |
| ROLLBACK | `work/api_doc_tx/ROLLBACK.sh` / `ROLLBACK.ps1` |
| baseline | `work/api_doc_tx/baseline_doc.md` |
| 审计报告 | `work/api_doc_tx/AUDIT.md`（155 条断言逐条核对） |
| 回归全表 | `work/api_doc_tx/REGRESSION.md` |
| 三态契约输出 | `work/api_doc_tx/PROBE_matrix.txt` |
| 面板实测 | `work/api_doc_tx/ui_final_probe.txt` |
| 真机 e2e | `work/api_doc_tx/ship_e2e_result.json` |

## 复现脚本

| 文件 | 作用 |
|---|---|
| `work/rebuild_key_tx.py` | 重建 baseline / MODIFIED / DIFF，跑三态与回滚 |
| `work/rebuild_doc_tx.py` | 重建文档 baseline / MODIFIED / DIFF，跑 9 条契约与回滚 |

> 注意：这些脚本里出现了真实 key 字面量（用于真机调用复现）。若要重新跑，请替换为自己的 key。
