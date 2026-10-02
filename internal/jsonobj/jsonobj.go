// Package jsonobj edits a JSON object that belongs to someone else without disturbing it.
//
// The installer writes into files other programs own: OpenCode's opencode.json and Claude
// Code's ~/.claude.json. It changes one entry in each, and a Go map would write every other
// key back in alphabetical order, rewriting a file it has no business reordering. An Object
// keeps its keys in the order they were read, and keeps every value it does not touch as
// the raw bytes it was read as.
package jsonobj

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// Object is a JSON object whose keys keep their order.
type Object struct {
	keys []string
	vals map[string]json.RawMessage
}

func New() *Object { return &Object{vals: map[string]json.RawMessage{}} }

// Parse reads a JSON object. Anything other than an object is an error.
func Parse(data []byte) (*Object, error) {
	o := New()
	if err := o.UnmarshalJSON(data); err != nil {
		return nil, err
	}
	return o, nil
}

func (o *Object) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return errors.New("jsonobj: not an object")
	}

	o.keys, o.vals = nil, map[string]json.RawMessage{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := tok.(string)
		if !ok {
			return fmt.Errorf("jsonobj: unexpected token %v", tok)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return err
		}
		o.Set(key, raw)
	}
	if _, err := dec.Token(); err != nil {
		return err
	}
	if _, err := dec.Token(); err == nil {
		return errors.New("jsonobj: data after the object")
	}
	return nil
}

// MarshalJSON writes the object compactly, values exactly as they were read or set.
func (o *Object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, key := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		quoted, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		b.Write(quoted)
		b.WriteByte(':')
		b.Write(o.vals[key])
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// Get returns the raw value of a key.
func (o *Object) Get(key string) (json.RawMessage, bool) {
	raw, ok := o.vals[key]
	return raw, ok
}

// Set replaces a key's value where the key already is, or appends it.
func (o *Object) Set(key string, raw json.RawMessage) {
	if _, ok := o.vals[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = raw
}

func (o *Object) Delete(key string) {
	if _, ok := o.vals[key]; !ok {
		return
	}
	delete(o.vals, key)
	for i, k := range o.keys {
		if k == key {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			return
		}
	}
}

func (o *Object) Keys() []string { return append([]string(nil), o.keys...) }

func (o *Object) Len() int { return len(o.keys) }

// Child reads a key's value as an object, or returns an empty one when the key is absent
// or holds something else.
func (o *Object) Child(key string) *Object {
	if raw, ok := o.vals[key]; ok {
		if child, err := Parse(raw); err == nil {
			return child
		}
	}
	return New()
}

// SetChild stores an object under a key.
func (o *Object) SetChild(key string, child *Object) {
	raw, _ := child.MarshalJSON()
	o.Set(key, raw)
}

// Raw marshals a value without escaping <, > and &: the prompts Alfred writes are prose
// full of them, and the files are read by people as well as by programs.
func Raw(v any) (json.RawMessage, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

// Format renders the object indented by two spaces, with a trailing newline.
func Format(o *Object) ([]byte, error) {
	compact, err := o.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := json.Indent(&b, compact, "", "  "); err != nil {
		return nil, err
	}
	b.WriteByte('\n')
	return b.Bytes(), nil
}
