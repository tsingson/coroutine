package types

import (
	"fmt"
	"reflect"

	coroutinev1 "github.com/stealthrocket/coroutine/gen/proto/go/coroutine/v1"
)

// Inspect inspects serialized durable coroutine state.
//
// The input should be a buffer produced by (*coroutine.Context).Marshal
// or by types.Serialize.
func Inspect(b []byte) (*State, error) {
	var state coroutinev1.State
	if err := state.UnmarshalVT(b); err != nil {
		return nil, err
	}
	return &State{state: &state}, nil
}

// State wraps durable coroutine state.
type State struct {
	state *coroutinev1.State
}

// BuildID returns the build ID of the program that generated this state.
func (s *State) BuildID() string {
	return s.state.Build.Id
}

// OS returns the operating system the coroutine was compiled for.
func (s *State) OS() string {
	return s.state.Build.Os
}

// Arch returns the architecture the coroutine was compiled for.
func (s *State) Arch() string {
	return s.state.Build.Arch
}

// NumType returns the number of types referenced by the coroutine.
func (s *State) NumType() int {
	return len(s.state.Types)
}

// Type returns a type by index.
func (s *State) Type(i int) *Type {
	if i < 0 || i >= len(s.state.Types) {
		panic(fmt.Sprintf("type %d not found", i))
	}
	return &Type{
		state: s,
		typ:   s.state.Types[i],
		index: i,
	}
}

// NumFunction returns the number of functions/methods/closures
// referenced by the coroutine.
func (s *State) NumFunction() int {
	return len(s.state.Functions)
}

// Function returns a function by index.
func (s *State) Function(i int) *Function {
	if i < 0 || i >= len(s.state.Functions) {
		panic(fmt.Sprintf("function %d not found", i))
	}
	return &Function{
		state:    s,
		function: s.state.Functions[i],
		index:    i,
	}
}

// NumRegion returns the number of memory regions referenced by the
// coroutine.
func (s *State) NumRegion() int {
	return len(s.state.Regions)
}

// Region retrieves a region by index.
func (s *State) Region(i int) *Region {
	if i < 0 || i >= len(s.state.Regions) {
		panic(fmt.Sprintf("region %d not found", i))
	}
	return &Region{
		state:  s,
		region: s.state.Regions[i],
		index:  i,
	}
}

// NumString returns the number of strings referenced by types.
func (s *State) NumString() int {
	return len(s.state.Strings)
}

// String retrieves a string by index.
func (s *State) String(i int) string {
	if i < 0 || i >= len(s.state.Strings) {
		panic(fmt.Sprintf("string %d not found", i))
	}
	return s.state.Strings[i]
}

// Root is the root object that was serialized.
func (s *State) Root() *Region {
	return &Region{
		state:  s,
		region: s.state.Root,
		index:  -1,
	}
}

// Type is a type referenced by a durable coroutine.
type Type struct {
	state *State
	typ   *coroutinev1.Type
	index int
}

// Index is the index of the type in the serialized state.
func (t *Type) Index() int {
	return t.index
}

// Name is the name of the type within the package it was defined.
func (t *Type) Name() string {
	if t.typ.Name == 0 {
		return ""
	}
	return t.state.String(int(t.typ.Name - 1))
}

// Package is the name of the package that defines the type.
func (t *Type) Package() string {
	if t.typ.Package == 0 {
		return ""
	}
	return t.state.String(int(t.typ.Package - 1))
}

// Kind is the underlying kind for this type.
func (t *Type) Kind() reflect.Kind {
	switch t.typ.Kind {
	case coroutinev1.Kind_KIND_BOOL:
		return reflect.Bool
	case coroutinev1.Kind_KIND_INT:
		return reflect.Int
	case coroutinev1.Kind_KIND_INT8:
		return reflect.Int8
	case coroutinev1.Kind_KIND_INT16:
		return reflect.Int16
	case coroutinev1.Kind_KIND_INT32:
		return reflect.Int32
	case coroutinev1.Kind_KIND_INT64:
		return reflect.Int64
	case coroutinev1.Kind_KIND_UINT:
		return reflect.Uint
	case coroutinev1.Kind_KIND_UINT8:
		return reflect.Uint8
	case coroutinev1.Kind_KIND_UINT16:
		return reflect.Uint16
	case coroutinev1.Kind_KIND_UINT32:
		return reflect.Uint32
	case coroutinev1.Kind_KIND_UINT64:
		return reflect.Uint64
	case coroutinev1.Kind_KIND_UINTPTR:
		return reflect.Uintptr
	case coroutinev1.Kind_KIND_FLOAT32:
		return reflect.Float32
	case coroutinev1.Kind_KIND_FLOAT64:
		return reflect.Float64
	case coroutinev1.Kind_KIND_COMPLEX64:
		return reflect.Complex64
	case coroutinev1.Kind_KIND_COMPLEX128:
		return reflect.Complex128
	case coroutinev1.Kind_KIND_ARRAY:
		return reflect.Array
	case coroutinev1.Kind_KIND_CHAN:
		return reflect.Chan
	case coroutinev1.Kind_KIND_FUNC:
		return reflect.Func
	case coroutinev1.Kind_KIND_INTERFACE:
		return reflect.Interface
	case coroutinev1.Kind_KIND_MAP:
		return reflect.Map
	case coroutinev1.Kind_KIND_POINTER:
		return reflect.Pointer
	case coroutinev1.Kind_KIND_SLICE:
		return reflect.Slice
	case coroutinev1.Kind_KIND_STRING:
		return reflect.String
	case coroutinev1.Kind_KIND_STRUCT:
		return reflect.Struct
	case coroutinev1.Kind_KIND_UNSAFE_POINTER:
		return reflect.UnsafePointer
	default:
		panic(fmt.Sprintf("invalid type kind %s", t.typ.Kind))
	}
}

// Elem is the type of an array, slice, pointer, chan or map's element.
func (t *Type) Elem() *Type {
	if t.typ.Elem == 0 {
		return nil
	}
	return t.state.Type(int(t.typ.Elem - 1))
}

// Key is the key type for map types.
func (t *Type) Key() *Type {
	if t.typ.Key == 0 {
		return nil
	}
	return t.state.Type(int(t.typ.Key - 1))
}

// NumField is the number of struct fields for struct types.
func (t *Type) NumField() int {
	return len(t.typ.Fields)
}

// Field retrieves a struct field by index.
func (t *Type) Field(i int) *Field {
	if i < 0 || i >= len(t.typ.Fields) {
		return nil
	}
	return &Field{
		state: t.state,
		field: t.typ.Fields[i],
	}
}

// NumParam is the number of parameters for function types.
func (t *Type) NumParam() int {
	return len(t.typ.Params)
}

// Param is the type of a function parameter with the specified index.
func (t *Type) Param(i int) *Type {
	if i < 0 || i >= len(t.typ.Params) {
		return nil
	}
	return t.state.Type(int(t.typ.Params[i] - 1))
}

// NumResult is the number of results for function types.
func (t *Type) NumResult() int {
	return len(t.typ.Results)
}

// Result is the type of a function result with the specified index.
func (t *Type) Result(i int) *Type {
	if i < 0 || i >= len(t.typ.Results) {
		return nil
	}
	return t.state.Type(int(t.typ.Results[i] - 1))
}

// Len is the length of an array type.
func (t *Type) Len() int {
	return int(t.typ.Length)
}

// MemoryOffset is the location of this type in memory.
//
// The offset is only applicable to the build that generated the state.
func (t *Type) MemoryOffset() uint64 {
	return t.typ.MemoryOffset
}

// ChanDir is the direction of a channel type.
func (t *Type) ChanDir() reflect.ChanDir {
	switch t.typ.ChanDir {
	case coroutinev1.ChanDir_CHAN_DIR_RECV:
		return reflect.RecvDir
	case coroutinev1.ChanDir_CHAN_DIR_SEND:
		return reflect.SendDir
	case coroutinev1.ChanDir_CHAN_DIR_BOTH:
		return reflect.BothDir
	default:
		panic(fmt.Sprintf("invalid chan dir %s", t.typ.ChanDir))
	}
}

// Variadic is true for function types with a variadic final argument.
func (t *Type) Variadic() bool {
	return t.typ.Variadic
}

// Opaue is true for types that had a custom serializer registered
// in the program that generated the coroutine state. Custom types
// are opaque and cannot be inspected.
func (t *Type) Opaque() bool {
	return t.typ.CustomSerializer > 0
}

// Format implements fmt.Formatter.
func (t *Type) Format(s fmt.State, v rune) {
	name := t.Name()
	if pkg := t.Package(); pkg != "" {
		if name == "" {
			name = fmt.Sprintf("<anon %s>", t.Kind())
		}
		name = pkg + "." + name
	}

	if t.Opaque() {
		if name == "" {
			name = fmt.Sprintf("<anon %s>", t.Kind())
		}
		if t.typ.Kind == coroutinev1.Kind_KIND_POINTER {
			name = "*" + name
		}
		s.Write([]byte(name))
		return
	}

	verbose := s.Flag('+') || s.Flag('#')
	if name != "" && !verbose {
		s.Write([]byte(name))
		return
	}

	var primitiveKind string
	switch t.typ.Kind {
	case coroutinev1.Kind_KIND_BOOL:
		primitiveKind = "bool"
	case coroutinev1.Kind_KIND_INT:
		primitiveKind = "int"
	case coroutinev1.Kind_KIND_INT8:
		primitiveKind = "int8"
	case coroutinev1.Kind_KIND_INT16:
		primitiveKind = "int16"
	case coroutinev1.Kind_KIND_INT32:
		primitiveKind = "int32"
	case coroutinev1.Kind_KIND_INT64:
		primitiveKind = "int64"
	case coroutinev1.Kind_KIND_UINT:
		primitiveKind = "uint"
	case coroutinev1.Kind_KIND_UINT8:
		primitiveKind = "uint8"
	case coroutinev1.Kind_KIND_UINT16:
		primitiveKind = "uint16"
	case coroutinev1.Kind_KIND_UINT32:
		primitiveKind = "uint32"
	case coroutinev1.Kind_KIND_UINT64:
		primitiveKind = "uint64"
	case coroutinev1.Kind_KIND_UINTPTR:
		primitiveKind = "uintptr"
	case coroutinev1.Kind_KIND_FLOAT32:
		primitiveKind = "float32"
	case coroutinev1.Kind_KIND_FLOAT64:
		primitiveKind = "float64"
	case coroutinev1.Kind_KIND_COMPLEX64:
		primitiveKind = "complex64"
	case coroutinev1.Kind_KIND_COMPLEX128:
		primitiveKind = "complex128"
	case coroutinev1.Kind_KIND_STRING:
		primitiveKind = "string"
	case coroutinev1.Kind_KIND_INTERFACE:
		primitiveKind = "interface"
	case coroutinev1.Kind_KIND_UNSAFE_POINTER:
		primitiveKind = "unsafe.Pointer"
	}
	if primitiveKind != "" {
		if name == primitiveKind {
			name = ""
		}
		var result string
		switch {
		case (name == "error" && primitiveKind == "interface") ||
			(name == "any" && primitiveKind == "interface") ||
			(name == "byte" && primitiveKind == "uint8") ||
			(name == "rune" && primitiveKind == "int32"):
			result = name
		case name != "":
			result = fmt.Sprintf("(%s=%s)", name, primitiveKind)
		default:
			result = primitiveKind
		}
		s.Write([]byte(result))
		return
	}

	var elemPrefix string
	switch t.typ.Kind {
	case coroutinev1.Kind_KIND_ARRAY:
		elemPrefix = fmt.Sprintf("[%d]", t.Len())
	case coroutinev1.Kind_KIND_CHAN:
		switch t.typ.ChanDir {
		case coroutinev1.ChanDir_CHAN_DIR_RECV:
			elemPrefix = "<-chan "
		case coroutinev1.ChanDir_CHAN_DIR_SEND:
			elemPrefix = "chan<- "
		default:
			elemPrefix = "chan "
		}
	case coroutinev1.Kind_KIND_POINTER:
		elemPrefix = "*"
	case coroutinev1.Kind_KIND_SLICE:
		elemPrefix = "[]"
	}
	if elemPrefix != "" {
		if name != "" {
			elemPrefix = fmt.Sprintf("(%s=%s", name, elemPrefix)
		}
		s.Write([]byte(elemPrefix))
		t.Elem().Format(withoutFlags{s}, v)
		if name != "" {
			s.Write([]byte(")"))
		}
		return
	}

	if name != "" {
		s.Write([]byte(fmt.Sprintf("(%s=", name)))
	}
	switch t.typ.Kind {
	case coroutinev1.Kind_KIND_FUNC:
		s.Write([]byte("func("))
		paramCount := t.NumParam()
		for i := 0; i < paramCount; i++ {
			if i > 0 {
				s.Write([]byte(", "))
			}
			if i == paramCount-1 && t.Variadic() {
				s.Write([]byte("..."))
			}
			t.Param(i).Format(withoutFlags{s}, v)
		}
		s.Write([]byte(")"))
		n := t.NumResult()
		if n > 0 {
			s.Write([]byte(" "))
		}
		if n > 1 {
			s.Write([]byte("("))
		}
		for i := 0; i < n; i++ {
			if i > 0 {
				s.Write([]byte(", "))
			}
			t.Result(i).Format(withoutFlags{s}, v)
		}
		if n > 1 {
			s.Write([]byte(")"))
		}
		if name != "" {
			s.Write([]byte(")"))
		}

	case coroutinev1.Kind_KIND_MAP:
		s.Write([]byte("map["))
		t.Key().Format(withoutFlags{s}, v)
		s.Write([]byte("]"))
		t.Elem().Format(withoutFlags{s}, v)

	case coroutinev1.Kind_KIND_STRUCT:
		n := t.NumField()
		if n == 0 {
			s.Write([]byte("struct{}"))
		} else {
			s.Write([]byte("struct{ "))
			for i := 0; i < n; i++ {
				if i > 0 {
					s.Write([]byte("; "))
				}
				f := t.Field(i)
				if !f.Anonymous() {
					s.Write([]byte(f.Name()))
					s.Write([]byte(" "))
				}
				f.Type().Format(withoutFlags{State: s}, v)
			}
			s.Write([]byte(" }"))
		}

	default:
		s.Write([]byte("invalid"))
	}
	if name != "" {
		s.Write([]byte(")"))
	}
}

type withoutFlags struct{ fmt.State }

func (withoutFlags) Flag(c int) bool { return false }

// Field is a struct field.
type Field struct {
	state *State
	field *coroutinev1.Field
}

// Name is the name of the field.
func (f *Field) Name() string {
	if f.field.Name == 0 {
		return ""
	}
	return f.state.String(int(f.field.Name - 1))
}

// Package is the package path that qualifies a lwer case (unexported)
// field name. It is empty for upper case (exported) field names.
func (f *Field) Package() string {
	if f.field.Package == 0 {
		return ""
	}
	return f.state.String(int(f.field.Package - 1))
}

// Type is the type of the field.
func (f *Field) Type() *Type {
	return f.state.Type(int(f.field.Type - 1))
}

// Offset is the offset of the field within its struct, in bytes.
func (f *Field) Offset() uint64 {
	return f.field.Offset
}

// Anonymous is true of the field is an embedded field (with a name
// derived from its type).
func (f *Field) Anonymous() bool {
	return f.field.Anonymous
}

// Tag contains struct field metadata.
func (f *Field) Tag() reflect.StructTag {
	return reflect.StructTag(f.field.Tag)
}

// Function is a function, method or closure referenced by the coroutine.
type Function struct {
	state    *State
	function *coroutinev1.Function
	index    int
}

// Name is the name of the function.
func (f *Function) Name() string {
	if f.function.Name == 0 {
		return ""
	}
	return f.state.String(int(f.function.Name - 1))
}

// Index is the index of the function in the serialized state.
func (f *Function) Index() int {
	return f.index
}

// Type is the type of the function.
func (f *Function) Type() *Type {
	return f.state.Type(int(f.function.Type - 1))
}

// ClosureType returns the memory layout for closure functions.
//
// The returned type is a struct where the first field is a function
// pointer and the remaining fields are the variables from outer scopes
// that are referenced by the closure.
//
// Nil is returned for functions that are not closures.
func (f *Function) ClosureType() *Type {
	if f.function.Closure == 0 {
		return nil
	}
	return f.state.Type(int(f.function.Closure - 1))
}

// String is the name of the function.
func (f *Function) String() string {
	return f.Name()
}

// Region is a region of memory referenced by the coroutine.
type Region struct {
	state  *State
	region *coroutinev1.Region
	index  int
}

// Index is the index of the region in the serialized state,
// or -1 if this is the root region.
func (t *Region) Index() int {
	return t.index
}

// Type is the type of the region.
func (r *Region) Type() *Type {
	return r.state.Type(int(r.region.Type - 1))
}

// Size is the size of the region in bytes.
func (r *Region) Size() int64 {
	return int64(len(r.region.Data))
}

// String is a summary of the region in string form.
func (r *Region) String() string {
	return fmt.Sprintf("Region(%d byte(s), %#v)", len(r.region.Data), r.Type())
}
