// Package marshaling provides a uniform thread-safe interface to various encoding/decoding formats
// such as Gob, JSON, and MessagePack, following the JSON marshal/unmarshal function interface.
package marshaling

import (
	"bytes"
	"encoding/gob"
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/vmihailenco/msgpack/v5"
)

type Marshaler interface {
	Marshal(value any) ([]byte, error)
	Unmarshal(marshaled []byte, result any) error
}

// Gob

type Gob struct {
	// The Gob marshaler would be faster if it reused the encoder and decoder, instead of
	// constructing new ones on each method call. However, then it wouldn't be concurrency-safe.
	// We prefer to use MsgPack because it's probably faster anyways.
}

func (m Gob) Marshal(value any) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(value); err != nil {
		return nil, errors.Wrapf(err, "couldn't gob-encode value %#v", value)
	}
	return buf.Bytes(), nil
}

func (m Gob) Unmarshal(marshaled []byte, result any) error {
	buf := bytes.NewBuffer(marshaled)
	dec := gob.NewDecoder(buf)
	if err := dec.Decode(result); err != nil {
		return errors.Wrapf(err, "couldn't gob-decode type %T from bytes %+v", result, marshaled)
	}
	return nil
}

// Json

type JSON struct{}

func (m JSON) Marshal(value any) ([]byte, error) {
	return json.Marshal(value)
}

func (m JSON) Unmarshal(marshaled []byte, result any) error {
	return json.Unmarshal(marshaled, result)
}

// MessagePack

type MessagePack struct{}

func (m MessagePack) Marshal(value any) ([]byte, error) {
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	enc.SetCustomStructTag("json")
	if err := enc.Encode(value); err != nil {
		return nil, errors.Wrapf(err, "couldn't msgpack-encode value %#v", value)
	}
	return buf.Bytes(), nil
}

func (m MessagePack) Unmarshal(marshaled []byte, result any) error {
	buf := bytes.NewBuffer(marshaled)
	dec := msgpack.NewDecoder(buf)
	dec.SetCustomStructTag("json")
	if err := dec.Decode(result); err != nil {
		return errors.Wrapf(err, "couldn't msgpack-decode type %T from bytes %+v", result, marshaled)
	}
	return nil
}
