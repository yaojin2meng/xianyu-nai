package nai

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

// 以下真值表由 32 位宿主直接调用脱壳后插件 unpacked.dll 的 fcn.10038428
// （w=arg1, h=arg2, &outArray=arg3）实测得到，为逐元素比对基准。
var curveTruth = []struct {
	w, h int
	seq  []int
}{
	{2, 1, []int{0, 1}},
	{1, 2, []int{0, 1}},
	{2, 2, []int{0, 2, 3, 1}},
	{2, 3, []int{0, 1, 3, 5, 4, 2}},
	{3, 2, []int{0, 3, 4, 5, 2, 1}},
	{3, 3, []int{0, 3, 6, 7, 8, 5, 4, 1, 2}},
	{4, 2, []int{0, 4, 5, 1, 2, 6, 7, 3}},
	{4, 3, []int{0, 1, 5, 4, 8, 9, 10, 11, 7, 6, 2, 3}},
	{4, 4, []int{0, 1, 5, 4, 8, 12, 13, 9, 10, 14, 15, 11, 7, 6, 2, 3}},
	{5, 2, []int{0, 5, 6, 1, 2, 7, 8, 9, 4, 3}},
	{5, 4, []int{0, 1, 6, 5, 10, 15, 16, 11, 12, 17, 18, 19, 14, 13, 9, 8, 7, 2, 3, 4}},
	{8, 4, []int{0, 1, 9, 8, 16, 24, 25, 17, 18, 26, 27, 19, 11, 10, 2, 3,
		4, 5, 13, 12, 20, 28, 29, 21, 22, 30, 31, 23, 15, 14, 6, 7}},
}

func eqInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCurveOrderTruth(t *testing.T) {
	for _, tc := range curveTruth {
		got := CurveOrder(tc.w, tc.h)
		if !eqInts(got, tc.seq) {
			t.Errorf("CurveOrder(%d,%d):\n got  %v\n want %v", tc.w, tc.h, got, tc.seq)
		}
	}
}

// FNV-1a 64：与宿主端逐元素哈希口径一致（uint32 小端 4 字节）。
func fnvSeq(a []int) uint64 {
	var h uint64 = 14695981039346656037
	for _, v := range a {
		u := uint32(v)
		for b := 0; b < 4; b++ {
			h ^= uint64(byte(u >> (8 * b)))
			h *= 1099511628211
		}
	}
	return h
}

// 大尺寸真值哈希，全部来自对插件本体 fcn.10038428 的实测输出。
var curveHashTruth = []struct {
	w, h int
	hash uint64
}{
	{512, 512, 0x502068e7245d7525},
	{300, 200, 0x8cd554a4b2da4525},
	{1024, 1024, 0x62a1f7b92bdf3fa5},
	{832, 1216, 0xea03234947fd32cd},
	{1216, 832, 0xa8dc6f78b2a4c1fd},
	{2000, 1500, 0xf908e282932406c1},
	{4096, 4096, 0x032f8927572df325},
}

func TestCurveOrderHashes(t *testing.T) {
	for _, tc := range curveHashTruth {
		seq := CurveOrder(tc.w, tc.h)
		if len(seq) != tc.w*tc.h {
			t.Fatalf("CurveOrder(%d,%d) len=%d want %d", tc.w, tc.h, len(seq), tc.w*tc.h)
		}
		if got := fnvSeq(seq); got != tc.hash {
			t.Errorf("CurveOrder(%d,%d) hash=%016x want=%016x", tc.w, tc.h, got, tc.hash)
		}
	}
}

// 每个尺寸都必须是 [0,w*h) 的一个双射。
func TestCurveOrderBijection(t *testing.T) {
	for _, s := range [][2]int{{1, 1}, {2, 3}, {7, 5}, {13, 9}, {32, 17}, {64, 64}, {100, 37}, {256, 129}} {
		w, h := s[0], s[1]
		seq := CurveOrder(w, h)
		if len(seq) != w*h {
			t.Fatalf("%dx%d len=%d", w, h, len(seq))
		}
		seen := make([]bool, w*h)
		for _, v := range seq {
			if v < 0 || v >= w*h {
				t.Fatalf("%dx%d out of range value %d", w, h, v)
			}
			if seen[v] {
				t.Fatalf("%dx%d duplicate %d", w, h, v)
			}
			seen[v] = true
		}
	}
}

// 13x9 真值（实测）：校验曲线在非 2 的幂、非方形下的逐元素正确性。
func TestCurveOrderTruth13x9(t *testing.T) {
	want := []int{0, 13, 14, 1, 2, 15, 16, 3, 4, 5, 18, 17, 30, 31, 44, 43, 42, 29,
		28, 41, 40, 27, 26, 39, 52, 53, 54, 67, 66, 65}
	got := CurveOrder(13, 9)
	if !eqInts(got[:len(want)], want) {
		t.Errorf("CurveOrder(13,9) 前 %d 项不符:\n got  %v\n want %v", len(want), got[:len(want)], want)
	}
}

// 整图往返：PNG 解码 → 混淆 → 解混淆 → 逐像素应与原图完全一致。
func TestConfuseImageRoundTrip(t *testing.T) {
	w, h := 37, 23
	src := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			src.SetRGBA(x, y, color.RGBA{
				R: uint8(x*7 + y),
				G: uint8(x*3 + y*5),
				B: uint8(x ^ y),
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	if err := EncodePNG(&buf, src); err != nil {
		t.Fatal(err)
	}
	img, err := DecodeImage(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	iw, ih := b.Dx(), b.Dy()
	orig := ToRGBA(img)

	enc, err := Confuse(orig, iw, ih, true)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(enc, orig) {
		t.Fatal("混淆没有改变像素")
	}
	dec, err := Confuse(enc, iw, ih, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dec, orig) {
		t.Fatal("整图混淆→解混淆未逐像素还原")
	}
	if got := RGBABytesToImage(dec, iw, ih); got.Bounds().Dx() != iw {
		t.Fatal("落盘图尺寸不符")
	}
}
