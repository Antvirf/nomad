// Package object contains interfaces and structures to represent
// object values within Sentinel.
//
// Runtime implementations may choose not to use these representations.
// The purpose for a standard set of representations of object values
// is to have a single share-able API for optimizers, semantic passes,
// package implementations, etc. It doesn't need to be used for actual
// execution.
package object

import (
	"bytes"
	"fmt"
	"sort"

	sdk "github.com/hashicorp/sentinel-sdk"
	"github.com/hashicorp/sentinel/lang/ast"
	"github.com/hashicorp/sentinel/lang/token"
)

// Object is the value of an entity.
type Object interface {
	Type() Type     // The type of this value
	String() string // Human-friendly representation for debugging

	object()
}

type (
	// UndefinedObj represents an undefined value.
	UndefinedObj struct {
		Pos []token.Pos // len(Pos) > 0
	}

	// nullObj represents null. This isn't exported because it is a
	// singleton that should be referenced with Null
	nullObj struct{}

	// BoolObj represents a boolean value. This value should NOT be
	// created directly. Instead, the singleton value should be created
	// with the Bool function.
	BoolObj struct {
		Value bool
	}

	// IntObj represents an integer value.
	IntObj struct {
		Value int64
	}

	// FloatObj represents a float value.
	FloatObj struct {
		Value float64
	}

	// StringObj represents a string value.
	StringObj struct {
		Value string
	}

	// ListObj represents a list of values.
	ListObj struct {
		Elts []Object
	}

	// MapObj represents a key/value mapping.
	MapObj struct {
		Elts []KeyedObj
	}

	// MemoizedRemoteObj represents a memoized return result.  This
	// usually comes from a plugin.
	//
	// During specific steps of lookups of data in the underlying
	// MapObj, the tag will follow returns of relevant data.
	MemoizedRemoteObj struct {
		Tag     string  // Referential tag, ie: import name
		Depth   int64   // The traversal index, incremented each lookup
		Context *MapObj // The data
	}

	// RuleObj represents a rule.
	RuleObj struct {
		Scope     *Scope       // Scope to evaluate the rule in
		Rule      *ast.RuleLit // Rule AST
		Eval      bool         // Eval is true if evaluated
		WhenValue Object       // WhenValue is the value of the predicate once evaluated
		Value     Object       // Value is the set value once evaluated
	}

	// KeyedObj represents a key/value pair. This doens't actual implement
	// Object.
	KeyedObj struct {
		Key   Object
		Value Object
	}

	// FuncObj represents a function
	FuncObj struct {
		Params []*ast.Ident
		Body   []ast.Stmt
		Scope  *Scope // Scope to evaluate the func in
	}

	// ExternalObj represents external keyed data that is loaded on-demand.
	ExternalObj struct {
		External External
	}

	// RuntimeObj represents runtime-specific data. This is unsafe to use
	// unless you're sure that the runtime will be able to handle this data.
	// Please reference the runtime documentation for more information.
	RuntimeObj struct {
		Value interface{}
	}

	// ModuleObj represents a module, a scope behind a namespace.
	ModuleObj struct {
		Scope *Scope
	}

	// ImportObj represents a reference to a loaded import. Multiple
	// references may point to the same import as a singleton.
	ImportObj struct {
		Import sdk.Import
	}

	// MemoizedRemoteObjMiss represents a missed lookup on a
	// MemoizedRemoteObj. This is used to signal a lookup or call back
	// to the source for ultimate confirmation if the data is present
	// or missing.
	MemoizedRemoteObjMiss struct {
		*MemoizedRemoteObj
	}
)

func (o *UndefinedObj) Type() Type          { return UNDEFINED }
func (o *nullObj) Type() Type               { return NULL }
func (o *BoolObj) Type() Type               { return BOOL }
func (o *IntObj) Type() Type                { return INT }
func (o *FloatObj) Type() Type              { return FLOAT }
func (o *StringObj) Type() Type             { return STRING }
func (o *ListObj) Type() Type               { return LIST }
func (o *MapObj) Type() Type                { return MAP }
func (o *MemoizedRemoteObj) Type() Type     { return MEMOIZED_REMOTE_OBJECT }
func (o *RuleObj) Type() Type               { return RULE }
func (o *FuncObj) Type() Type               { return FUNC }
func (o *ExternalObj) Type() Type           { return EXTERNAL }
func (o *RuntimeObj) Type() Type            { return RUNTIME }
func (o *ModuleObj) Type() Type             { return MODULE }
func (o *ImportObj) Type() Type             { return IMPORT }
func (o *MemoizedRemoteObjMiss) Type() Type { return MEMOIZED_REMOTE_OBJECT_MISS }

func (o *UndefinedObj) String() string { return "undefined" }
func (o *nullObj) String() string      { return "null" }
func (o *BoolObj) String() string      { return fmt.Sprintf("%v", o.Value) }
func (o *IntObj) String() string       { return fmt.Sprintf("%d", o.Value) }
func (o *FloatObj) String() string     { return fmt.Sprintf("%f", o.Value) }
func (o *StringObj) String() string    { return fmt.Sprintf("%q", o.Value) }
func (o *ListObj) String() string      { return fmt.Sprintf("%s", o.Elts) }

func (o *MapObj) String() string {
	var buf bytes.Buffer
	buf.WriteString(`{`)

	// Get the keys and sort them
	keys := make([]string, len(o.Elts))
	keyIndex := make(map[string]int, len(o.Elts))
	for i, elt := range o.Elts {
		keys[i] = elt.Key.String()
		keyIndex[keys[i]] = i
	}
	sort.Strings(keys)

	// Build the list of map elements alphabetically ordered
	for i, key := range keys {
		last := i >= len(keys)-1
		elt := o.Elts[keyIndex[key]]

		buf.WriteString(fmt.Sprintf("%s: %s", key, elt.Value))
		if !last {
			buf.WriteString(", ")
		}
	}

	buf.WriteString(`}`)
	return buf.String()
}

func (o *RuleObj) String() string {
	// TODO: use printer here once we have it

	if !o.Eval {
		return fmt.Sprintf("un-evaluated rule: %#v", o.Rule)
	}

	return fmt.Sprintf("evaluated rule. result: %v, result: %v", o.Value, o.Rule)
}
func (o *MemoizedRemoteObj) String() string { return o.Context.String() }
func (o *FuncObj) String() string           { return "func" }
func (o *ExternalObj) String() string       { return "external" }
func (o *RuntimeObj) String() string        { return "runtime" }
func (o *ModuleObj) String() string         { return "module" }
func (o *ImportObj) String() string         { return "import" }
func (o *MemoizedRemoteObjMiss) String() string {
	return fmt.Sprintf("missed lookup on memoized remote object. tag: %q", o.Tag)
}

func (o *UndefinedObj) object()          {}
func (o *nullObj) object()               {}
func (o *BoolObj) object()               {}
func (o *IntObj) object()                {}
func (o *FloatObj) object()              {}
func (o *StringObj) object()             {}
func (o *ListObj) object()               {}
func (o *MemoizedRemoteObj) object()     {}
func (o *MapObj) object()                {}
func (o *RuleObj) object()               {}
func (o *FuncObj) object()               {}
func (o *ExternalObj) object()           {}
func (o *RuntimeObj) object()            {}
func (o *ModuleObj) object()             {}
func (o *ImportObj) object()             {}
func (o *MemoizedRemoteObjMiss) object() {}

// Copyable is an interface for objects that need to support special
// duplication logic. This mainly applies to collections; in this
// particular application, the runtime uses this interface to create
// copies of collections that may be freely modified with index
// assignments without affecting the original.
type Copyable interface {
	Copy() Object
}

// Copy implements Copyable for ListObj.
func (o *ListObj) Copy() Object {
	l := &ListObj{
		Elts: make([]Object, len(o.Elts)),
	}

	for i, elt := range o.Elts {
		if c, ok := elt.(Copyable); ok {
			l.Elts[i] = c.Copy()
		} else {
			l.Elts[i] = elt
		}
	}

	return l
}

// Copy implements Copyable for MapObj.
func (o *MapObj) Copy() Object {
	m := &MapObj{
		Elts: make([]KeyedObj, len(o.Elts)),
	}

	for i, elt := range o.Elts {
		if c, ok := elt.Value.(Copyable); ok {
			m.Elts[i] = KeyedObj{
				Key:   elt.Key,
				Value: c.Copy(),
			}
		} else {
			m.Elts[i] = elt
		}
	}

	return m
}

// Copy implements Copyable for MemoizedRemoteObj.
func (o *MemoizedRemoteObj) Copy() Object {
	return &MemoizedRemoteObj{
		Tag:     o.Tag,
		Depth:   o.Depth,
		Context: o.Context.Copy().(*MapObj),
	}
}

// Copy implements Copyable for MemoizedRemoteObjMiss.
//
// This is mostly included for completeness and safety. It
// technically should not be needed - when updating receivers over
// the import bridge in the interpreter, the receiver context
// (map) is usually assigned a new copy of the map from the import
// return data.
//
// Conversely, this function call should be a small cost over the RPC
// call and data conversion that may happen as a result of the import
// call itself, so singling it out for elimination in the spirit of
// optimization should be a last resort.
func (o *MemoizedRemoteObjMiss) Copy() Object {
	return &MemoizedRemoteObjMiss{
		MemoizedRemoteObj: o.MemoizedRemoteObj.Copy().(*MemoizedRemoteObj),
	}
}
