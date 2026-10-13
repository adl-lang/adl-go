// M3 behavioural tests for the govisitor sub-task: generic structs and
// unions, and newtypes (plan4.md sections 3, 4.6 and 7). They compile
// the generated generated/m3/{gen,nt} packages and run hand-written
// visitors over hand-built values; nothing here compares against golden
// files.
//
// m3.gen has Box<T> (a T field, a Vector<T> field and an Atom field),
// Opt<T> (a T branch, a Void branch and a Vector<Atom> branch) and
// Weird<P, R>, whose type params clash with the payload/result params.
// m3.nt has the newtypes Path = Vector<Atom>, Wrapped = Atom (across
// modules), Id = String and Ids<T> = Vector<T>, and a Holder struct with
// a field of every M3 shape plus a bundle-generic leaf.
package m3_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"adl_visitor_tests/generated/m2/tree"
	"adl_visitor_tests/generated/m3/gen"
	"adl_visitor_tests/generated/m3/nt"
	"github.com/adl-lang/adl-go/adl/sys/types"
)

// unit is the (P, R) pair for visitors that carry no payload and return
// no result.
type unit = struct{}

var errBoom = errors.New("boom")

// ---------------------------------------------------------------------
// fixtures
// ---------------------------------------------------------------------

func atom(name string) tree.Atom {
	return tree.MakeAll_Atom(name, false)
}

func ptr[T any](v T) *T {
	return &v
}

func eq(t *testing.T, what string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: got %#v, want %#v", what, got, want)
	}
}

// newBox holds Atoms in item and items as well as tag. Only tag is ever
// reached: item (T) and items (Vector<T>) are leaves of the generic decl,
// whatever T is instantiated with.
func newBox() gen.Box[tree.Atom] {
	return gen.MakeAll_Box[tree.Atom](
		atom("item"),
		[]tree.Atom{
			atom("i1"),
			atom("i2"),
		},
		atom("tag"),
	)
}

func newSBox() gen.Box[string] {
	return gen.MakeAll_Box[string](
		"item",
		[]string{
			"i1",
		},
		atom("stag"),
	)
}

func newPath(names ...string) nt.Path {
	var p = make(nt.Path, 0, len(names))
	for _, n := range names {
		p = append(p, atom(n))
	}
	return p
}

// newHolder fills every field of m3.nt.Holder.
func newHolder() nt.Holder {
	return nt.MakeAll_Holder(
		newBox(),
		newSBox(),
		gen.Make_Opt_xs[tree.Atom]([]tree.Atom{
			atom("x1"),
			atom("x2"),
		}),
		newPath("p1", "p2"),
		nt.Wrapped(atom("w")),
		nt.Id("id"),
		[]nt.Path{
			newPath("pp1"),
			newPath(),
			newPath("pp2", "pp3"),
		},
		ptr(nt.Wrapped(atom("nw"))),
		nt.Ids[tree.Atom]{
			atom("ids1"),
		},
		types.Make_Maybe_just[tree.Atom](atom("maybe")),
	)
}

// holderAtoms is the order a full walk of newHolder reaches every Atom.
// Box.item/items, Ids and Maybe hold Atoms that are never reached.
var holderAtoms = []string{
	"tag",      // box: Box<Atom>.tag
	"stag",     // sbox: Box<String>.tag
	"x1", "x2", // opt: Opt.xs
	"p1", "p2", // path
	"w",          // wrapped
	"pp1",        // paths[0]
	"pp2", "pp3", // paths[2]; paths[1] is empty
	"nw", // nullWrapped
}

// ---------------------------------------------------------------------
// visitors
// ---------------------------------------------------------------------

// atomLister has only a plain VisitAtom: everything else falls through
// DefaultAccept.
type atomLister struct {
	seen []string
}

var (
	_ tree.Visitor_Atom[unit, unit] = (*atomLister)(nil)
)

func (l *atomLister) VisitAtom(node *tree.Atom) {
	l.seen = append(l.seen, node.Name)
}

// atomName is an R-only visitor: VisitAtom returns the atom's name, so a
// node's DefaultAccept returns the name of the last atom it descended to
// and "" when it descended to none.
type atomName struct{}

var (
	_ tree.VisitorR_Atom[unit, string] = (*atomName)(nil)
)

func (*atomName) VisitAtom(node *tree.Atom) string {
	return node.Name
}

// stopAtAtom is the E variant: fails on the atom with the given name.
type stopAtAtom struct {
	stop string
	seen []string
}

var (
	_ tree.VisitorE_Atom[unit, unit] = (*stopAtAtom)(nil)
)

func (s *stopAtAtom) VisitAtom(node *tree.Atom) error {
	s.seen = append(s.seen, node.Name)
	if node.Name == s.stop {
		return errBoom
	}
	return nil
}

// boxCounter is the (int, int) visitor over Box<Atom>: VisitBox is the
// PRE variant instantiated as [tree.Atom, int, int]; it descends with
// depth+1 and adds one to the result. VisitAtom records the depth.
type boxCounter struct {
	boxes  int
	atoms  int
	depths map[string]int
}

var (
	_ gen.VisitorPRE_Box[tree.Atom, int, int] = (*boxCounter)(nil)
	_ tree.VisitorPR_Atom[int, int]           = (*boxCounter)(nil)
)

func newBoxCounter() *boxCounter {
	return &boxCounter{
		depths: map[string]int{},
	}
}

func (c *boxCounter) VisitBox(node *gen.Box[tree.Atom], depth int) (result int, err error) {
	c.boxes++
	if result, err = node.DefaultAccept[int, int](c, depth+1); err != nil {
		return
	}
	return result + 1, nil
}

func (c *boxCounter) VisitAtom(node *tree.Atom, depth int) int {
	c.atoms++
	c.depths[node.Name] = depth
	return 1
}

// optLister has a union-level VisitOpt on Opt<Atom> (RE, instantiated
// [tree.Atom, unit, string]) that records the branch and descends, and a
// VisitAtom.
type optLister struct {
	opts  []string
	atoms []string
}

var (
	_ gen.VisitorRE_Opt[tree.Atom, unit, string] = (*optLister)(nil)
	_ tree.VisitorR_Atom[unit, string]           = (*optLister)(nil)
)

func (l *optLister) VisitOpt(node *gen.Opt[tree.Atom]) (result string, err error) {
	var kind = strings.TrimPrefix(reflect.TypeOf(node.Branch).Name(), "_Opt_")
	// A generic branch type's reflect name carries its instantiation.
	kind, _, _ = strings.Cut(kind, "[")
	l.opts = append(l.opts, kind)
	if result, err = node.DefaultAccept[unit, string](l, unit{}); err != nil {
		return
	}
	return kind + ":" + result, nil
}

func (l *optLister) VisitAtom(node *tree.Atom) string {
	l.atoms = append(l.atoms, node.Name)
	return node.Name
}

// weirdVisitor visits Weird<P, R> instantiated as Weird[int, string],
// with the visitor's own payload and result (the renamed P2, R2) bool
// and int. VisitWeird is the PR variant.
type weirdVisitor struct {
	payloads []bool
	atoms    []string
}

var (
	_ gen.VisitorPR_Weird[int, string, bool, int] = (*weirdVisitor)(nil)
	_ tree.VisitorPR_Atom[bool, int]              = (*weirdVisitor)(nil)
)

func (w *weirdVisitor) VisitWeird(node *gen.Weird[int, string], payload bool) (result int) {
	w.payloads = append(w.payloads, payload)
	result, _ = node.DefaultAccept[bool, int](w, !payload)
	return result + node.P
}

func (w *weirdVisitor) VisitAtom(node *tree.Atom, payload bool) int {
	w.atoms = append(w.atoms, node.Name)
	if payload {
		return 100
	}
	return 10
}

// pathGate has a VisitPath (E) on the Path newtype that only descends
// when told to, and a VisitAtom.
type pathGate struct {
	descend bool
	paths   int
	atoms   []string
}

var (
	_ nt.VisitorE_Path[unit, unit]  = (*pathGate)(nil)
	_ tree.Visitor_Atom[unit, unit] = (*pathGate)(nil)
)

func (g *pathGate) VisitPath(node *nt.Path) (err error) {
	g.paths++
	if g.descend {
		_, err = node.DefaultAccept[unit, unit](g, unit{})
	}
	return
}

func (g *pathGate) VisitAtom(node *tree.Atom) {
	g.atoms = append(g.atoms, node.Name)
}

// pathFail fails (PE) on the Path whose length is stop and counts the
// atoms (P) reached.
type pathFail struct {
	stop  int
	atoms int
}

var (
	_ nt.VisitorPE_Path[int, int]  = (*pathFail)(nil)
	_ tree.VisitorP_Atom[int, int] = (*pathFail)(nil)
)

func (f *pathFail) VisitPath(node *nt.Path, depth int) (err error) {
	if len(*node) == f.stop {
		return errBoom
	}
	_, err = node.DefaultAccept[int, int](f, depth+1)
	return
}

func (f *pathFail) VisitAtom(node *tree.Atom, depth int) {
	f.atoms++
}

// wrappedVisitor has a VisitWrapped (R) that never descends: the Atom
// under a Wrapped is only reached when VisitWrapped is absent.
type wrappedVisitor struct {
	wrapped []string
	atoms   []string
}

var (
	_ nt.VisitorR_Wrapped[unit, string] = (*wrappedVisitor)(nil)
	_ tree.VisitorR_Atom[unit, string]  = (*wrappedVisitor)(nil)
)

func (w *wrappedVisitor) VisitWrapped(node *nt.Wrapped) string {
	w.wrapped = append(w.wrapped, node.Name)
	return "wrapped:" + node.Name
}

func (w *wrappedVisitor) VisitAtom(node *tree.Atom) string {
	w.atoms = append(w.atoms, node.Name)
	return node.Name
}

// tracer intercepts every M3 decl with a P-variant Visit method that
// records a line and descends with an extra indent; the trace of a
// Holder pins which fields its DefaultAccept visits and in what order.
//
// A visitor can have only one VisitBox, here for Box[tree.Atom]. On
// Box[string] none of the eight probes match, and TypeCheckMethod
// recognises the method as one for another instantiation of the same
// generic decl rather than a P/R mismatch
// (TestTypeCheckMethodGenericInstantiation), so Box[string] falls
// through to DefaultAccept and the trace shows its tag without a "box"
// line.
type tracer struct {
	lines []string
}

var (
	_ gen.VisitorP_Box[tree.Atom, string, unit] = (*tracer)(nil)
	_ gen.VisitorP_Opt[tree.Atom, string, unit] = (*tracer)(nil)
	_ nt.VisitorP_Holder[string, unit]          = (*tracer)(nil)
	_ nt.VisitorP_Path[string, unit]            = (*tracer)(nil)
	_ nt.VisitorP_Wrapped[string, unit]         = (*tracer)(nil)
	_ nt.VisitorP_Id[string, unit]              = (*tracer)(nil)
	_ nt.VisitorP_Ids[tree.Atom, string, unit]  = (*tracer)(nil)
	_ tree.VisitorP_Atom[string, unit]          = (*tracer)(nil)
)

func (tr *tracer) line(indent, text string) string {
	tr.lines = append(tr.lines, indent+text)
	return indent + "  "
}

func (tr *tracer) VisitHolder(node *nt.Holder, indent string) {
	_, _ = node.DefaultAccept[string, unit](tr, tr.line(indent, "holder"))
}

func (tr *tracer) VisitBox(node *gen.Box[tree.Atom], indent string) {
	_, _ = node.DefaultAccept[string, unit](tr, tr.line(indent, "box"))
}

func (tr *tracer) VisitOpt(node *gen.Opt[tree.Atom], indent string) {
	_, _ = node.DefaultAccept[string, unit](tr, tr.line(indent, "opt"))
}

func (tr *tracer) VisitPath(node *nt.Path, indent string) {
	_, _ = node.DefaultAccept[string, unit](tr, tr.line(indent, fmt.Sprintf("path(%d)", len(*node))))
}

func (tr *tracer) VisitWrapped(node *nt.Wrapped, indent string) {
	_, _ = node.DefaultAccept[string, unit](tr, tr.line(indent, "wrapped"))
}

func (tr *tracer) VisitId(node *nt.Id, indent string) {
	_, _ = node.DefaultAccept[string, unit](tr, tr.line(indent, "id "+string(*node)))
}

func (tr *tracer) VisitIds(node *nt.Ids[tree.Atom], indent string) {
	_, _ = node.DefaultAccept[string, unit](tr, tr.line(indent, fmt.Sprintf("ids(%d)", len(*node))))
}

func (tr *tracer) VisitAtom(node *tree.Atom, indent string) {
	tr.line(indent, "atom "+node.Name)
}

// traced is the exact trace of tracer over newHolder.
var traced = []string{
	"holder",
	"  box",
	"    atom tag",
	"  atom stag", // Box[string]: VisitBox skipped, DefaultAccept
	"  opt",
	"    atom x1",
	"    atom x2",
	"  path(2)",
	"    atom p1",
	"    atom p2",
	"  wrapped",
	"    atom w",
	"  id id",
	"  path(1)",
	"    atom pp1",
	"  path(0)",
	"  path(2)",
	"    atom pp2",
	"    atom pp3",
	"  wrapped",
	"    atom nw",
	"  ids(1)",
}

// wrongBox has a VisitBox for Box[tree.Atom] whose payload type is
// string. Used with [tree.Atom, int, int] none of the eight probes match
// and TypeCheckMethod must panic; used with [tree.Atom, string, int] it
// is the PR variant.
type wrongBox struct {
	calls int
}

func (w *wrongBox) VisitBox(node *gen.Box[tree.Atom], payload string) int {
	w.calls++
	return 7
}

// ---------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------

// TestBoxTypeParamFieldsAreLeaves pins the M3 limitation: Box<T>'s item
// and items fields are leaves because their type is the type parameter,
// so even Box<Atom> never descends into the Atoms they hold. Only tag
// is reached, for Box<Atom> and Box<String> alike.
func TestBoxTypeParamFieldsAreLeaves(t *testing.T) {
	var (
		box    = newBox()
		sbox   = newSBox()
		lister atomLister
		names  atomName
		s      string
		err    error
	)
	if _, err = box.Accept[unit, unit](&lister, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "Box<Atom>: only tag reached, not item/items", lister.seen, []string{"tag"})

	if _, err = sbox.Accept[unit, unit](&lister, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "Box<String> walks the same way", lister.seen, []string{"tag", "stag"})

	// The per-field helpers: item and items return the zero R and visit
	// nothing; tag returns the atom's result.
	var leaves atomLister
	if s, err = box.AcceptItem[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "AcceptItem is a leaf", s, "")
	if s, err = box.AcceptItems[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "AcceptItems is a leaf", s, "")
	if _, err = box.AcceptItem[unit, unit](&leaves, unit{}); err != nil {
		t.Fatal(err)
	}
	if _, err = box.AcceptItems[unit, unit](&leaves, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "item/items visit nothing", len(leaves.seen), 0)
	if s, err = box.AcceptTag[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "AcceptTag descends", s, "tag")

	// DefaultAccept's result is tag's, the only descended field.
	if s, err = box.DefaultAccept[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "DefaultAccept result", s, "tag")
	if s, err = sbox.Accept[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "Box<String> result", s, "stag")

	// An error from tag comes out of the Box.
	var atTag = stopAtAtom{
		stop: "tag",
	}
	if _, err = box.Accept[unit, unit](&atTag, unit{}); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
	eq(t, "stops at tag", atTag.seen, []string{"tag"})
}

// TestVisitBoxDispatch: a VisitBox instantiated [tree.Atom, int, int] is
// dispatched to for a Box[tree.Atom] with (int, int), and the payload and
// result flow through DefaultAccept to the tag.
func TestVisitBoxDispatch(t *testing.T) {
	var (
		box = newBox()
		c   = newBoxCounter()
		n   int
		err error
	)
	if n, err = box.Accept[int, int](c, 3); err != nil {
		t.Fatal(err)
	}
	eq(t, "boxes", c.boxes, 1)
	eq(t, "atoms", c.atoms, 1)
	eq(t, "tag depth", c.depths, map[string]int{
		"tag": 4,
	})
	eq(t, "result = 1 + tag's", n, 2)
}

// TestGenericUnionBranches: Opt<Atom>'s some (T) and none (Void)
// branches are leaves, xs (Vector<Atom>) is descended, and a VisitOpt
// instantiated [tree.Atom, unit, string] intercepts every one.
func TestGenericUnionBranches(t *testing.T) {
	var (
		some = gen.Make_Opt_some[tree.Atom](atom("some"))
		none = gen.Make_Opt_none[tree.Atom]()
		xs   = gen.Make_Opt_xs[tree.Atom]([]tree.Atom{
			atom("x1"),
			atom("x2"),
		})
		lister atomLister
		names  atomName
		opts   optLister
		s      string
		err    error
	)

	// Without VisitOpt: DefaultAccept.
	if s, err = some.Accept[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "some is a leaf", s, "")
	if s, err = none.Accept[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "none is a leaf", s, "")
	if s, err = xs.Accept[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "xs result is its last atom's", s, "x2")
	for _, o := range []gen.Opt[tree.Atom]{some, none, xs} {
		if _, err = o.Accept[unit, unit](&lister, unit{}); err != nil {
			t.Fatal(err)
		}
	}
	eq(t, "only xs's atoms reached", lister.seen, []string{"x1", "x2"})

	// With VisitOpt: intercepted before the branch.
	for i, want := range []string{"Some:", "None:", "Xs:x2"} {
		if s, err = []gen.Opt[tree.Atom]{some, none, xs}[i].Accept[unit, string](&opts, unit{}); err != nil {
			t.Fatal(err)
		}
		eq(t, fmt.Sprintf("opt %d result", i), s, want)
	}
	eq(t, "every Opt intercepted", opts.opts, []string{"Some", "None", "Xs"})
	eq(t, "atoms", opts.atoms, []string{"x1", "x2"})

	// An error inside xs comes out of the Opt.
	var atX1 = stopAtAtom{
		stop: "x1",
	}
	if _, err = xs.Accept[unit, unit](&atX1, unit{}); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
	eq(t, "stops at x1", atX1.seen, []string{"x1"})
}

// TestRenamedPayloadResultParams: Weird<P, R> keeps its own P and R as
// the first two params of its interfaces and the methods take [P2, R2].
// The compile-time assertions above pin the spelling; here the PR
// variant is dispatched with (bool, int) and the p and r fields are
// leaves.
func TestRenamedPayloadResultParams(t *testing.T) {
	var (
		w = gen.MakeAll_Weird[int, string](
			5,
			"r",
			atom("wtag"),
		)
		wv     weirdVisitor
		lister atomLister
		names  atomName
		n      int
		s      string
		err    error
	)
	if n, err = w.Accept[bool, int](&wv, true); err != nil {
		t.Fatal(err)
	}
	eq(t, "VisitWeird payload", wv.payloads, []bool{true})
	eq(t, "tag reached with the flipped payload", wv.atoms, []string{"wtag"})
	// tag's result with payload false is 10, plus node.P.
	eq(t, "result", n, 15)

	// Without VisitWeird: p and r are leaves, tag is descended.
	if _, err = w.Accept[unit, unit](&lister, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "only tag reached", lister.seen, []string{"wtag"})
	if s, err = w.AcceptP[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "AcceptP is a leaf", s, "")
	if s, err = w.AcceptR[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "AcceptR is a leaf", s, "")
	if s, err = w.DefaultAccept[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "DefaultAccept result is tag's", s, "wtag")
}

// TestNewtypePath: Path = Vector<Atom> visits its elements in order
// through DefaultAccept, a VisitPath intercepts it, and an error from an
// element or from VisitPath itself propagates.
func TestNewtypePath(t *testing.T) {
	var (
		path   = newPath("p1", "p2", "p3")
		empty  = newPath()
		lister atomLister
		names  atomName
		s      string
		err    error
	)
	if _, err = path.Accept[unit, unit](&lister, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "elements in order", lister.seen, []string{"p1", "p2", "p3"})
	if s, err = path.Accept[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "result is the last element's", s, "p3")
	if s, err = empty.Accept[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "empty path result", s, "")

	// Interception.
	var closed = pathGate{
		descend: false,
	}
	if _, err = path.Accept[unit, unit](&closed, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "VisitPath called", closed.paths, 1)
	eq(t, "no atoms when VisitPath does not descend", len(closed.atoms), 0)
	var open = pathGate{
		descend: true,
	}
	if _, err = path.Accept[unit, unit](&open, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "atoms when VisitPath descends", open.atoms, []string{"p1", "p2", "p3"})

	// Errors: from an element, through the Path's DefaultAccept.
	var atP2 = stopAtAtom{
		stop: "p2",
	}
	if _, err = path.Accept[unit, unit](&atP2, unit{}); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
	eq(t, "stops after p2", atP2.seen, []string{"p1", "p2"})

	// From VisitPath itself, out of a Holder's Vector<Path>: paths[0] has
	// one element, so the walk stops there after the atoms before it.
	var (
		holder = newHolder()
		at1    = pathFail{
			stop: 1,
		}
	)
	if _, err = holder.Accept[int, int](&at1, 0); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
	// tag, stag, x1, x2, p1, p2, w: path (2 elements) descended, paths[0]
	// (1 element) failed before its atom.
	eq(t, "atoms before the failing path", at1.atoms, 7)
}

// TestNewtypeWrapped: Wrapped = Atom across modules. A VisitWrapped
// intercepts; without one, DefaultAccept converts to *tree.Atom and
// VisitAtom is reached.
func TestNewtypeWrapped(t *testing.T) {
	var (
		w      = nt.Wrapped(atom("w"))
		wv     wrappedVisitor
		lister atomLister
		names  atomName
		s      string
		err    error
	)
	if s, err = w.Accept[unit, string](&wv, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "VisitWrapped result", s, "wrapped:w")
	eq(t, "VisitWrapped called", wv.wrapped, []string{"w"})
	eq(t, "atom not reached (VisitWrapped did not descend)", len(wv.atoms), 0)

	// DefaultAccept from VisitWrapped's owner still reaches the atom.
	if s, err = w.DefaultAccept[unit, string](&wv, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "DefaultAccept result is the atom's", s, "w")
	eq(t, "atom reached", wv.atoms, []string{"w"})

	// Fall-through.
	if _, err = w.Accept[unit, unit](&lister, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "VisitAtom reached via the conversion", lister.seen, []string{"w"})
	if s, err = w.Accept[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "result is the atom's", s, "w")

	// Error from the atom comes out of the Wrapped.
	var atW = stopAtAtom{
		stop: "w",
	}
	if _, err = w.Accept[unit, unit](&atW, unit{}); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
}

// TestNewtypeLeaves: Id = String and Ids<Atom> (Vector<T>, a type-param
// element) are generated but all-leaf: Accept and DefaultAccept visit
// nothing and return the zero R, and a VisitId / VisitIds intercepts.
func TestNewtypeLeaves(t *testing.T) {
	var (
		id  = nt.Id("id")
		ids = nt.Ids[tree.Atom]{
			atom("ids1"),
			atom("ids2"),
		}
		lister atomLister
		names  atomName
		tr     tracer
		s      string
		err    error
	)
	if s, err = id.Accept[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "Id Accept result", s, "")
	if s, err = id.DefaultAccept[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "Id DefaultAccept result", s, "")
	if s, err = ids.Accept[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "Ids<Atom> Accept result", s, "")
	if s, err = ids.DefaultAccept[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "Ids<Atom> DefaultAccept result", s, "")
	if _, err = id.Accept[unit, unit](&lister, unit{}); err != nil {
		t.Fatal(err)
	}
	if _, err = ids.Accept[unit, unit](&lister, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "nothing visited", len(lister.seen), 0)

	if _, err = id.Accept[string, unit](&tr, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = ids.Accept[string, unit](&tr, ""); err != nil {
		t.Fatal(err)
	}
	eq(t, "VisitId and VisitIds intercept", tr.lines, []string{"id id", "ids(2)"})
}

// TestHolderDefaultAcceptOrder: Holder's DefaultAccept visits box, sbox,
// opt, path, wrapped, id, paths, nullWrapped and ids in declaration
// order and skips maybe (a bundle generic, a leaf). The VisitAtom-only
// walk pins the atoms reached; the tracer pins every node reached.
func TestHolderDefaultAcceptOrder(t *testing.T) {
	var (
		holder = newHolder()
		lister atomLister
		names  atomName
		tr     tracer
		s      string
		err    error
	)
	if _, err = holder.Accept[unit, unit](&lister, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "atoms in walk order", lister.seen, holderAtoms)

	if _, err = holder.Accept[string, unit](&tr, ""); err != nil {
		t.Fatal(err)
	}
	eq(t, "trace", tr.lines, traced)

	// The result is the last descended field's: ids, which is all-leaf,
	// so "" even though nullWrapped produced "nw" just before it.
	if s, err = holder.Accept[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "result is ids' (zero)", s, "")
	if s, err = holder.AcceptNullWrapped[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "nullWrapped result", s, "nw")
	if s, err = holder.AcceptMaybe[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "maybe is a leaf", s, "")

	// A nil Nullable<Wrapped> is skipped and an empty Vector<Path> visits
	// nothing.
	holder.NullWrapped = nil
	holder.Paths = nil
	var fewer atomLister
	if _, err = holder.Accept[unit, unit](&fewer, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "nil nullWrapped and empty paths skipped", fewer.seen, holderAtoms[:7])
}

// TestTypeCheckMethodMismatchedVisitBox: a VisitBox with the wrong P
// matches no probe and TypeCheckMethod panics naming VisitBox; the same
// method with the matching [T, P, R] is dispatched.
func TestTypeCheckMethodMismatchedVisitBox(t *testing.T) {
	var (
		box = newBox()
		w   wrongBox
		n   int
		err error
	)
	eq(t, "wrong payload panics", panics(func() {
		_, _ = box.Accept[int, int](&w, 0)
	}), "VisitBox")
	eq(t, "not called", w.calls, 0)

	if n, err = box.Accept[string, int](&w, "x"); err != nil {
		t.Fatal(err)
	}
	eq(t, "matching (P, R) dispatches", []int{n, w.calls}, []int{7, 1})
}

// TestTypeCheckMethodGenericInstantiation: a VisitBox for Box[tree.Atom]
// is a method named VisitBox that matches none of Box[string]'s probes.
// TypeCheckMethod is instantiation-aware: it sees that the method's node
// parameter is another instantiation of the same generic decl and does
// not panic, so a Box[string] silently falls through to DefaultAccept
// (TestHolderDefaultAcceptOrder relies on this for tracer). A genuine
// P/R mismatch on the same instantiation still panics.
func TestTypeCheckMethodGenericInstantiation(t *testing.T) {
	var (
		sbox = newSBox()
		box  = newBox()
		c    = newBoxCounter()
		w    wrongBox
		n    int
		err  error
	)
	eq(t, "other instantiation does not panic", panics(func() {
		n, err = sbox.Accept[int, int](c, 0)
	}), "")
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "VisitBox not called", c.boxes, 0)
	eq(t, "falls through to DefaultAccept: tag visited at the same depth", c.depths, map[string]int{
		"stag": 0,
	})
	eq(t, "result is tag's", n, 1)

	var tr tracer
	if _, err = sbox.Accept[string, unit](&tr, ""); err != nil {
		t.Fatal(err)
	}
	eq(t, "tracer falls through without a box line", tr.lines, []string{"atom stag"})

	// The matching instantiation is dispatched as before.
	if n, err = box.Accept[int, int](c, 0); err != nil {
		t.Fatal(err)
	}
	eq(t, "Box[tree.Atom] still dispatched", []int{n, c.boxes}, []int{2, 1})

	// And a mismatched (P, R) on Box[tree.Atom] itself still panics.
	eq(t, "same instantiation, wrong payload panics", panics(func() {
		_, _ = box.Accept[int, int](&w, 0)
	}), "VisitBox")
	eq(t, "not called", w.calls, 0)
}

// panics runs f and returns "VisitBox" if it panicked with a message
// naming that method, the whole message for any other panic, or "" if f
// returned normally.
func panics(f func()) (msg string) {
	defer func() {
		var rec = recover()
		if rec == nil {
			return
		}
		msg = fmt.Sprint(rec)
		if strings.Contains(msg, "VisitBox") {
			msg = "VisitBox"
		}
	}()
	f()
	return ""
}
