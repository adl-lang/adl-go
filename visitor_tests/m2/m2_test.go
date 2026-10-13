// M2 behavioural tests for the govisitor sub-task: non-generic unions
// (plan4.md section 4.4 and 7). They compile the generated
// generated/m2/{tree,extra} packages and run hand-written visitors over
// hand-built values; nothing here compares against golden files.
//
// m2.tree is sheafdb's tree.adl: Node, Path and Part are unions whose
// branches are structs (or, for Part.index and SeqSpec.nums, leaves).
// m2.extra adds the branch shapes tree.adl lacks: Void, Vector, Nullable,
// StringMap and String, all over the cross-module tree.Atom, and a struct
// with Vector<Union> and Nullable<Union> fields.
package m2_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"adl_visitor_tests/generated/m2/extra"
	"adl_visitor_tests/generated/m2/tree"
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

func quoted(name string) tree.Atom {
	return tree.MakeAll_Atom(name, true)
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

// newAxis uses every union branch of m2.tree at least once:
//
//	Node.atom, Node.ref, Node.seq, Node.hierachy
//	Path.canonical, Path.short
//	Part.atom, Part.index
//	SeqSpec.nums
//
// and reaches atoms through Node.atom, Hierachy.node, Hierachy.kids,
// ShortPath.anchor and Part.atom.
func newAxis() tree.Axis {
	return tree.MakeAll_Axis(
		[]tree.Node{
			tree.Make_Node_atom(atom("a")),
			tree.Make_Node_ref(tree.MakeAll_Ref(
				tree.Make_Path_canonical(tree.MakeAll_CanonicalPath(
					[]tree.Part{
						tree.Make_Part_atom(atom("c1")),
						tree.Make_Part_index(7),
					},
				)),
			)),
			tree.Make_Node_seq(tree.MakeAll_Seq(
				tree.Make_SeqSpec_nums(types.MakeAll_Pair[int64, int64](1, 2)),
			)),
			tree.Make_Node_hierachy(tree.MakeAll_Hierachy(
				atom("h"),
				[]tree.Node{
					tree.Make_Node_atom(quoted("k1")),
					tree.Make_Node_ref(tree.MakeAll_Ref(
						tree.Make_Path_short(tree.MakeAll_ShortPath(
							atom("s"),
							[]tree.Part{
								tree.Make_Part_index(3),
								tree.Make_Part_atom(atom("s1")),
							},
						)),
					)),
					tree.Make_Node_hierachy(tree.MakeAll_Hierachy(
						atom("hh"),
						nil,
					)),
				},
			)),
		},
	)
}

// allAtoms is the order a full walk of newAxis reaches every Atom.
var allAtoms = []string{
	"a",  // nodes[0]: Node.atom
	"c1", // nodes[1]: Node.ref -> Path.canonical -> parts[0] Part.atom
	"h",  // nodes[3]: Node.hierachy -> Hierachy.node
	"k1", // kids[0]: Node.atom
	"s",  // kids[1]: Node.ref -> Path.short -> ShortPath.anchor
	"s1", // ... -> parts[1] Part.atom
	"hh", // kids[2]: Node.hierachy -> Hierachy.node
}

// newHolder uses every branch of m2.extra.Thing, inside the Vector<Thing>
// and Nullable<Thing> fields of Holder.
func newHolder() extra.Holder {
	return extra.MakeAll_Holder(
		[]extra.Thing{
			extra.Make_Thing_nothing(),
			extra.Make_Thing_atoms([]tree.Atom{
				atom("v1"),
				atom("v2"),
			}),
			extra.Make_Thing_maybe(ptr(atom("n1"))),
			extra.Make_Thing_maybe(nil),
			extra.Make_Thing_named(map[string]tree.Atom{
				"zeta":  atom("m-z"),
				"alpha": atom("m-a"),
				"mid":   atom("m-m"),
			}),
			extra.Make_Thing_text("t"),
		},
		ptr(extra.Make_Thing_atoms([]tree.Atom{
			atom("f1"),
		})),
	)
}

var holderAtoms = []string{
	"v1", "v2", // things[1]: Vector<Atom>
	"n1",                // things[2]: Nullable<Atom> set; things[3] is nil
	"m-a", "m-m", "m-z", // things[4]: StringMap<Atom>, sorted keys
	"f1", // focus: Nullable<Thing> set
}

// ---------------------------------------------------------------------
// visitors
// ---------------------------------------------------------------------

// atomLister has only a plain VisitAtom: every struct and union falls
// through DefaultAccept.
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
// union's DefaultAccept returns the name of the atom its branch holds
// and "" for a leaf branch.
type atomName struct{}

var (
	_ tree.VisitorR_Atom[unit, string] = (*atomName)(nil)
)

func (*atomName) VisitAtom(node *tree.Atom) string {
	return node.Name
}

// printer is the (any, string) visitor: the payload is the indent
// (a string carried as any), the result is the text of the last line
// printed beneath the node. It has no VisitNode and no VisitPath, so
// those unions dispatch through DefaultAccept to the branch value.
type printer struct {
	out strings.Builder
}

var (
	_ tree.VisitorPR_Axis[any, string]          = (*printer)(nil)
	_ tree.VisitorPR_Atom[any, string]          = (*printer)(nil)
	_ tree.VisitorPRE_Hierachy[any, string]     = (*printer)(nil)
	_ tree.VisitorPR_Ref[any, string]           = (*printer)(nil)
	_ tree.VisitorPRE_Seq[any, string]          = (*printer)(nil)
	_ tree.VisitorPR_CanonicalPath[any, string] = (*printer)(nil)
	_ tree.VisitorPR_ShortPath[any, string]     = (*printer)(nil)
	_ tree.VisitorPR_Part[any, string]          = (*printer)(nil)
	_ tree.VisitorPR_SeqSpec[any, string]       = (*printer)(nil)
)

// line prints text at the payload's indent and returns the indent for
// the node's children.
func (p *printer) line(payload any, text string) any {
	var (
		indent, _ = payload.(string)
	)
	fmt.Fprintf(&p.out, "%s%s\n", indent, text)
	return indent + "  "
}

func (p *printer) VisitAxis(node *tree.Axis, payload any) (result string) {
	var next = p.line(payload, "axis")
	result, _ = node.DefaultAccept[any, string](p, next)
	return
}

func (p *printer) VisitAtom(node *tree.Atom, payload any) (result string) {
	result = "atom " + node.Name
	if node.Quoted {
		result = fmt.Sprintf("atom %q", node.Name)
	}
	p.line(payload, result)
	return
}

func (p *printer) VisitHierachy(node *tree.Hierachy, payload any) (result string, err error) {
	var next = p.line(payload, "hierachy")
	if _, err = node.DefaultAccept[any, string](p, next); err != nil {
		return
	}
	return "hierachy " + node.Node.Name, nil
}

func (p *printer) VisitRef(node *tree.Ref, payload any) (result string) {
	var next = p.line(payload, "ref")
	result, _ = node.DefaultAccept[any, string](p, next)
	return
}

func (p *printer) VisitSeq(node *tree.Seq, payload any) (result string, err error) {
	var next = p.line(payload, "seq")
	return node.DefaultAccept[any, string](p, next)
}

func (p *printer) VisitCanonicalPath(node *tree.CanonicalPath, payload any) (result string) {
	var next = p.line(payload, "canonical")
	result, _ = node.DefaultAccept[any, string](p, next)
	return
}

func (p *printer) VisitShortPath(node *tree.ShortPath, payload any) (result string) {
	var next = p.line(payload, "short")
	result, _ = node.DefaultAccept[any, string](p, next)
	return
}

// VisitPart prints the leaf index branch itself and lets the atom branch
// fall through to VisitAtom.
func (p *printer) VisitPart(node *tree.Part, payload any) (result string) {
	var (
		index uint64
		ok    bool
	)
	if index, ok = node.Cast_index(); ok {
		result = fmt.Sprintf("#%d", index)
		p.line(payload, result)
		return
	}
	result, _ = node.DefaultAccept[any, string](p, payload)
	return
}

func (p *printer) VisitSeqSpec(node *tree.SeqSpec, payload any) (result string) {
	var nums, _ = node.Cast_nums()
	result = fmt.Sprintf("nums %d..%d", nums.V1, nums.V2)
	p.line(payload, result)
	return
}

// printed is the exact output of printer over newAxis.
var printed = strings.Join([]string{
	"axis",
	"  atom a",
	"  ref",
	"    canonical",
	"      atom c1",
	"      #7",
	"  seq",
	"    nums 1..2",
	"  hierachy",
	"    atom h",
	`    atom "k1"`,
	"    ref",
	"      short",
	"        atom s",
	"        #3",
	"        atom s1",
	"    hierachy",
	"      atom hh",
	"",
}, "\n")

// counter is the (int, int) visitor: P is the depth at which the node is
// visited, R is a count that flows up through DefaultAccept. It has a
// union-level VisitNode, VisitPath and VisitPart, each of which descends
// with depth+1 and adds one to the branch's result, so it exercises the
// PRE variant on unions; Ref, Seq, Hierachy, CanonicalPath and ShortPath
// fall through with the payload unchanged.
type counter struct {
	axes   int
	nodes  int
	atoms  int
	paths  int
	parts  int
	specs  int
	depths map[string][]int
}

var (
	_ tree.VisitorPRE_Axis[int, int]  = (*counter)(nil)
	_ tree.VisitorPRE_Node[int, int]  = (*counter)(nil)
	_ tree.VisitorPR_Atom[int, int]   = (*counter)(nil)
	_ tree.VisitorPRE_Path[int, int]  = (*counter)(nil)
	_ tree.VisitorPRE_Part[int, int]  = (*counter)(nil)
	_ tree.VisitorP_SeqSpec[int, int] = (*counter)(nil)
)

func newCounter() *counter {
	return &counter{
		depths: map[string][]int{},
	}
}

func (c *counter) at(k string, depth int) {
	c.depths[k] = append(c.depths[k], depth)
}

func (c *counter) VisitAxis(node *tree.Axis, depth int) (result int, err error) {
	c.axes++
	c.at("axis", depth)
	if result, err = node.DefaultAccept[int, int](c, depth+1); err != nil {
		return
	}
	return result + 1, nil
}

func (c *counter) VisitNode(node *tree.Node, depth int) (result int, err error) {
	c.nodes++
	c.at("node", depth)
	if result, err = node.DefaultAccept[int, int](c, depth+1); err != nil {
		return
	}
	return result + 1, nil
}

func (c *counter) VisitAtom(node *tree.Atom, depth int) int {
	c.atoms++
	c.at(node.Name, depth)
	return 1
}

func (c *counter) VisitPath(node *tree.Path, depth int) (result int, err error) {
	c.paths++
	c.at("path", depth)
	if result, err = node.DefaultAccept[int, int](c, depth+1); err != nil {
		return
	}
	return result + 1, nil
}

func (c *counter) VisitPart(node *tree.Part, depth int) (result int, err error) {
	c.parts++
	c.at("part", depth)
	if result, err = node.DefaultAccept[int, int](c, depth+1); err != nil {
		return
	}
	return result + 1, nil
}

func (c *counter) VisitSeqSpec(node *tree.SeqSpec, depth int) {
	c.specs++
	c.at("spec", depth)
}

// nodeGate has a union-level VisitNode and a VisitAtom. With descend
// false VisitNode never calls DefaultAccept, so no branch value is
// visited.
type nodeGate struct {
	descend bool
	nodes   int
	atoms   []string
}

var (
	_ tree.VisitorE_Node[unit, unit] = (*nodeGate)(nil)
	_ tree.Visitor_Atom[unit, unit]  = (*nodeGate)(nil)
)

func (g *nodeGate) VisitNode(node *tree.Node) (err error) {
	g.nodes++
	if g.descend {
		_, err = node.DefaultAccept[unit, unit](g, unit{})
	}
	return
}

func (g *nodeGate) VisitAtom(node *tree.Atom) {
	g.atoms = append(g.atoms, node.Name)
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

// stopAtIndex fails (PE) on the Part whose index branch holds stop, and
// counts the atoms (P) it reached; the error has to come up through Part,
// CanonicalPath/ShortPath, Path, Ref and Node.
type stopAtIndex struct {
	stop  uint64
	atoms int
}

var (
	_ tree.VisitorPE_Part[int, int] = (*stopAtIndex)(nil)
	_ tree.VisitorP_Atom[int, int]  = (*stopAtIndex)(nil)
)

func (s *stopAtIndex) VisitPart(node *tree.Part, depth int) (err error) {
	var (
		index uint64
		ok    bool
	)
	if index, ok = node.Cast_index(); ok && index == s.stop {
		return errBoom
	}
	_, err = node.DefaultAccept[int, int](s, depth+1)
	return
}

func (s *stopAtIndex) VisitAtom(node *tree.Atom, depth int) {
	s.atoms++
}

// thingLister has a union-level VisitThing on m2.extra.Thing that
// records the branch and descends.
type thingLister struct {
	things []string
	atoms  []string
}

var (
	_ extra.VisitorRE_Thing[unit, string] = (*thingLister)(nil)
	_ tree.VisitorR_Atom[unit, string]    = (*thingLister)(nil)
)

func (l *thingLister) VisitThing(node *extra.Thing) (result string, err error) {
	var kind = strings.TrimPrefix(reflect.TypeOf(node.Branch).Name(), "_Thing_")
	l.things = append(l.things, kind)
	if result, err = node.DefaultAccept[unit, string](l, unit{}); err != nil {
		return
	}
	return kind + ":" + result, nil
}

func (l *thingLister) VisitAtom(node *tree.Atom) string {
	l.atoms = append(l.atoms, node.Name)
	return node.Name
}

// wrongNode has a VisitNode whose payload type is string. Used with
// (int, int) none of the eight probes match and TypeCheckMethod must
// panic; used with (string, int) it is the PR variant.
type wrongNode struct {
	calls int
}

func (w *wrongNode) VisitNode(node *tree.Node, payload string) int {
	w.calls++
	return 7
}

// One visitor per Accept probe on the Node union, for the dispatch table
// test.
type vPlain struct{ n int }
type vE struct{ n int }
type vP struct {
	n int
	p int
}
type vPE struct {
	n int
	p int
}
type vR struct{ n int }
type vRE struct{ n int }
type vPR struct {
	n int
	p int
}
type vPRE struct {
	n int
	p int
}

var (
	_ tree.Visitor_Node[int, int]    = (*vPlain)(nil)
	_ tree.VisitorE_Node[int, int]   = (*vE)(nil)
	_ tree.VisitorP_Node[int, int]   = (*vP)(nil)
	_ tree.VisitorPE_Node[int, int]  = (*vPE)(nil)
	_ tree.VisitorR_Node[int, int]   = (*vR)(nil)
	_ tree.VisitorRE_Node[int, int]  = (*vRE)(nil)
	_ tree.VisitorPR_Node[int, int]  = (*vPR)(nil)
	_ tree.VisitorPRE_Node[int, int] = (*vPRE)(nil)
)

func (v *vPlain) VisitNode(node *tree.Node) { v.n++ }
func (v *vE) VisitNode(node *tree.Node) error {
	v.n++
	return errBoom
}
func (v *vP) VisitNode(node *tree.Node, payload int) {
	v.n++
	v.p = payload
}
func (v *vPE) VisitNode(node *tree.Node, payload int) error {
	v.n++
	v.p = payload
	return errBoom
}
func (v *vR) VisitNode(node *tree.Node) int {
	v.n++
	return 5
}
func (v *vRE) VisitNode(node *tree.Node) (int, error) {
	v.n++
	return 5, errBoom
}
func (v *vPR) VisitNode(node *tree.Node, payload int) int {
	v.n++
	v.p = payload
	return payload * 2
}
func (v *vPRE) VisitNode(node *tree.Node, payload int) (int, error) {
	v.n++
	v.p = payload
	return payload * 2, errBoom
}

// ---------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------

// TestPrinterAndCounterSameValue: the one hand-built Axis is walked by the
// (any, string) printer and then by the (int, int) counter. The printer's
// output pins the dispatch order through every union branch; the
// counter pins the depth each node is visited at and the count that
// flows back up.
func TestPrinterAndCounterSameValue(t *testing.T) {
	var (
		axis = newAxis()
		p    printer
		c    = newCounter()
		s    string
		n    int
		err  error
	)

	if s, err = axis.Accept[any, string](&p, ""); err != nil {
		t.Fatal(err)
	}
	eq(t, "printer output", p.out.String(), printed)
	// Axis's result is its last node's, a Hierachy, whose VisitHierachy
	// returns its own text.
	eq(t, "printer result", s, "hierachy h")

	if n, err = axis.Accept[int, int](c, 0); err != nil {
		t.Fatal(err)
	}
	eq(t, "axes", c.axes, 1)
	eq(t, "nodes", c.nodes, 7)
	eq(t, "atoms", c.atoms, len(allAtoms))
	eq(t, "paths", c.paths, 2)
	eq(t, "parts", c.parts, 4)
	eq(t, "specs", c.specs, 1)

	// Depths: the Visit methods that descend add one; the structs that
	// fall through pass the payload on unchanged.
	eq(t, "axis depth", c.depths["axis"], []int{0})
	eq(t, "node depths", c.depths["node"], []int{1, 1, 1, 1, 2, 2, 2})
	eq(t, "path depths", c.depths["path"], []int{2, 3})
	eq(t, "part depths", c.depths["part"], []int{3, 3, 4, 4})
	eq(t, "spec depth", c.depths["spec"], []int{2})
	eq(t, "atom a", c.depths["a"], []int{2})
	eq(t, "atom c1", c.depths["c1"], []int{4})
	eq(t, "atom h", c.depths["h"], []int{2})
	eq(t, "atom k1", c.depths["k1"], []int{3})
	eq(t, "atom s", c.depths["s"], []int{4})
	eq(t, "atom s1", c.depths["s1"], []int{5})
	eq(t, "atom hh", c.depths["hh"], []int{3})

	// Result: Axis = 1 + last node. The last node is the hierachy, whose
	// Hierachy falls through and so returns its last descended field's
	// (kids') result: the last kid is the "hh" hierachy node = 1 + (its
	// Hierachy's kids are empty, so 0) = 1. So 1 + (1 + 1) = 3.
	eq(t, "counter result", n, 3)

	// The same counter on the ref node alone: Node = 1 + Path, Path = 1 +
	// CanonicalPath's last part, the index part = 1 + leaf 0. So 3.
	if n, err = axis.Nodes[1].Accept[int, int](c, 0); err != nil {
		t.Fatal(err)
	}
	eq(t, "ref node result", n, 3)
}

// TestUnionDefaultAcceptDispatchesToBranch: a visitor with only VisitAtom
// reaches every Atom, through Node.atom, Hierachy.kids, Path.short ->
// ShortPath.anchor, Path.canonical -> CanonicalPath.parts -> Part.atom,
// because each union's DefaultAccept hands the walk to its branch value.
func TestUnionDefaultAcceptDispatchesToBranch(t *testing.T) {
	var (
		axis   = newAxis()
		lister atomLister
		r      unit
		err    error
	)
	if r, err = axis.Accept[unit, unit](&lister, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "result", r, unit{})
	eq(t, "atoms in walk order", lister.seen, allAtoms)

	// The branch value's result is the union's result.
	var (
		names atomName
		s     string
	)
	if s, err = axis.Nodes[0].Accept[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "Node.atom result", s, "a")
	if s, err = axis.Nodes[0].DefaultAccept[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "Node.atom DefaultAccept result", s, "a")
	// Node.ref -> Ref -> Path.canonical -> CanonicalPath -> parts: last
	// part is the index leaf, so "".
	if s, err = axis.Nodes[1].Accept[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "Node.ref result is the last part's (a leaf)", s, "")
	// Node.hierachy -> Hierachy{node h, kids []}: kids empty, so "".
	var lone = tree.Make_Node_hierachy(tree.MakeAll_Hierachy(atom("x"), nil))
	if s, err = lone.Accept[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "empty kids overwrite node's result", s, "")
	// Path.short -> ShortPath{anchor s, parts [#3, s1]}: last part's atom.
	if s, err = axis.Nodes[3].Accept[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "hierachy result is last kid's", s, "")
	var short, _ = axis.Nodes[3].Cast_hierachy()
	if s, err = short.Kids[1].Accept[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "Path.short result is the last part's atom", s, "s1")
}

// TestLeafBranches: Part.index (Word64), SeqSpec.nums (a bundle generic),
// Thing.nothing (Void) and Thing.text (String) are leaves: DefaultAccept
// visits nothing and returns the zero R, and so does Accept when there is
// no Visit method for the union.
func TestLeafBranches(t *testing.T) {
	var (
		names  atomName
		lister atomLister
		part   = tree.Make_Part_index(9)
		spec   = tree.Make_SeqSpec_nums(types.MakeAll_Pair[int64, int64](3, 4))
		void   = extra.Make_Thing_nothing()
		text   = extra.Make_Thing_text("t")
		s      string
		err    error
		leaves = []struct {
			name          string
			accept        func(visitor any) (string, error)
			defaultAccept func(visitor any) (string, error)
			walk          func(visitor any) error
		}{
			{
				name: "Part.index",
				accept: func(v any) (string, error) {
					return part.Accept[unit, string](v, unit{})
				},
				defaultAccept: func(v any) (string, error) {
					return part.DefaultAccept[unit, string](v, unit{})
				},
				walk: func(v any) error {
					var _, err = part.Accept[unit, unit](v, unit{})
					return err
				},
			},
			{
				name: "SeqSpec.nums",
				accept: func(v any) (string, error) {
					return spec.Accept[unit, string](v, unit{})
				},
				defaultAccept: func(v any) (string, error) {
					return spec.DefaultAccept[unit, string](v, unit{})
				},
				walk: func(v any) error {
					var _, err = spec.Accept[unit, unit](v, unit{})
					return err
				},
			},
			{
				name: "Thing.nothing",
				accept: func(v any) (string, error) {
					return void.Accept[unit, string](v, unit{})
				},
				defaultAccept: func(v any) (string, error) {
					return void.DefaultAccept[unit, string](v, unit{})
				},
				walk: func(v any) error {
					var _, err = void.Accept[unit, unit](v, unit{})
					return err
				},
			},
			{
				name: "Thing.text",
				accept: func(v any) (string, error) {
					return text.Accept[unit, string](v, unit{})
				},
				defaultAccept: func(v any) (string, error) {
					return text.DefaultAccept[unit, string](v, unit{})
				},
				walk: func(v any) error {
					var _, err = text.Accept[unit, unit](v, unit{})
					return err
				},
			},
		}
	)
	for _, leaf := range leaves {
		if s, err = leaf.accept(&names); err != nil {
			t.Fatal(err)
		}
		eq(t, leaf.name+" Accept result", s, "")
		if s, err = leaf.defaultAccept(&names); err != nil {
			t.Fatal(err)
		}
		eq(t, leaf.name+" DefaultAccept result", s, "")
		if err = leaf.walk(&lister); err != nil {
			t.Fatal(err)
		}
	}
	eq(t, "nothing visited", len(lister.seen), 0)

	// By contrast the atom branch of Part is descended.
	var atomPart = tree.Make_Part_atom(atom("z"))
	if s, err = atomPart.Accept[unit, string](&names, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "Part.atom result", s, "z")
}

// TestVisitNodeIntercepts: a union-level VisitNode is dispatched to
// before DefaultAccept, so the branch value is only visited if VisitNode
// descends.
func TestVisitNodeIntercepts(t *testing.T) {
	var (
		axis = newAxis()
		err  error
	)

	var closed = nodeGate{
		descend: false,
	}
	if _, err = axis.Accept[unit, unit](&closed, unit{}); err != nil {
		t.Fatal(err)
	}
	// Only the four top-level nodes: the kids sit inside a Hierachy that
	// was never descended into.
	eq(t, "nodes seen", closed.nodes, 4)
	eq(t, "no atoms", len(closed.atoms), 0)

	var open = nodeGate{
		descend: true,
	}
	if _, err = axis.Accept[unit, unit](&open, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "nodes seen", open.nodes, 7)
	eq(t, "atoms seen", open.atoms, allAtoms)
}

// TestErrorPropagatesThroughUnion: an error from a branch value comes
// out of the union's Accept unchanged and stops the walk.
func TestErrorPropagatesThroughUnion(t *testing.T) {
	var (
		axis = newAxis()
		err  error
	)

	// Failing on an atom reached through Node.ref -> Path.canonical ->
	// Part.atom.
	var atC1 = stopAtAtom{
		stop: "c1",
	}
	if _, err = axis.Accept[unit, unit](&atC1, unit{}); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
	eq(t, "stops after c1", atC1.seen, allAtoms[:2])

	// Failing on the ShortPath anchor, inside a kid of a Hierachy.
	var atS = stopAtAtom{
		stop: "s",
	}
	if _, err = axis.Accept[unit, unit](&atS, unit{}); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
	eq(t, "stops after s", atS.seen, allAtoms[:5])

	// Never failing: the walk completes.
	var never = stopAtAtom{
		stop: "-",
	}
	if _, err = axis.Accept[unit, unit](&never, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "every atom seen", never.seen, allAtoms)

	// Failing on the union itself (Part #3): the atoms before it (a, c1,
	// h, k1, s) were reached; s1 and hh were not.
	var at3 = stopAtIndex{
		stop: 3,
	}
	if _, err = axis.Accept[int, int](&at3, 0); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
	eq(t, "atoms before #3", at3.atoms, 5)

	// Failing on Part #7 stops inside the second top-level node.
	var at7 = stopAtIndex{
		stop: 7,
	}
	if _, err = axis.Accept[int, int](&at7, 0); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
	eq(t, "atoms before #7", at7.atoms, 2)
}

// TestUnionContainerFields: Holder's Vector<Thing> and Nullable<Thing>
// are descended, and inside Thing the Vector<Atom>, Nullable<Atom> and
// StringMap<Atom> branches are descended element by element (sorted keys
// for the map, nil skipped), while the Void and String branches are
// leaves. The atoms are m2.tree's, reached from m2.extra.
func TestUnionContainerFields(t *testing.T) {
	var (
		holder = newHolder()
		lister atomLister
		things thingLister
		s      string
		err    error
	)
	if _, err = holder.Accept[unit, unit](&lister, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "atoms through Vector<Thing> and Nullable<Thing>", lister.seen, holderAtoms)

	if s, err = holder.Accept[unit, string](&things, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "every Thing visited, in order", things.things, []string{
		"Nothing", "Atoms", "Maybe", "Maybe", "Named", "Text", "Atoms",
	})
	eq(t, "atoms", things.atoms, holderAtoms)
	// Holder's result is its last descended field's: focus, an Atoms
	// branch whose result is its last atom's.
	eq(t, "result", s, "Atoms:f1")

	// The per-branch results: a leaf branch is "", a container branch's is
	// its last element's.
	for i, want := range []string{"Nothing:", "Atoms:v2", "Maybe:n1", "Maybe:", "Named:m-z", "Text:"} {
		if s, err = holder.Things[i].Accept[unit, string](&things, unit{}); err != nil {
			t.Fatal(err)
		}
		eq(t, fmt.Sprintf("things[%d] result", i), s, want)
	}

	// StringMap order is sorted regardless of insertion order.
	for i := 0; i < 20; i++ {
		var named atomLister
		if _, err = holder.Things[4].Accept[unit, unit](&named, unit{}); err != nil {
			t.Fatal(err)
		}
		eq(t, "sorted keys", named.seen, []string{"m-a", "m-m", "m-z"})
	}

	// A nil Nullable<Thing> is skipped, and an empty Vector<Thing> visits
	// nothing.
	holder.Focus = nil
	var noFocus atomLister
	if _, err = holder.Accept[unit, unit](&noFocus, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "nil focus skipped", noFocus.seen, holderAtoms[:6])
	holder.Things = nil
	var none atomLister
	if _, err = holder.Accept[unit, unit](&none, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "nothing", len(none.seen), 0)

	// An error inside a container branch comes out through the Thing.
	var (
		fresh = newHolder()
		atN1  = stopAtAtom{
			stop: "n1",
		}
	)
	if _, err = fresh.Accept[unit, unit](&atN1, unit{}); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
	eq(t, "stops after n1", atN1.seen, holderAtoms[:3])
}

// TestUnionAcceptDispatchesEachVariant: Node.Accept picks the one probe a
// visitor satisfies, exactly as a struct's does.
func TestUnionAcceptDispatchesEachVariant(t *testing.T) {
	var (
		n   = tree.Make_Node_atom(atom("a"))
		r   int
		err error
	)

	var plain vPlain
	r, err = n.Accept[int, int](&plain, 9)
	eq(t, "plain", []any{plain.n, r, err}, []any{1, 0, error(nil)})

	var e vE
	r, err = n.Accept[int, int](&e, 9)
	eq(t, "E", []any{e.n, r, err}, []any{1, 0, errBoom})

	var pp vP
	r, err = n.Accept[int, int](&pp, 9)
	eq(t, "P", []any{pp.n, pp.p, r, err}, []any{1, 9, 0, error(nil)})

	var pe vPE
	r, err = n.Accept[int, int](&pe, 9)
	eq(t, "PE", []any{pe.n, pe.p, r, err}, []any{1, 9, 0, errBoom})

	var rr vR
	r, err = n.Accept[int, int](&rr, 9)
	eq(t, "R", []any{rr.n, r, err}, []any{1, 5, error(nil)})

	var re vRE
	r, err = n.Accept[int, int](&re, 9)
	eq(t, "RE", []any{re.n, r, err}, []any{1, 5, errBoom})

	var pr vPR
	r, err = n.Accept[int, int](&pr, 9)
	eq(t, "PR", []any{pr.n, pr.p, r, err}, []any{1, 9, 18, error(nil)})

	var pre vPRE
	r, err = n.Accept[int, int](&pre, 9)
	eq(t, "PRE", []any{pre.n, pre.p, r, err}, []any{1, 9, 18, errBoom})
}

// TestTypeCheckMethodUnion: a VisitNode with the wrong P matches no probe
// and TypeCheckMethod panics naming VisitNode; the same method with the
// matching (P, R) is dispatched.
func TestTypeCheckMethodUnion(t *testing.T) {
	var (
		n   = tree.Make_Node_atom(atom("a"))
		w   wrongNode
		r   int
		err error
	)
	eq(t, "wrong payload panics", panics(func() {
		_, _ = n.Accept[int, int](&w, 0)
	}), "VisitNode")
	eq(t, "not called", w.calls, 0)

	if r, err = n.Accept[string, int](&w, "x"); err != nil {
		t.Fatal(err)
	}
	eq(t, "matching (P, R) dispatches", []int{r, w.calls}, []int{7, 1})
}

// panics runs f and returns "VisitNode" if it panicked with a message
// naming that method, the whole message for any other panic, or "" if f
// returned normally.
func panics(f func()) (msg string) {
	defer func() {
		var rec = recover()
		if rec == nil {
			return
		}
		msg = fmt.Sprint(rec)
		if strings.Contains(msg, "VisitNode") {
			msg = "VisitNode"
		}
	}()
	f()
	return ""
}
