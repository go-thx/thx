package thx

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
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
		Count *int  `thx:"count"`
		Addr  addr  `thx:"addr"`
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

func TestDecodeCheckboxBool(t *testing.T) {
	type form struct {
		Agree  bool  `thx:"agree"`
		Notify *bool `thx:"notify"`
		Legacy bool  `thx:"legacy"`
	}

	var got form
	src := map[string][]string{
		"agree":  {"on"},
		"notify": {"On"},
		"legacy": {"true"},
	}

	if err := newDecoder().decode(&got, src); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Agree || got.Notify == nil || !*got.Notify || !got.Legacy {
		t.Fatalf("got %+v", got)
	}

	var off form
	if err := newDecoder().decode(&off, map[string][]string{"agree": {"off"}}); err != nil {
		t.Fatalf("decode off: %v", err)
	}
	if off.Agree {
		t.Fatal("off decoded as true")
	}

	var bad form
	if err := newDecoder().decode(&bad, map[string][]string{"agree": {"maybe"}}); !errors.Is(err, ErrMalformedValue) {
		t.Fatalf("expected malformed value, got %v", err)
	}
}

func TestDecodeEmptyValuesAreZero(t *testing.T) {
	type form struct {
		Name  string     `thx:"name"`
		Note  *string    `thx:"note"`
		Age   int        `thx:"age"`
		Ratio *float64   `thx:"ratio"`
		At    time.Time  `thx:"at"`
		Due   *time.Time `thx:"due"`
	}

	var got form
	src := map[string][]string{
		"name":  {""},
		"note":  {""},
		"age":   {""},
		"ratio": {""},
		"at":    {""},
		"due":   {""},
	}

	if err := newDecoder().decode(&got, src); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Age != 0 || got.Ratio != nil || !got.At.IsZero() || got.Due != nil {
		t.Fatalf("blank inputs not zeroed: %+v", got)
	}
	// An empty string is a value for string fields, so the pointer is set.
	if got.Note == nil || *got.Note != "" {
		t.Fatalf("note: %v", got.Note)
	}
}

func TestDecodeIndexedSlice(t *testing.T) {
	type item struct {
		Name string `thx:"name"`
		Qty  int    `thx:"qty"`
	}
	type form struct {
		Tags  []string `thx:"tags"`
		Items []item   `thx:"items"`
		Pair  [2]int   `thx:"pair"`
	}

	var got form
	src := map[string][]string{
		"tags.2":       {"c"},
		"tags.0":       {"a"},
		"items.1.name": {"second"},
		"items.1.qty":  {"3"},
		"items.0.name": {"first"},
		"pair.1":       {"9"},
	}

	if err := newDecoder().decode(&got, src); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(got.Tags, []string{"a", "", "c"}) {
		t.Fatalf("tags: %#v", got.Tags)
	}
	want := []item{{Name: "first"}, {Name: "second", Qty: 3}}
	if !reflect.DeepEqual(got.Items, want) {
		t.Fatalf("items: %#v", got.Items)
	}
	if got.Pair != [2]int{0, 9} {
		t.Fatalf("pair: %v", got.Pair)
	}
}

func TestDecodeIndexedPointerSlice(t *testing.T) {
	type item struct {
		Name string `thx:"name"`
	}
	type form struct {
		Items []*item `thx:"items"`
	}

	var got form
	if err := newDecoder().decode(&got, map[string][]string{"items.0.name": {"x"}}); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0] == nil || got.Items[0].Name != "x" {
		t.Fatalf("items: %#v", got.Items)
	}
}

func TestDecodeMap(t *testing.T) {
	type person struct {
		Age int `thx:"age"`
	}
	type form struct {
		Attrs  map[string]string   `thx:"attrs"`
		Counts map[string]int      `thx:"counts"`
		People map[string]person   `thx:"people"`
		Lists  map[string][]string `thx:"lists"`
	}

	var got form
	src := map[string][]string{
		"attrs.color":     {"red"},
		"attrs.size":      {"l"},
		"counts.visits":   {"7"},
		"people.marc.age": {"42"},
		"lists.tags":      {"a", "b"},
	}

	if err := newDecoder().decode(&got, src); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(got.Attrs, map[string]string{"color": "red", "size": "l"}) {
		t.Fatalf("attrs: %#v", got.Attrs)
	}
	if got.Counts["visits"] != 7 {
		t.Fatalf("counts: %#v", got.Counts)
	}
	if got.People["marc"].Age != 42 {
		t.Fatalf("people: %#v", got.People)
	}
	if !reflect.DeepEqual(got.Lists["tags"], []string{"a", "b"}) {
		t.Fatalf("lists: %#v", got.Lists)
	}
}

func TestDecodeMapIntKeyAndMerge(t *testing.T) {
	type row struct {
		Name string `thx:"name"`
		Qty  int    `thx:"qty"`
	}
	type form struct {
		Rows map[int]row `thx:"rows"`
	}

	var got form
	src := map[string][]string{
		"rows.4.name": {"widget"},
		"rows.4.qty":  {"2"},
	}

	if err := newDecoder().decode(&got, src); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Rows[4] != (row{Name: "widget", Qty: 2}) {
		t.Fatalf("rows: %#v", got.Rows)
	}
}

func TestDecodeLimits(t *testing.T) {
	type form struct {
		Tags []string `thx:"tags"`
		Pair [2]int   `thx:"pair"`
	}

	cases := map[string]map[string][]string{
		"slice index": {"tags." + strconv.Itoa(maxIndex): {"x"}},
		"array index": {"pair.7": {"1"}},
		"depth":       {"tags" + strings.Repeat(".0", maxKeyDepth): {"x"}},
	}

	for name, src := range cases {
		var got form
		err := newDecoder().decode(&got, src)
		if !errors.Is(err, ErrLimitExceeded) {
			t.Fatalf("%s: expected limit exceeded, got %v", name, err)
		}
	}

	// A limit violation is not an unknown key, so ignoreUnknownKeys must not
	// swallow it.
	d := newDecoder()
	d.ignoreUnknownKeys(true)
	var got form
	if err := d.decode(&got, map[string][]string{"tags.5000": {"x"}}); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("expected limit exceeded, got %v", err)
	}
}

func TestDecodeErrorKinds(t *testing.T) {
	type form struct {
		Age  int       `thx:"age"`
		Tags []string  `thx:"tags"`
		At   time.Time `thx:"at"`
	}

	var got form
	tests := []struct {
		name string
		src  map[string][]string
		kind DecodeErrorKind
	}{
		{"unknown field", map[string][]string{"nope": {"x"}}, ErrUnknownKey},
		{"non-numeric index", map[string][]string{"tags.first": {"x"}}, ErrUnknownKey},
		{"descend into scalar", map[string][]string{"age.0": {"1"}}, ErrUnknownKey},
		{"descend into leaf", map[string][]string{"at.year": {"2026"}}, ErrUnknownKey},
		{"bad int", map[string][]string{"age": {"nope"}}, ErrMalformedValue},
	}

	for _, tc := range tests {
		err := newDecoder().decode(&got, tc.src)
		if !errors.Is(err, tc.kind) {
			t.Fatalf("%s: expected %s, got %v", tc.name, tc.kind, err)
		}

		var decodeErr *DecodeError
		if !errors.As(err, &decodeErr) {
			t.Fatalf("%s: not a *DecodeError: %v", tc.name, err)
		}
		if decodeErr.Key == "" {
			t.Fatalf("%s: error carries no key: %v", tc.name, err)
		}
	}

	if err := newDecoder().decode(form{}, nil); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("expected invalid target, got %v", err)
	}
	if err := newDecoder().decode(new(int), nil); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("expected invalid target, got %v", err)
	}
}

func TestDecodeIgnoreUnknownNested(t *testing.T) {
	type form struct {
		Tags []string `thx:"tags"`
	}

	d := newDecoder()
	d.ignoreUnknownKeys(true)

	var got form
	src := map[string][]string{"tags.0": {"a"}, "tags.x": {"b"}, "other.1.deep": {"c"}}
	if err := d.decode(&got, src); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(got.Tags, []string{"a"}) {
		t.Fatalf("tags: %#v", got.Tags)
	}
}

func TestWarmRecursiveType(t *testing.T) {
	type node struct {
		Name string  `thx:"name"`
		Next *node   `thx:"next"`
		Kids []*node `thx:"kids"`
	}

	d := newDecoder()
	d.warm(reflect.TypeFor[node]())

	var got node
	if err := d.decode(&got, map[string][]string{"next.kids.0.name": {"deep"}}); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Next == nil || len(got.Next.Kids) != 1 || got.Next.Kids[0].Name != "deep" {
		t.Fatalf("got %+v", got)
	}
}
