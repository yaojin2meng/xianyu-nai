package nai

import "math"

// 本文件复刻插件的"图片混淆/解混淆"。
//
// 相关文案（VA）：
//
//	0x102b664c "对图片像素进行曲线重排混淆保护"
//	0x102b666c "还原被混淆处理的图片"
//	0x102bc3c0 "解混淆"      0x102bc3c8 "混淆"
//	0x102bc3d0 "的图片"      0x102bc3d8 "请提供需要"
//	0x102bc3e4 "正在进行图片"
//	0x102bc3f4 "处理失败，请确认图片格式受支持（最大1600万像素）"
//	0x102bc428 "解混淆已完成" 0x102bc438 "混淆已完成"
//	0x102bc454 "解混淆完成"   0x102bc460 "混淆完成"
//	0x102bc46c "图片已"
//
// 实现位置：fcn.10037ad8（混淆）与 fcn.1008c28c（解混淆），两者互为逆置换。
// 方向由调用方 0x1008B798/0x1008B9B3 的 arg_ch 决定：
//
//	0x1008B798  cmp dword [ebp+0xc], 0
//	0x1008B79C  je  0x1008B7AC    ; ==0 → eax = 0x102bc3c8 "混淆"
//	0x1008B7A2  mov eax, 0x102bc3c0 ; !=0 → "解混淆"
//	0x1008B99F  call fcn.1008c28c → [ebp-0x50]
//	0x1008B9AB  call fcn.10037ad8 → [ebp-0x54]
//	0x1008B9B3  cmp dword [ebp+0xc], 0
//	0x1008B9B7  je  0x1008B9DC    ; ==0（混淆）→ 用 [ebp-0x54] = fcn.10037ad8
//	                              ; !=0（解混淆）→ 用 [ebp-0x50] = fcn.1008c28c
//
// ---- 重排公式（已由反汇编逐指令确认） ----
//
// 记 order = CurveOrder(w,h)（长度 n = w*h），全部下标对 n 取模，记
//
//	K = 截断( ((sqrt(5)-1)/2) * n )
//
// 则（像素按 4 字节整体搬移）：
//
//	混淆  （fcn.10037ad8）：out[order[i+K]] = in[order[i]]
//	解混淆（fcn.1008c28c）：out[order[i]]   = in[order[i+K]]
//
// 混淆侧循环体（0x100381CC-0x100382AB）：
//
//	[ebp-0x84] = &A[order[j]]      (A = [ebp-0x2c]，输出数组)
//	[ebp-0x9c] = &P[order[i-1]]    (P = [ebp-0x28]，输入像素副本)
//	mov eax,[ebp-0x9c] ; mov ebx,[ebp-0x84] ; mov [ebx],eax
//
// 解混淆侧循环体（0x1008CA49-0x1008CA5F）为镜像：
//
//	[ebp-0x4c] = &A[order[i-1]] ；[ebp-0x9c] = &P[order[j]]
//	mov eax,[ebp-0x9c] ; mov ebx,[ebp-0x4c] ; mov [ebx],eax
//
// 其中 j = (i-1+K) mod n：
//
//	0x1008C6C2-0x1008C6E8  K = 截断( ((sqrt(5)-1)/2) * n )
//	  0x1008C694 mov ebx,0x101c6e30      ; fsqrt 叶子
//	  0x1008C6AA fsub [0x102b5a7c]=1.0
//	  0x1008C6B6 fdiv [0x102b5a74]=2.0
//	  0x1008C6C8 fmul n
//	  0x1008C6DE mov ebx,0x101c7860 / 0x101C7970(fld qword)/0x10280BF8
//	  0x10280BF8 用 fistp + 控制字 or ah,0xc（RC=11 向零截断）
//	  0x1008C6EB mov [ebp-0x24],eax      ; K
//	  0x1008C5xx … 0x1008C7xx 用 fmod(0x101c6810) 取 (i-1+K) mod n
//
// 像素为 4 字节（BGRA）：数组每项 4 字节（0x1008C629 shl ecx,2），
// 拷贝长度 trunc(n*4.0)（0x8C557 / 0x8C810，0x102b6b3c=4.0），
// 且校验新图行跨距 == w*4（0x8C554 的 fmul qword [0x102b6b3c]）。
//
// ---- 已确认（来自反汇编） ----
//  1. 像素上限 0x1000000 = 16,777,216，超出直接失败：
//     0x1008C477  cmp dword [ebp-0x14], 0x1000000 / jle 继续
//  2. 索引数组每项 4 字节，长度为 w*h：
//     0x1008C629  shl ecx, 2 / add ecx, 8
//  3. 像素下标公式 X + Y*W：
//     0x10038637  fild [ebp+0xc] (Y) * fild [ebp+0x20] (W) + fild [ebp+8] (X) -> 取整
//  4. 索引由 fcn.10038428(w,h,&out) 生成；其内部 fcn.100384a8 是 9 参数递归
//     （ret 0x24）。
//  5. 全链无随机数调用，算法确定性。
//
// ---- 已实测确认（用 32 位宿主直接调用 unpacked.dll 的 0x38428 取真值） ----
// 递归主体 fcn.100384a8 的参数为
//
//	(x, y, dx1, dy1, dx2, dy2, stride, outArray, &counter)
//
// 记 sgn() 为符号函数（0x10038d20），f() = floor(x/2)（0x10038d6c，
// 对负数做"截断后再减 1"的修正，实测 F(-3)=-2、F(-1)=-1、F(3)=1），
// 记 A=|dx1+dy1|、B=|dx2+dy2|（0x101c6be0 为取绝对值，见 0x3850C/0x3854F）。
// 于是：
//
//	B==1 : 沿 (dx1,dy1) 走 A 步，每步记录 x+y*stride，坐标按 sgn 前进
//	A==1 : 沿 (dx2,dy2) 走 B 步
//	否则 :  fx1,fy1,fx2,fy2 = f(dx1),f(dy1),f(dx2),f(dy2)
//	        若 2*A-3*B > 1e-7（0x102b5a6c=1e-7）：
//	           若 |fx1+fy1| 为奇数且 A>2 则 fx1+=sgn(dx1), fy1+=sgn(dy1)
//	           walk(x,y,fx1,fy1,dx2,dy2)
//	           walk(x+fx1,y+fy1,dx1-fx1,dy1-fy1,dx2,dy2)
//	        否则：
//	           若 |fx2+fy2| 为奇数且 B>2 则 fx2+=sgn(dx2), fy2+=sgn(dy2)
//	           walk(x,y,fx2,fy2,fx1,fy1)
//	           walk(x+fx2,y+fy2,dx1,dy1,dx2-fx2,dy2-fy2)
//	           walk(x+dx1-sgn(dx1)+fx2-sgn(dx2),
//	                y+dy1-sgn(dy1)+fy2-sgn(dy2),
//	                -fx2,-fy2, fx1-dx1, fy1-dy1)
//
// 外层包装 fcn.10038428(w,h,&out) 的入口参数（w=[ebp+8], h=[ebp+0xc]）：
//
//	w < h ： walk(0,0, 0,h, w,0, stride=w)
//	w >= h： walk(0,0, w,0, 0,h, stride=w)
//
// 实测：本实现与插件对 2x1..4096x4096 共 40 余种尺寸（含 832x1216、
// 1216x832、1024x1024、4096x4096 全 16,777,216 像素）逐元素完全一致。

// MaxConfusePixels 是插件写死的像素上限 0x1000000。
const MaxConfusePixels = 0x1000000

// ImageError 是图片处理类错误。
type ImageError struct{ Msg string }

func (e *ImageError) Error() string { return e.Msg }

// ErrTooLarge 对应 0x102bc3f4 的原文案。
var ErrTooLarge = &ImageError{Msg: "处理失败，请确认图片格式受支持（最大1600万像素）"}

// ErrInvalidImage 对应 0x102b7ee4 "无效图片"。
var ErrInvalidImage = &ImageError{Msg: "无效图片"}

// CurveOrder 复刻 fcn.10038428：返回长度 w*h 的像素遍历序，
// 值为 X + Y*W（W 为 stride，即图片宽度）。
func CurveOrder(w, h int) []int {
	out := make([]int, 0, w*h)
	if w <= 0 || h <= 0 {
		return out
	}
	if w < h {
		walkCurve(0, 0, 0, h, w, 0, w, &out)
	} else {
		walkCurve(0, 0, w, 0, 0, h, w, &out)
	}
	return out
}

// halfFloor 复刻 0x10038D6C：floor(x/2)。
func halfFloor(x int) int {
	if x >= 0 {
		return x >> 1
	}
	return -(((-x) + 1) >> 1)
}

// walkCurve 复刻 fcn.100384a8。
func walkCurve(x, y, dx1, dy1, dx2, dy2, stride int, out *[]int) {
	a := absInt(dx1 + dy1)
	b := absInt(dx2 + dy2)

	switch {
	case b == 1:
		sx, sy := signInt(dx1), signInt(dy1)
		for i := 0; i < a; i++ {
			*out = append(*out, x+y*stride)
			x += sx
			y += sy
		}

	case a == 1:
		sx, sy := signInt(dx2), signInt(dy2)
		for i := 0; i < b; i++ {
			*out = append(*out, x+y*stride)
			x += sx
			y += sy
		}

	default:
		fx1, fy1 := halfFloor(dx1), halfFloor(dy1)
		fx2, fy2 := halfFloor(dx2), halfFloor(dy2)
		if 2*a-3*b > 0 { // 2*A-3*B > 1e-7，整数即 >0
			if absInt(fx1+fy1)%2 != 0 && a > 2 {
				fx1 += signInt(dx1)
				fy1 += signInt(dy1)
			}
			walkCurve(x, y, fx1, fy1, dx2, dy2, stride, out)
			walkCurve(x+fx1, y+fy1, dx1-fx1, dy1-fy1, dx2, dy2, stride, out)
			return
		}
		if absInt(fx2+fy2)%2 != 0 && b > 2 {
			fx2 += signInt(dx2)
			fy2 += signInt(dy2)
		}
		walkCurve(x, y, fx2, fy2, fx1, fy1, stride, out)
		walkCurve(x+fx2, y+fy2, dx1, dy1, dx2-fx2, dy2-fy2, stride, out)
		walkCurve(x+dx1-signInt(dx1)+fx2-signInt(dx2),
			y+dy1-signInt(dy1)+fy2-signInt(dy2),
			-fx2, -fy2, fx1-dx1, fy1-dy1, stride, out)
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func signInt(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	default:
		return 0
	}
}

// ConfuseShift 复刻插件两次重排使用的曲线位移量
//
//	K = trunc( ((sqrt(5)-1)/2) * n )
//
// 见 0x1008C680-0x1008C6EB（解混淆）与 0x10037EE0-0x10037F37（混淆）：
// fsqrt(5) → 减 1.0 → 除 2.0 → 乘 n → 取整（0x10280BF8 用 fistp，
// 控制字 or ah,0xc，即 RC=11 向零截断）。
func ConfuseShift(n int) int {
	if n <= 0 {
		return 0
	}
	return int(((math.Sqrt(5) - 1) / 2) * float64(n))
}

// Confuse 复刻插件的图片像素曲线重排。
//
// pixels 为紧密排列的 4 字节像素（插件按 4 字节步长整体搬移，见文件头）。
// dir=true 为混淆（fcn.10037ad8），false 为解混淆（fcn.1008c28c）。
//
//	混淆  ：out[order[i+K]] = in[order[i]]
//	解混淆：out[order[i]]   = in[order[i+K]]
func Confuse(pixels []byte, w, h int, dir bool) ([]byte, error) {
	total := w * h
	if w <= 0 || h <= 0 || total > MaxConfusePixels {
		return nil, ErrTooLarge
	}
	if len(pixels) < total*4 {
		return nil, ErrInvalidImage
	}
	order := CurveOrder(w, h)
	if len(order) != total {
		return nil, ErrInvalidImage
	}
	k := ConfuseShift(total)
	out := make([]byte, total*4)
	for i := 0; i < total; i++ {
		j := (i + k) % total
		if dir {
			copy(out[order[j]*4:order[j]*4+4], pixels[order[i]*4:order[i]*4+4])
		} else {
			copy(out[order[i]*4:order[i]*4+4], pixels[order[j]*4:order[j]*4+4])
		}
	}
	return out, nil
}

// ConfuseName 复刻 0x1008B7B4 起的"请提供需要{op}的图片"。
func ConfuseName(dir bool) string {
	return "请提供需要" + confuseOp(dir) + "的图片"
}

// ConfuseProgress 复刻 "正在进行图片" + op（0x102bc3e4）。
func ConfuseProgress(dir bool) string {
	return "正在进行图片" + confuseOp(dir)
}

// ConfuseDone 复刻完成文案：进度句与结束句。
func ConfuseDone(dir bool) (progress, done string) {
	return "图片已" + confuseOp(dir) + "已完成", "图片已" + confuseOp(dir) + "完成"
}

func confuseOp(dir bool) string {
	if dir {
		return "混淆"
	}
	return "解混淆"
}
