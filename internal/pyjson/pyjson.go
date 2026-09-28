// Package pyjson reads and writes JSON the way CPython's json module does.
//
// It exists because the Go port has to produce byte-identical files to the Python
// scripts it replaces, and encoding/json differs from json.dumps in four ways that all
// show up in Alfred's output: Go sorts map keys where Python preserves insertion order,
// Go writes "," and ":" where Python writes ", " and ": ", Go escapes <, > and & where
// Python does not, and Go emits raw UTF-8 where Python escapes it. An ordered value with
// its own encoder is the only way to keep a diff against the Python output empty.
package pyjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
)

type Kind int

const (
	Null Kind = iota
	Bool
	Number
	String
	Array
	Object
)

// Member is one key/value pair of an object, kept in the order it was read or written.
type Member struct {
	Key string
	Val *Value
}

// Value is a JSON value whose objects remember their key order.
type Value struct {
	Kind Kind
	B    bool
	Num  json.Number
	Str  string
	Arr  []*Value
	Obj  []Member
}

func NewObject() *Value         { return &Value{Kind: Object} }
func NewArray() *Value          { return &Value{Kind: Array} }
func NewString(s string) *Value { return &Value{Kind: String, Str: s} }
func NewBool(b bool) *Value     { return &Value{Kind: Bool, B: b} }

// Get returns the member's value, or nil when the key is absent or the value is not an
// object. A missing key and a null value are distinguishable: this returns nil only for
// the former.
func (v *Value) Get(key string) *Value {
	if v == nil || v.Kind != Object {
		return nil
	}
	for _, m := range v.Obj {
		if m.Key == key {
			return m.Val
		}
	}
	return nil
}

// Set replaces a member in place, keeping its position, or appends it when new. This is
// what makes `d[k] = x` and `d.setdefault(k, x)` reproducible: Python keeps the original
// position of a key that already exists.
func (v *Value) Set(key string, val *Value) {
	for i := range v.Obj {
		if v.Obj[i].Key == key {
			v.Obj[i].Val = val
			return
		}
	}
	v.Obj = append(v.Obj, Member{Key: key, Val: val})
}

// SetDefault appends the value only when the key is absent, as dict.setdefault does.
func (v *Value) SetDefault(key string, val *Value) {
	if v.Get(key) == nil {
		v.Set(key, val)
	}
}

// Members returns the object's key/value pairs in order, and nothing for a nil value or
// a value of another kind, so a missing key reads as an empty object rather than panicking.
func (v *Value) Members() []Member {
	if v == nil || v.Kind != Object {
		return nil
	}
	return v.Obj
}

// Keys returns the object's keys in order.
func (v *Value) Keys() []string {
	if v == nil || v.Kind != Object {
		return nil
	}
	keys := make([]string, 0, len(v.Obj))
	for _, m := range v.Obj {
		keys = append(keys, m.Key)
	}
	return keys
}

// StringOr returns a string value, or the fallback when the value is absent or another
// type. Profiles are hand-edited, so a wrong type is reported as absent rather than fatal.
func (v *Value) StringOr(fallback string) string {
	if v == nil || v.Kind != String {
		return fallback
	}
	return v.Str
}

// Strings returns an array of strings, skipping members of any other type.
func (v *Value) Strings() []string {
	if v == nil || v.Kind != Array {
		return nil
	}
	out := make([]string, 0, len(v.Arr))
	for _, item := range v.Arr {
		if item.Kind == String {
			out = append(out, item.Str)
		}
	}
	return out
}

// Decode parses JSON, preserving object key order.
func Decode(data []byte) (*Value, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	val, err := decodeValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing data after JSON value")
	}
	return val, nil
}

func decodeValue(dec *json.Decoder) (*Value, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	return decodeFrom(dec, tok)
}

func decodeFrom(dec *json.Decoder, tok json.Token) (*Value, error) {
	switch t := tok.(type) {
	case nil:
		return &Value{Kind: Null}, nil
	case bool:
		return &Value{Kind: Bool, B: t}, nil
	case json.Number:
		return &Value{Kind: Number, Num: t}, nil
	case string:
		return &Value{Kind: String, Str: t}, nil
	case json.Delim:
		switch t {
		case '{':
			obj := NewObject()
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyTok.(string)
				if !ok {
					return nil, errors.New("object key is not a string")
				}
				val, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				obj.Set(key, val)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return obj, nil
		case '[':
			arr := NewArray()
			for dec.More() {
				val, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				arr.Arr = append(arr.Arr, val)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return arr, nil
		}
	}
	return nil, fmt.Errorf("unexpected token %v", tok)
}

// Encode renders the value as json.dumps would. A zero indent is json.dumps(obj), which
// separates with ", " and ": "; a positive indent is json.dumps(obj, indent=n).
func Encode(v *Value, indent int) string {
	var b strings.Builder
	encode(&b, v, indent, 0)
	return b.String()
}

func encode(b *strings.Builder, v *Value, indent, depth int) {
	if v == nil {
		b.WriteString("null")
		return
	}

	switch v.Kind {
	case Null:
		b.WriteString("null")
	case Bool:
		if v.B {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case Number:
		b.WriteString(v.Num.String())
	case String:
		b.WriteString(EncodeString(v.Str))
	case Array:
		encodeSeq(b, len(v.Arr), '[', ']', indent, depth, func(i int) {
			encode(b, v.Arr[i], indent, depth+1)
		})
	case Object:
		encodeSeq(b, len(v.Obj), '{', '}', indent, depth, func(i int) {
			b.WriteString(EncodeString(v.Obj[i].Key))
			b.WriteString(": ")
			encode(b, v.Obj[i].Val, indent, depth+1)
		})
	}
}

// encodeSeq writes the shared shape of arrays and objects. Python renders an empty
// container as "[]" or "{}" with no inner whitespace even in indent mode.
func encodeSeq(b *strings.Builder, n int, openCh, closeCh byte, indent, depth int, item func(int)) {
	b.WriteByte(openCh)
	if n == 0 {
		b.WriteByte(closeCh)
		return
	}

	pad := strings.Repeat(" ", indent*(depth+1))
	for i := range n {
		if i > 0 {
			b.WriteByte(',')
			if indent == 0 {
				b.WriteByte(' ')
			}
		}
		if indent > 0 {
			b.WriteByte('\n')
			b.WriteString(pad)
		}
		item(i)
	}
	if indent > 0 {
		b.WriteByte('\n')
		b.WriteString(strings.Repeat(" ", indent*depth))
	}
	b.WriteByte(closeCh)
}

// EncodeString quotes a string as json.dumps does with its default ensure_ascii=True:
// short escapes where they exist, \uXXXX for every control character and every rune
// above U+007E, surrogate pairs above the BMP, and no escaping of <, > or &.
func EncodeString(s string) string {
	var b strings.Builder
	b.WriteByte('"')

	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			switch {
			case r < 0x20 || r > 0x7e:
				if r > 0xffff {
					hi, lo := utf16.EncodeRune(r)
					fmt.Fprintf(&b, `\u%04x\u%04x`, hi, lo)
				} else {
					fmt.Fprintf(&b, `\u%04x`, r)
				}
			default:
				b.WriteRune(r)
			}
		}
	}

	b.WriteByte('"')
	return b.String()
}
