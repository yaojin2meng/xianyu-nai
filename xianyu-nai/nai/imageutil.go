package nai

import (
	"image"
	"image/png"
	"io"
)

// imageutil.go 只放 4 字节像素与 image.Image 的互转，供 CLI 的混淆命令使用。
// 插件按 4 字节（BGRA）整体搬移像素，这里的 4 字节排列仅为有序载体，
// 通道次序不影响重排结果。

// RGBABytesToImage 把紧密 4 字节 RGBA 字节还原成 image.Image。
func RGBABytesToImage(rgba []byte, w, h int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		src := y * w * 4
		dstOff := dst.PixOffset(0, y)
		copy(dst.Pix[dstOff:dstOff+w*4], rgba[src:src+w*4])
	}
	return dst
}

// EncodePNG 写 PNG。
func EncodePNG(w io.Writer, img image.Image) error {
	return png.Encode(w, img)
}
