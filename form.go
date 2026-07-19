package thx

import (
	"encoding"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// tagName is the struct tag key used to map form and query fields to struct
// fields. A field tagged `thx:"-"` is skipped entirely.
const tagName = "thx"

// Converter turns a single raw string value into a reflect.Value of the target
// type. Register one via Decoder.RegisterConverter to control how a custom type
// is decoded. It returns the zero Value if the input cannot be converted.
type Converter func(string) reflect.Value

// Decoder decodes url.Values-shaped data (map[string][]string) into a struct
// using `thx` struct tags. It is safe for concurrent use once configured.
type Decoder struct {
	ignoreUnknown bool
	converters    map[reflect.Type]Converter
	cache         sync.Map // reflect.Type -> map[string]field
}

// NewDecoder returns a Decoder that maps struct fields by their `thx` tag.
func NewDecoder() *Decoder {
	return &Decoder{converters: map[reflect.Type]Converter{}}
}

// IgnoreUnknownKeys controls whether keys without a matching struct field are
// silently ignored (true) or cause Decode to return an error (false).
func (d *Decoder) IgnoreUnknownKeys(ignore bool) {
	d.ignoreUnknown = ignore
}

// RegisterConverter registers a Converter for the type of value, overriding the
// built-in decoding for that type.
func (d *Decoder) RegisterConverter(value any, converter Converter) {
	d.converters[reflect.TypeOf(value)] = converter
}

// field is a decodable leaf, addressed by its full dotted key. index is the
// field-index path from the root struct, including any embedded parents.
type field struct {
	index []int
	typ   reflect.Type
}

// Warm builds and caches the field map for a struct type ahead of time, so the
// reflection walk is paid once at startup rather than on the first request.
// Non-struct types are ignored.
func (d *Decoder) Warm(typ reflect.Type) {
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == nil || typ.Kind() != reflect.Struct {
		return
	}
	d.fieldsFor(typ)
}

// Decode populates dst (a pointer to a struct) from src. Keys are matched
// case-sensitively; dotted keys (e.g. "addr.city") address nested struct
// fields.
func (d *Decoder) Decode(dst any, src map[string][]string) error {
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("thx: Decode expects a non-nil pointer to a struct, got %T", dst)
	}

	root := rv.Elem()
	if root.Kind() != reflect.Struct {
		return fmt.Errorf("thx: Decode expects a pointer to a struct, got pointer to %s", root.Kind())
	}

	fields := d.fieldsFor(root.Type())

	for key, values := range src {
		f, ok := fields[key]
		if !ok {
			if d.ignoreUnknown {
				continue
			}
			return fmt.Errorf("thx: unknown key %q", key)
		}

		if err := d.setField(fieldByIndex(root, f.index), f.typ, values); err != nil {
			return fmt.Errorf("thx: %q: %w", key, err)
		}
	}

	return nil
}

// setField assigns values to a field according to its type, handling slices
// (one element per value) and scalars (last value wins).
func (d *Decoder) setField(target reflect.Value, typ reflect.Type, values []string) error {
	if len(values) == 0 {
		return nil
	}

	if typ.Kind() == reflect.Slice && typ.Elem().Kind() != reflect.Uint8 {
		elemType := typ.Elem()
		slice := reflect.MakeSlice(typ, len(values), len(values))
		for i, raw := range values {
			if err := d.setScalar(slice.Index(i), elemType, raw); err != nil {
				return err
			}
		}
		target.Set(slice)
		return nil
	}

	return d.setScalar(target, typ, values[len(values)-1])
}

// setScalar converts a single raw string into target, following pointers and
// consulting registered converters and encoding.TextUnmarshaler.
func (d *Decoder) setScalar(target reflect.Value, typ reflect.Type, raw string) error {
	if conv, ok := d.converters[typ]; ok {
		out := conv(raw)
		if !out.IsValid() {
			return fmt.Errorf("converter rejected value %q", raw)
		}
		target.Set(out)
		return nil
	}

	if typ.Kind() == reflect.Pointer {
		ptr := reflect.New(typ.Elem())
		if err := d.setScalar(ptr.Elem(), typ.Elem(), raw); err != nil {
			return err
		}
		target.Set(ptr)
		return nil
	}

	if target.CanAddr() {
		if u, ok := target.Addr().Interface().(encoding.TextUnmarshaler); ok {
			return u.UnmarshalText([]byte(raw))
		}
	}

	if typ.Kind() == reflect.Slice && typ.Elem().Kind() == reflect.Uint8 {
		target.SetBytes([]byte(raw))
		return nil
	}

	switch typ.Kind() {
	case reflect.String:
		target.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		target.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, typ.Bits())
		if err != nil {
			return err
		}
		target.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, typ.Bits())
		if err != nil {
			return err
		}
		target.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(raw, typ.Bits())
		if err != nil {
			return err
		}
		target.SetFloat(f)
	default:
		return fmt.Errorf("unsupported field type %s", typ)
	}

	return nil
}

// fieldsFor returns the cached flat field map for a struct type, building it on
// the first request.
func (d *Decoder) fieldsFor(typ reflect.Type) map[string]field {
	if cached, ok := d.cache.Load(typ); ok {
		return cached.(map[string]field)
	}
	fields := map[string]field{}
	d.buildFields(typ, nil, "", fields)
	actual, _ := d.cache.LoadOrStore(typ, fields)
	return actual.(map[string]field)
}

// buildFields walks a struct type into a flat map of dotted key -> leaf field,
// flattening embedded structs and descending into nested structs.
func (d *Decoder) buildFields(typ reflect.Type, indexPrefix []int, keyPrefix string, out map[string]field) {
	for i := range typ.NumField() {
		sf := typ.Field(i)

		tag := sf.Tag.Get(tagName)
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")

		index := append(append([]int{}, indexPrefix...), i)

		// Embedded anonymous struct without an explicit tag: flatten its fields
		// into the parent namespace.
		if sf.Anonymous && name == "" {
			ft := deref(sf.Type)
			if ft.Kind() == reflect.Struct && !d.isLeaf(sf.Type) {
				d.buildFields(ft, index, keyPrefix, out)
				continue
			}
		}

		if !sf.IsExported() {
			continue
		}
		if name == "" {
			name = sf.Name
		}

		key := name
		if keyPrefix != "" {
			key = keyPrefix + "." + name
		}

		ft := deref(sf.Type)
		if ft.Kind() == reflect.Struct && !d.isLeaf(sf.Type) {
			d.buildFields(ft, index, key, out)
			continue
		}

		out[key] = field{index: index, typ: sf.Type}
	}
}

// isLeaf reports whether a struct type should be decoded from a single value
// rather than descended into: it has a registered converter or implements
// encoding.TextUnmarshaler.
func (d *Decoder) isLeaf(typ reflect.Type) bool {
	if _, ok := d.converters[typ]; ok {
		return true
	}
	if _, ok := d.converters[deref(typ)]; ok {
		return true
	}
	return isTextUnmarshaler(typ)
}

func deref(typ reflect.Type) reflect.Type {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ
}

var textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()

func isTextUnmarshaler(typ reflect.Type) bool {
	if typ.Kind() != reflect.Pointer {
		typ = reflect.PointerTo(typ)
	}
	return typ.Implements(textUnmarshalerType)
}

// fieldByIndex descends index, allocating nil pointers to nested structs along
// the way so the leaf field is addressable.
func fieldByIndex(v reflect.Value, index []int) reflect.Value {
	v = v.Field(index[0])
	for _, x := range index[1:] {
		if v.Kind() == reflect.Pointer {
			if v.IsNil() {
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.Field(x)
	}
	return v
}
