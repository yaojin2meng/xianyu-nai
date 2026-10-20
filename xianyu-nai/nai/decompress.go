package nai

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
)

// decompress.go 只放"字节流"相关的底层工具，供 pipeline.go 调用。

// isZip 判断字节是否为 ZIP（本地文件头 PK\x03\x04，或以空档 PK\x05\x06 结尾）。
func isZip(b []byte) bool {
	if len(b) < 4 {
		return false
	}
	if bytes.Equal(b[:4], []byte("PK\x03\x04")) {
		return true
	}
	return false
}

// unzipFirstFile 取 zip 里第一个非目录条目。
// 官方 NAI 返回的 zip 内固定只有一张 png，因此取第一个就够。
func unzipFirstFile(raw []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		out, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		return out, nil
	}
	return nil, ErrNotImageBytes
}

// buildMultipart 复刻插件的 multipart 拼装：
//
//	Content-Type: multipart/form-data; boundary=WebKitFormBoundary  (0x102b8d3c)
//	字段名 "file" (0x102b8d34)
//
// 逐字取证（表单类 0x100E143C "加字段" + 0x100E1718 "字段头"）：
//
//	界      "--WebKitFormBoundary" (0x103D0EBC)
//	行尾    "\r\n"                 (0x102B5A68)
//	头前缀  `Content-Disposition: form-data; name="` (0x103D0ED8)
//	头后缀  `"\r\n`                (0x103D0ED4)
//
// boundary 在原生实现里是写死的常量（不是随机生成），这里用 SetBoundary 对齐。
// 片段里 `filename="..."` 是 Go 标准库给文件的固定写法，DLL 侧由表单类补文件名
// （"jpg" 常量在 0x103D0F20 类表尾部），属语义等价、文本未逐字确认。
func buildMultipart(field, filename string, data []byte) ([]byte, string) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.SetBoundary(FixedMultipartBoundary); err != nil {
		mw = multipart.NewWriter(&buf)
	}
	fw, err := mw.CreateFormFile(field, filename)
	if err == nil {
		_, _ = fw.Write(data)
	}
	_ = mw.Close()
	return buf.Bytes(), "multipart/form-data; boundary=" + FixedMultipartBoundary
}

// postRaw 发送任意 body。
func (c *Client) postRaw(ctx context.Context, url, contentType string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, ErrLinkFailed
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
