package nai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// 本文件实现一个"保留对象键顺序"的 JSON 树。
//
// 为什么需要它：插件用的 E 语言 JSON 类（vtbl 0x102b66c0 那一族方法，
// 方法 +0x188 置 JSON 文本、+0xcc 按路径置值、+0x18 取文本）在
// "解析模板 -> 按路径换一个值 -> 再取文本"之后，键顺序与原模板一致。
// 典型证据：按钮串常量 0x102b8740（146 字节，单串）的键序是
//
//	[[{"type":0,"name":"下载图片","data":"...","reply":0,"enter":1,"blue":1}]]
//
// 而该串要原样（只换 data）被 base64 出去（见 CardButtonField）。
// Go 的 map 序列化按字母序重排，会得到 blue/data/enter/name/reply/type，
// 与原串不符，所以这里自己保序收发。

// ojson 是保序 JSON 值：kind 为 'o'（对象）'a'（数组）'s'（串）'n'（数字）'b'（布尔）'z'（null）。
type ojson struct {
	kind byte
	keys []string // 仅 'o'：键序
	vals []*ojson // 'o' 的成员值 / 'a' 的元素
	str  string   // 仅 's'
	num  string   // 仅 'n'：原样数字文本
	b    bool     // 仅 'b'
}

// decodeOJSON 解析 JSON 文本，保留对象键顺序与数字原样文本。
func decodeOJSON(data []byte) (*ojson, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := decOJSON(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			return nil, errors.New("JSON 尾部有多余内容")
		}
		return nil, err
	}
	return v, nil
}

func decOJSON(dec *json.Decoder) (*ojson, error) {
	t, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch tt := t.(type) {
	case json.Delim:
		switch tt {
		case '{':
			o := &ojson{kind: 'o'}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := kt.(string)
				if !ok {
					return nil, fmt.Errorf("对象键不是字符串：%v", kt)
				}
				val, err := decOJSON(dec)
				if err != nil {
					return nil, err
				}
				o.keys = append(o.keys, key)
				o.vals = append(o.vals, val)
			}
			if _, err := dec.Token(); err != nil { // '}'
				return nil, err
			}
			return o, nil
		case '[':
			a := &ojson{kind: 'a'}
			for dec.More() {
				val, err := decOJSON(dec)
				if err != nil {
					return nil, err
				}
				a.vals = append(a.vals, val)
			}
			if _, err := dec.Token(); err != nil { // ']'
				return nil, err
			}
			return a, nil
		}
		return nil, fmt.Errorf("意外的分隔符 %q", tt)
	case string:
		return &ojson{kind: 's', str: tt}, nil
	case json.Number:
		return &ojson{kind: 'n', num: tt.String()}, nil
	case bool:
		return &ojson{kind: 'b', b: tt}, nil
	case nil:
		return &ojson{kind: 'z'}, nil
	}
	return nil, fmt.Errorf("未知 JSON 记号 %T", t)
}

// set 按路径写入文本值，缺失的中间节点自动创建，返回（可能被替换的）节点。
func (o *ojson) set(segs []pathSeg, text string) *ojson {
	return o.setNode(segs, &ojson{kind: 's', str: text})
}

// setNum 按路径写入数字值（原样文本，如 "30" / "0.5"）。
// 对应易语言 JSON 置长整数(+0xf0) / 置整数(+0xf8) / 置小数(+0xe8)。
func (o *ojson) setNum(segs []pathSeg, num string) *ojson {
	return o.setNode(segs, &ojson{kind: 'n', num: num})
}

// setBool 按路径写入逻辑值（对应置逻辑 +0xe0）。
func (o *ojson) setBool(segs []pathSeg, b bool) *ojson {
	return o.setNode(segs, &ojson{kind: 'b', b: b})
}

// setNode 按路径写入已构造好的节点，缺失的中间节点自动创建。
func (o *ojson) setNode(segs []pathSeg, v *ojson) *ojson {
	if len(segs) == 0 {
		return v
	}
	s := segs[0]
	if s.isIdx {
		if o == nil || o.kind != 'a' {
			o = &ojson{kind: 'a'}
		}
		for len(o.vals) <= s.index {
			o.vals = append(o.vals, nil)
		}
		o.vals[s.index] = o.vals[s.index].setNode(segs[1:], v)
		return o
	}
	if o == nil || o.kind != 'o' {
		o = &ojson{kind: 'o'}
	}
	for i, k := range o.keys {
		if k == s.key {
			o.vals[i] = o.vals[i].setNode(segs[1:], v)
			return o
		}
	}
	o.keys = append(o.keys, s.key)
	var child *ojson
	o.vals = append(o.vals, child.setNode(segs[1:], v))
	return o
}

// encode 输出紧凑 JSON 文本。
func (o *ojson) encode(buf *bytes.Buffer) {
	if o == nil {
		buf.WriteString("null")
		return
	}
	switch o.kind {
	case 'o':
		buf.WriteByte('{')
		for i, k := range o.keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			appendJSONString(buf, k)
			buf.WriteByte(':')
			o.vals[i].encode(buf)
		}
		buf.WriteByte('}')
	case 'a':
		buf.WriteByte('[')
		for i, v := range o.vals {
			if i > 0 {
				buf.WriteByte(',')
			}
			v.encode(buf)
		}
		buf.WriteByte(']')
	case 's':
		appendJSONString(buf, o.str)
	case 'n':
		buf.WriteString(o.num)
	case 'b':
		if o.b {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	default:
		buf.WriteString("null")
	}
}

// Text 返回紧凑 JSON 文本（键序保持解析时的顺序）。
func (o *ojson) Text() string {
	var buf bytes.Buffer
	o.encode(&buf)
	return buf.String()
}

// appendJSONString 按 JSON 规则转义字符串，且不转义 < > &（E 语言置文本不转义）。
func appendJSONString(buf *bytes.Buffer, s string) {
	var tmp bytes.Buffer
	enc := json.NewEncoder(&tmp)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		// Encode(string) 不会失败；兜底走标准库 Marshal。
		b, _ := json.Marshal(s)
		buf.Write(b)
		return
	}
	out := tmp.Bytes()
	if n := len(out); n > 0 && out[n-1] == '\n' {
		out = out[:n-1]
	}
	buf.Write(out)
}
