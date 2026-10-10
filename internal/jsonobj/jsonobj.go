// Package jsonobj edits a JSON object owned by another program (opencode.json,
// ~/.claude.json) without disturbing it: keys keep their original order and untouched values
// keep their raw bytes, unlike a Go map, which would re-sort every key.
package jsonobj

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// Object is a JSON object whose keys keep their order and whose values stay raw bytes.
type Object struct {
	keys []string
	vals map[string]json.RawMessage
}

// New returns an empty Object.
func New() *Object { return &Object{vals: map[string]json.RawMessage{}} }

// Parse reads a JSON object. Anything other than an object is an error.
func Parse(data []byte) (*Object, error) {
	object := New()
	if err := object.UnmarshalJSON(data); err != nil {
		return nil, err
	}
	return object, nil
}

// UnmarshalJSON replaces the object's contents with the JSON object in data, keeping key
// order. Anything other than a single object, including trailing data, is an error.
func (o *Object) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return errors.New("jsonobj: not an object")
	}

	o.keys, o.vals = nil, map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			return fmt.Errorf("jsonobj: unexpected token %v", token)
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
		o.Set(key, raw)
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	if _, err := decoder.Token(); err == nil {
		return errors.New("jsonobj: data after the object")
	}
	return nil
}

// MarshalJSON writes the object compactly, values exactly as they were read or set.
func (o *Object) MarshalJSON() ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteByte('{')
	for i, key := range o.keys {
		if i > 0 {
			buffer.WriteByte(',')
		}
		quoted, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		buffer.Write(quoted)
		buffer.WriteByte(':')
		buffer.Write(o.vals[key])
	}
	buffer.WriteByte('}')
	return buffer.Bytes(), nil
}

// Get returns the raw value of a key and whether the key is present.
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

// Delete removes a key and its value; a missing key is a no-op.
func (o *Object) Delete(key string) {
	if _, ok := o.vals[key]; !ok {
		return
	}
	delete(o.vals, key)
	for i, existing := range o.keys {
		if existing == key {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			return
		}
	}
}

// Keys returns a copy of the keys in order.
func (o *Object) Keys() []string { return append([]string(nil), o.keys...) }

// Len returns the number of keys.
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

// Raw marshals a value without escaping <, > and &, since the prompts Alfred writes are
// prose full of them and people read these files too.
func Raw(value any) (json.RawMessage, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buffer.Bytes(), "\n"), nil
}

// Format renders the object indented by two spaces, with a trailing newline.
func Format(object *Object) ([]byte, error) {
	compact, err := object.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	if err := json.Indent(&buffer, compact, "", "  "); err != nil {
		return nil, err
	}
	buffer.WriteByte('\n')
	return buffer.Bytes(), nil
}
