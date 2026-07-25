package thx

import (
	"encoding"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// tagName is the struct tag key used to map form and query fields to struct
// fields. A field tagged `thx:"-"` is skipped entirely.
const tagName = "thx"

// keySeparator separates the path segments of a key: struct field names, slice
// and array indices, and map keys.
const keySeparator = "."

const (
	// maxKeyDepth bounds how many segments a single key may have, so deeply
	// nested keys cannot drive unbounded recursion.
	maxKeyDepth = 32

	// maxIndex bounds slice and array indices, so a key like "items.99999999"
	// cannot make a client force a huge allocation.
	maxIndex = 1000
)

// DecodeErrorKind classifies why decoding failed. It is itself an error, so a
// kind can be matched with errors.Is:
//
//	if errors.Is(err, thx.ErrUnknownKey) { /* client sent a key we don't know */ }
type DecodeErrorKind string

const (
	// ErrUnknownKey means the key does not address any field of the target
	// struct. Only returned for form bodies; query strings ignore extra keys.
	ErrUnknownKey DecodeErrorKind = "unknown key"

	// ErrMalformedValue means the key addressed a field, but the submitted
	// value could not be converted to that field's type.
	ErrMalformedValue DecodeErrorKind = "malformed value"

	// ErrLimitExceeded means the key exceeded a decoder limit: too many path
	// segments, or an index beyond what a client may address.
	ErrLimitExceeded DecodeErrorKind = "limit exceeded"

	// ErrInvalidTarget means the decode destination itself is wrong — a bug in
	// the handler's type parameters, not in the request.
	ErrInvalidTarget DecodeErrorKind = "invalid target"
)

func (k DecodeErrorKind) Error() string { return string(k) }

// DecodeError is the error returned by form and query decoding. Kind separates
// client mistakes (ErrUnknownKey, ErrMalformedValue, ErrLimitExceeded) from
// programming mistakes (ErrInvalidTarget), and Key names the offending input.
type DecodeError struct {
	Kind DecodeErrorKind
	Key  string
	Err  error
}

func (e *DecodeError) Error() string {
	switch {
	case e.Key == "" && e.Err == nil:
		return "thx: " + string(e.Kind)
	case e.Key == "":
		return fmt.Sprintf("thx: %s: %v", e.Kind, e.Err)
	case e.Err == nil:
		return fmt.Sprintf("thx: %s %q", e.Kind, e.Key)
	default:
		return fmt.Sprintf("thx: %q: %s: %v", e.Key, e.Kind, e.Err)
	}
}

func (e *DecodeError) Unwrap() error { return e.Err }

// Is matches a DecodeError against its kind, so errors.Is(err, ErrUnknownKey)
// works without unwrapping to a *DecodeError first.
func (e *DecodeError) Is(target error) bool {
	kind, ok := target.(DecodeErrorKind)
	return ok && kind == e.Kind
}

func unknownKey(key string) error {
	return &DecodeError{Kind: ErrUnknownKey, Key: key}
}

func malformed(key string, err error) error {
	return &DecodeError{Kind: ErrMalformedValue, Key: key, Err: err}
}

// converter turns a single raw string value into a reflect.Value of the target
// type. Register one via decoder.registerConverter to control how a custom type
// is decoded. It returns the zero Value if the input cannot be converted.
type converter func(string) reflect.Value

// decoder decodes url.Values-shaped data (map[string][]string) into a struct
// using `thx` struct tags. It is safe for concurrent use once configured.
type decoder struct {
	ignoreUnknown bool
	converters    map[reflect.Type]converter
	cache         sync.Map // reflect.Type -> map[string]structField
}

// newDecoder returns a decoder that maps struct fields by their `thx` tag.
func newDecoder() *decoder {
	return &decoder{converters: map[reflect.Type]converter{}}
}

// ignoreUnknownKeys controls whether keys without a matching struct field are
// silently ignored (true) or cause decode to return an error (false).
func (d *decoder) ignoreUnknownKeys(ignore bool) {
	d.ignoreUnknown = ignore
}

// registerConverter registers a converter for the type of value, overriding the
// built-in decoding for that type.
func (d *decoder) registerConverter(value any, conv converter) {
	d.converters[reflect.TypeOf(value)] = conv
}

// structField is one directly addressable field of a struct type. index is the
// field-index path from that struct, including any embedded parents.
type structField struct {
	index []int
	typ   reflect.Type
}

// warm builds and caches the field maps for a struct type and everything
// reachable from it ahead of time, so the reflection walk is paid once at
// startup rather than on the first request. Non-struct types are ignored.
func (d *decoder) warm(typ reflect.Type) {
	d.warmType(typ, map[reflect.Type]bool{})
}

func (d *decoder) warmType(typ reflect.Type, seen map[reflect.Type]bool) {
	if typ == nil || seen[typ] {
		return
	}
	seen[typ] = true

	switch typ.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		d.warmType(typ.Elem(), seen)
	case reflect.Struct:
		if d.isLeaf(typ) {
			return
		}
		for _, f := range d.fieldsFor(typ) {
			d.warmType(f.typ, seen)
		}
	}
}

// decode populates dst (a pointer to a struct) from src. Keys are matched
// case-sensitively and are dotted paths: "addr.city" addresses a nested struct
// field, "items.0.name" an element of a slice or array, "attrs.color" an entry
// of a map.
func (d *decoder) decode(dst any, src map[string][]string) error {
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return &DecodeError{Kind: ErrInvalidTarget, Err: fmt.Errorf("expected a non-nil pointer to a struct, got %T", dst)}
	}

	root := rv.Elem()
	if root.Kind() != reflect.Struct {
		return &DecodeError{Kind: ErrInvalidTarget, Err: fmt.Errorf("expected a pointer to a struct, got pointer to %s", root.Kind())}
	}

	for key, values := range src {
		segments := strings.Split(key, keySeparator)
		if len(segments) > maxKeyDepth {
			return &DecodeError{
				Kind: ErrLimitExceeded,
				Key:  key,
				Err:  fmt.Errorf("key nests %d levels, limit is %d", len(segments), maxKeyDepth),
			}
		}

		err := d.walk(root, segments, key, values)
		if err != nil {
			if d.ignoreUnknown && errors.Is(err, ErrUnknownKey) {
				continue
			}
			return err
		}
	}

	return nil
}

// walk descends the remaining key segments through target and assigns values to
// the leaf it reaches. target is always addressable.
func (d *decoder) walk(target reflect.Value, segments []string, key string, values []string) error {
	typ := target.Type()

	if len(segments) == 0 {
		return d.setField(target, key, values)
	}

	// Types decoded from a single value are leaves, even when they are structs
	// or slices underneath, so a key must not descend into them.
	if d.isLeaf(typ) {
		return unknownKey(key)
	}

	switch typ.Kind() {
	case reflect.Pointer:
		if target.IsNil() {
			target.Set(reflect.New(typ.Elem()))
		}
		return d.walk(target.Elem(), segments, key, values)

	case reflect.Struct:
		f, ok := d.fieldsFor(typ)[segments[0]]
		if !ok {
			return unknownKey(key)
		}
		return d.walk(fieldByIndex(target, f.index), segments[1:], key, values)

	case reflect.Slice:
		if typ.Elem().Kind() == reflect.Uint8 {
			return unknownKey(key)
		}
		idx, err := parseIndex(segments[0], key)
		if err != nil {
			return err
		}
		if idx >= target.Len() {
			grow(target, idx+1)
		}
		return d.walk(target.Index(idx), segments[1:], key, values)

	case reflect.Array:
		idx, err := parseIndex(segments[0], key)
		if err != nil {
			return err
		}
		if idx >= target.Len() {
			return &DecodeError{
				Kind: ErrLimitExceeded,
				Key:  key,
				Err:  fmt.Errorf("index %d exceeds array length %d", idx, target.Len()),
			}
		}
		return d.walk(target.Index(idx), segments[1:], key, values)

	case reflect.Map:
		return d.walkMap(target, segments, key, values)

	default:
		return unknownKey(key)
	}
}

// walkMap descends into a map entry. Map values are not addressable, so the
// entry is decoded into a temporary value and stored back afterwards.
func (d *decoder) walkMap(target reflect.Value, segments []string, key string, values []string) error {
	typ := target.Type()
	if target.IsNil() {
		target.Set(reflect.MakeMap(typ))
	}

	mapKey := reflect.New(typ.Key()).Elem()
	if err := d.setScalar(mapKey, segments[0], key); err != nil {
		return err
	}

	entry := reflect.New(typ.Elem()).Elem()
	if existing := target.MapIndex(mapKey); existing.IsValid() {
		entry.Set(existing)
	}
	if err := d.walk(entry, segments[1:], key, values); err != nil {
		return err
	}

	target.SetMapIndex(mapKey, entry)
	return nil
}

// setField assigns values to a leaf according to its type, handling slices
// (one element per value) and scalars (last value wins).
func (d *decoder) setField(target reflect.Value, key string, values []string) error {
	if len(values) == 0 {
		return nil
	}

	typ := target.Type()
	if typ.Kind() == reflect.Slice && typ.Elem().Kind() != reflect.Uint8 && !d.isLeaf(typ) {
		slice := reflect.MakeSlice(typ, len(values), len(values))
		for i, raw := range values {
			if err := d.setScalar(slice.Index(i), raw, key); err != nil {
				return err
			}
		}
		target.Set(slice)
		return nil
	}

	return d.setScalar(target, values[len(values)-1], key)
}

// setScalar converts a single raw string into target, following pointers and
// consulting registered converters and encoding.TextUnmarshaler.
func (d *decoder) setScalar(target reflect.Value, raw, key string) error {
	typ := target.Type()

	// Browsers submit an empty string for untouched number, date, and select
	// inputs. For types where that is not a value, it means "left blank".
	if raw == "" && !acceptsEmpty(typ) {
		target.Set(reflect.Zero(typ))
		return nil
	}

	if conv, ok := d.converters[typ]; ok {
		out := conv(raw)
		if !out.IsValid() {
			return malformed(key, fmt.Errorf("converter rejected value %q", raw))
		}
		target.Set(out)
		return nil
	}

	if typ.Kind() == reflect.Pointer {
		ptr := reflect.New(typ.Elem())
		if err := d.setScalar(ptr.Elem(), raw, key); err != nil {
			return err
		}
		target.Set(ptr)
		return nil
	}

	if target.CanAddr() {
		if u, ok := target.Addr().Interface().(encoding.TextUnmarshaler); ok {
			if err := u.UnmarshalText([]byte(raw)); err != nil {
				return malformed(key, err)
			}
			return nil
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
		b, err := parseBool(raw)
		if err != nil {
			return malformed(key, err)
		}
		target.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, typ.Bits())
		if err != nil {
			return malformed(key, err)
		}
		target.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, typ.Bits())
		if err != nil {
			return malformed(key, err)
		}
		target.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(raw, typ.Bits())
		if err != nil {
			return malformed(key, err)
		}
		target.SetFloat(f)
	default:
		return malformed(key, fmt.Errorf("unsupported field type %s", typ))
	}

	return nil
}

// acceptsEmpty reports whether the empty string is a value for typ rather than
// an unfilled input.
func acceptsEmpty(typ reflect.Type) bool {
	typ = deref(typ)
	if typ.Kind() == reflect.String {
		return true
	}
	return typ.Kind() == reflect.Slice && typ.Elem().Kind() == reflect.Uint8
}

// parseBool extends strconv.ParseBool with HTML's checkbox convention: an
// unchecked box submits nothing at all, a checked one submits "on".
func parseBool(raw string) (bool, error) {
	switch {
	case strings.EqualFold(raw, "on"):
		return true, nil
	case strings.EqualFold(raw, "off"):
		return false, nil
	}
	return strconv.ParseBool(raw)
}

// parseIndex reads a slice or array index segment. A non-numeric segment does
// not address anything, so it is an unknown key rather than a bad value.
func parseIndex(segment, key string) (int, error) {
	idx, err := strconv.Atoi(segment)
	if err != nil || idx < 0 {
		return 0, unknownKey(key)
	}
	if idx >= maxIndex {
		return 0, &DecodeError{
			Kind: ErrLimitExceeded,
			Key:  key,
			Err:  fmt.Errorf("index %d exceeds limit %d", idx, maxIndex),
		}
	}
	return idx, nil
}

// grow resizes a slice to n elements, keeping the ones already decoded. Indexed
// keys arrive in map order, so a later index can be seen first.
func grow(target reflect.Value, n int) {
	grown := reflect.MakeSlice(target.Type(), n, n)
	reflect.Copy(grown, target)
	target.Set(grown)
}

// fieldsFor returns the cached field map for a struct type, building it on the
// first request.
func (d *decoder) fieldsFor(typ reflect.Type) map[string]structField {
	if cached, ok := d.cache.Load(typ); ok {
		return cached.(map[string]structField)
	}
	fields := map[string]structField{}
	d.collectFields(typ, nil, fields)
	actual, _ := d.cache.LoadOrStore(typ, fields)
	return actual.(map[string]structField)
}

// collectFields maps the directly addressable names of a struct type to their
// fields, flattening embedded structs into the parent namespace.
func (d *decoder) collectFields(typ reflect.Type, indexPrefix []int, out map[string]structField) {
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
				d.collectFields(ft, index, out)
				continue
			}
		}

		if !sf.IsExported() {
			continue
		}
		if name == "" {
			name = sf.Name
		}

		out[name] = structField{index: index, typ: sf.Type}
	}
}

// isLeaf reports whether a type should be decoded from a single value rather
// than descended into: it has a registered converter or implements
// encoding.TextUnmarshaler.
func (d *decoder) isLeaf(typ reflect.Type) bool {
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
