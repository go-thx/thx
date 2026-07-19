package thx

import (
	"reflect"
	"testing"
	"time"
)

func TestDecodeScalars(t *testing.T) {
	type form struct {
		Name    string  `thx:"name"`
		Age     int     `thx:"age"`
		Ratio   float64 `thx:"ratio"`
		Active  bool    `thx:"active"`
		Ignored string  `thx:"-"`
		Bare    string
	}

	var got form
	src := map[string][]string{
		"name":   {"marc"},
		"age":    {"42"},
		"ratio":  {"1.5"},
		"active": {"true"},
		"Bare":   {"bare"},
	}

	d := newDecoder()
	d.ignoreUnknownKeys(true)
	if err := d.decode(&got, map[string][]string{"Ignored": {"nope"}}); err != nil {
		t.Fatalf("decode ignored: %v", err)
	}
	if got.Ignored != "" {
		t.Fatalf("thx:\"-\" field was populated: %q", got.Ignored)
	}

	if err := newDecoder().decode(&got, src); err != nil {
		t.Fatalf("decode: %v", err)
	}

	want := form{Name: "marc", Age: 42, Ratio: 1.5, Active: true, Bare: "bare"}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestDecodeSliceAndLastWins(t *testing.T) {
	type form struct {
		Tags   []string `thx:"tag"`
		Single string   `thx:"single"`
	}

	var got form
	src := map[string][]string{
		"tag":    {"a", "b", "c"},
		"single": {"first", "last"},
	}

	if err := newDecoder().decode(&got, src); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !reflect.DeepEqual(got.Tags, []string{"a", "b", "c"}) {
		t.Fatalf("tags: %v", got.Tags)
	}
	if got.Single != "last" {
		t.Fatalf("single: %q", got.Single)
	}
}

func TestDecodePointerAndNested(t *testing.T) {
	type addr struct {
		City string `thx:"city"`
	}
	type form struct {
		Count *int `thx:"count"`
		Addr  addr `thx:"addr"`
		Home  *addr `thx:"home"`
	}

	var got form
	src := map[string][]string{
		"count":     {"7"},
		"addr.city": {"berlin"},
		"home.city": {"munich"},
	}

	if err := newDecoder().decode(&got, src); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.Count == nil || *got.Count != 7 {
		t.Fatalf("count: %v", got.Count)
	}
	if got.Addr.City != "berlin" {
		t.Fatalf("addr.city: %q", got.Addr.City)
	}
	if got.Home == nil || got.Home.City != "munich" {
		t.Fatalf("home: %+v", got.Home)
	}
}

func TestDecodeEmbedded(t *testing.T) {
	type base struct {
		ID string `thx:"id"`
	}
	type form struct {
		base
		Name string `thx:"name"`
	}

	var got form
	src := map[string][]string{"id": {"x1"}, "name": {"n"}}

	if err := newDecoder().decode(&got, src); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != "x1" || got.Name != "n" {
		t.Fatalf("got %+v", got)
	}
}

func TestDecodeIgnoreUnknown(t *testing.T) {
	type form struct {
		Name string `thx:"name"`
	}

	src := map[string][]string{"name": {"a"}, "extra": {"b"}}

	var strict form
	if err := newDecoder().decode(&strict, src); err == nil {
		t.Fatal("expected error on unknown key")
	}

	d := newDecoder()
	d.ignoreUnknownKeys(true)
	var lax form
	if err := d.decode(&lax, src); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if lax.Name != "a" {
		t.Fatalf("name: %q", lax.Name)
	}
}

func TestDecodeCustomConverter(t *testing.T) {
	type form struct {
		At time.Time `thx:"at"`
	}

	d := newDecoder()
	d.registerConverter(time.Time{}, func(s string) reflect.Value {
		ts, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return reflect.Value{}
		}
		return reflect.ValueOf(ts)
	})

	var got form
	src := map[string][]string{"at": {"2026-07-14T00:00:00Z"}}
	if err := d.decode(&got, src); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.At.Equal(time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("at: %v", got.At)
	}
}

func TestDecodeUnexportedTagged(t *testing.T) {
	type form struct {
		secret string `thx:"secret"` //nolint:unused // asserts unexported fields are skipped
		Name   string `thx:"name"`
	}

	var got form
	src := map[string][]string{"secret": {"x"}, "name": {"ok"}}

	d := newDecoder()
	d.ignoreUnknownKeys(true)
	if err := d.decode(&got, src); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != "ok" {
		t.Fatalf("name: %q", got.Name)
	}
}

func TestDecodeConverterPointer(t *testing.T) {
	type form struct {
		At *time.Time `thx:"at"`
	}

	d := newDecoder()
	d.registerConverter(time.Time{}, func(s string) reflect.Value {
		ts, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return reflect.Value{}
		}
		return reflect.ValueOf(ts)
	})

	var got form
	if err := d.decode(&got, map[string][]string{"at": {"2026-07-14T00:00:00Z"}}); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.At == nil || !got.At.Equal(time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("at: %v", got.At)
	}
}

func TestDecodeBytes(t *testing.T) {
	type form struct {
		Blob []byte `thx:"blob"`
	}

	var got form
	if err := newDecoder().decode(&got, map[string][]string{"blob": {"hello"}}); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(got.Blob) != "hello" {
		t.Fatalf("blob: %q", got.Blob)
	}
}

func TestDecodeErrors(t *testing.T) {
	type form struct {
		Age int `thx:"age"`
	}

	if err := newDecoder().decode(form{}, nil); err == nil {
		t.Fatal("expected error for non-pointer")
	}

	var got form
	if err := newDecoder().decode(&got, map[string][]string{"age": {"nope"}}); err == nil {
		t.Fatal("expected error for bad int")
	}
}
