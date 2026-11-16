package qpack

import (
	"bytes"
	"reflect"
	"testing"
)

func encodeField(t *testing.T, field HeaderField) []byte {
	t.Helper()
	var wire bytes.Buffer
	encoder := NewEncoder(&wire)
	if err := encoder.WriteField(field); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	return wire.Bytes()
}

func TestDecodeFullSupportsLegacyDecoderAPI(t *testing.T) {
	field := HeaderField{Name: ":method", Value: "GET"}
	var emitted []HeaderField
	decoder := NewDecoder(func(field HeaderField) {
		emitted = append(emitted, field)
	})

	got, err := decoder.DecodeFull(encodeField(t, field))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []HeaderField{field}) {
		t.Fatalf("decoded fields = %#v, want %#v", got, []HeaderField{field})
	}
	if !reflect.DeepEqual(emitted, []HeaderField{field}) {
		t.Fatalf("emitted fields = %#v, want %#v", emitted, []HeaderField{field})
	}
}

func TestDecodeSupportsCurrentDecoderAPI(t *testing.T) {
	field := HeaderField{Name: "x-v2bx", Value: "compatible"}
	decode := NewDecoder().Decode(encodeField(t, field))
	got, err := decode()
	if err != nil {
		t.Fatal(err)
	}
	if got != field {
		t.Fatalf("decoded field = %#v, want %#v", got, field)
	}
}
