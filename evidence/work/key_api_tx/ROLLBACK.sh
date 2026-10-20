#!/bin/sh
# ROLLBACK.sh — 撤销 GOAL（NAI 反代默认值修正 + image_url 下载回填），
# 把目标副本恢复成 pristine 字节。
#
# 用法:
#   ./ROLLBACK.sh /path/to/target_tree
#
# 目标树须是 xianyu-nai 仓库副本（含 nai/ 子目录）；
# pristine 字节从本脚本同级的 baseline/ 目录读取。
set -eu

TARGET="${1:-}"
if [ -z "$TARGET" ]; then
  echo "usage: $0 <target_tree>" >&2
  exit 2
fi
if [ ! -d "$TARGET/nai" ]; then
  echo "ROLLBACK: '$TARGET/nai' not found" >&2
  exit 3
fi

HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
SRC="$HERE/baseline"
if [ ! -d "$SRC/nai" ]; then
  echo "ROLLBACK: baseline/ missing next to script" >&2
  exit 4
fi

FILES="main.go nai/client.go nai/ui.go nai/client_test.go nai/pipeline.go nai/sd_cmd.go"
for f in $FILES; do
  if [ ! -f "$SRC/$f" ]; then
    echo "ROLLBACK: missing baseline file $f" >&2
    exit 5
  fi
  cp -f "$SRC/$f" "$TARGET/$f"
  echo "restored $f"
done

echo "ROLLBACK_OK"
exit 0
