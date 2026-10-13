// M1 behavioural tests for the govisitor sub-task: non-generic structs
// (plan4.md sections 4.2, 4.3 and 7). They compile the generated
// generated/m1/{other,shapes} packages and run hand-written visitors over
// hand-built values; nothing here compares against golden files.
package m1_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"adl_visitor_tests/generated/m1/other"
	"adl_visitor_tests/generated/m1/shapes"
	"github.com/adl-lang/adl-go/adl/sys/types"
	"github.com/adl-lang/adl-go/adl/visit"
)

// unit is the (P, R) pair for visitors that carry no payload and return
// no result.
type unit = struct{}

var errBoom = errors.New("boom")

// ---------------------------------------------------------------------
// fixtures
// ---------------------------------------------------------------------

func pt(x, y int32) shapes.Point {
	return shapes.MakeAll_Point(x, y)
}

func ptr[T any](v T) *T {
	return &v
}

func key(p *shapes.Point) string {
	return fmt.Sprintf("(%d,%d)", p.X, p.Y)
}

func eq(t *testing.T, what string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: got %#v, want %#v", what, got, want)
	}
}

// newTri exercises every field kind: alias, Vector, Nullable (set),
// StringMap (unsorted insertion order), Vector<Nullable>, Vector<Vector>,
// and the leaves (String, Pair, union, Vector<String>) plus Empty and the
// cross-module Tag.
func newTri() shapes.Shape {
	return shapes.MakeAll_Shape(
		"tri",
		pt(0, 0),
		[]shapes.Point{
			pt(1, 1),
			pt(2, 2),
			pt(3, 3),
		},
		ptr(pt(4, 4)),
		map[string]shapes.Point{
			"zeta":  pt(5, 5),
			"alpha": pt(6, 6),
			"mid":   pt(7, 7),
		},
		[]*shapes.Point{
			nil,
			ptr(pt(8, 8)),
			nil,
		},
		[][]shapes.Point{
			{
				pt(9, 9),
				pt(10, 10),
			},
			{
				pt(11, 11),
			},
		},
		types.MakeAll_Pair[int64, int64](1, 2),
		shapes.Make_Colour_red(),
		[]string{
			"a",
			"b",
		},
		shapes.MakeAll_Empty(),
		other.MakeAll_Tag("t0"),
	)
}

// newDot has every optional/collection field empty or nil.
func newDot() shapes.Shape {
	return shapes.MakeAll_Shape(
		"dot",
		pt(20, 20),
		nil,
		nil,
		nil,
		nil,
		[][]shapes.Point{
			{},
		},
		types.MakeAll_Pair[int64, int64](3, 4),
		shapes.Make_Colour_custom("teal"),
		nil,
		shapes.MakeAll_Empty(),
		other.MakeAll_Tag("t1"),
	)
}

func newFoc() shapes.Shape {
	return shapes.MakeAll_Shape(
		"foc",
		pt(30, 30),
		[]shapes.Point{
			pt(31, 31),
		},
		nil,
		nil,
		nil,
		nil,
		types.MakeAll_Pair[int64, int64](5, 6),
		shapes.Make_Colour_green(),
		nil,
		shapes.MakeAll_Empty(),
		other.MakeAll_Tag("t2"),
	)
}

func newDiagonal() shapes.Segment {
	return shapes.MakeAll_Segment(pt(40, 40), pt(41, 41))
}

// newScene nests three levels deep: Scene -> Shape -> Point.
func newScene() shapes.Scene {
	return shapes.MakeAll_Scene(
		[]shapes.Shape{
			newTri(),
			newDot(),
		},
		ptr(newFoc()),
		newDiagonal(),
	)
}

// newSmallScene is newScene without the Vector<Shape>, for tests that
// assert exact printed output.
func newSmallScene() shapes.Scene {
	return shapes.MakeAll_Scene(
		nil,
		ptr(newFoc()),
		newDiagonal(),
	)
}

// allPoints is the order a full walk of newScene reaches every Point:
// declaration order of fields, index order in Vectors, sorted key order
// in StringMaps, nil Nullables skipped.
var allPoints = []string{
	// shapes[0] "tri"
	"(0,0)",                   // origin: alias to Point, descended
	"(1,1)", "(2,2)", "(3,3)", // outline
	"(4,4)",                   // pivot
	"(6,6)", "(7,7)", "(5,5)", // named: keys alpha, mid, zeta
	"(8,8)",                       // sparse: only the non-nil element
	"(9,9)", "(10,10)", "(11,11)", // rings
	// shapes[1] "dot"
	"(20,20)",
	// focus "foc"
	"(30,30)", "(31,31)",
	// diagonal
	"(40,40)", "(41,41)",
}

// ---------------------------------------------------------------------
// visitors
// ---------------------------------------------------------------------

// pointLister has only a plain VisitPoint: every other node falls through
// DefaultAccept.
type pointLister struct {
	seen []string
}

func (l *pointLister) VisitPoint(node *shapes.Point) {
	l.seen = append(l.seen, key(node))
}

// tagLister has only a plain VisitTag, on the other module's struct.
type tagLister struct {
	seen []string
}

func (l *tagLister) VisitTag(node *other.Tag) {
	l.seen = append(l.seen, node.Label)
}

// counter is the depth-payload / count-result visitor. P is the depth at
// which the node is visited, R is a count that flows up through
// DefaultAccept (and so, by the last-field rule, is the last descended
// field's count plus one for the node itself).
//
// It mixes variants: PRE on Scene and Shape, PR on Segment and Point, and
// P on Tag (which therefore contributes a zero result).
type counter struct {
	scenes   int
	shapes   int
	segments int
	points   int
	tags     int
	depths   map[string][]int
}

var (
	_ shapes.VisitorPRE_Scene[int, int]  = (*counter)(nil)
	_ shapes.VisitorPRE_Shape[int, int]  = (*counter)(nil)
	_ shapes.VisitorPR_Segment[int, int] = (*counter)(nil)
	_ shapes.VisitorPR_Point[int, int]   = (*counter)(nil)
	_ other.VisitorP_Tag[int, int]       = (*counter)(nil)
)

func newCounter() *counter {
	return &counter{
		depths: map[string][]int{},
	}
}

func (c *counter) at(k string, depth int) {
	c.depths[k] = append(c.depths[k], depth)
}

func (c *counter) VisitScene(node *shapes.Scene, depth int) (result int, err error) {
	c.scenes++
	c.at("scene", depth)
	if result, err = node.DefaultAccept[int, int](c, depth+1); err != nil {
		return
	}
	return result + 1, nil
}

func (c *counter) VisitShape(node *shapes.Shape, depth int) (result int, err error) {
	c.shapes++
	c.at("shape "+node.Name, depth)
	if result, err = node.DefaultAccept[int, int](c, depth+1); err != nil {
		return
	}
	return result + 1, nil
}

func (c *counter) VisitSegment(node *shapes.Segment, depth int) (result int) {
	c.segments++
	c.at("segment", depth)
	result, _ = node.DefaultAccept[int, int](c, depth+1)
	return result + 1
}

func (c *counter) VisitPoint(node *shapes.Point, depth int) int {
	c.points++
	c.at(key(node), depth)
	return 1
}

func (c *counter) VisitTag(node *other.Tag, depth int) {
	c.tags++
	c.at("tag "+node.Label, depth)
}

// printer is the second (P, R) pair over the same nodes: P is an indent
// string, R is the text of the last node visited beneath this one.
type printer struct {
	out strings.Builder
}

var (
	_ shapes.VisitorPRE_Scene[string, string]  = (*printer)(nil)
	_ shapes.VisitorPR_Shape[string, string]   = (*printer)(nil)
	_ shapes.VisitorPR_Segment[string, string] = (*printer)(nil)
	_ shapes.VisitorPR_Point[string, string]   = (*printer)(nil)
	_ other.VisitorPR_Tag[string, string]      = (*printer)(nil)
)

func (p *printer) VisitScene(node *shapes.Scene, indent string) (result string, err error) {
	fmt.Fprintf(&p.out, "%sscene\n", indent)
	return node.DefaultAccept[string, string](p, indent+"  ")
}

func (p *printer) VisitShape(node *shapes.Shape, indent string) (result string) {
	fmt.Fprintf(&p.out, "%sshape %s\n", indent, node.Name)
	result, _ = node.DefaultAccept[string, string](p, indent+"  ")
	return
}

func (p *printer) VisitSegment(node *shapes.Segment, indent string) (result string) {
	fmt.Fprintf(&p.out, "%ssegment\n", indent)
	_, _ = node.DefaultAccept[string, string](p, indent+"  ")
	return "segment"
}

func (p *printer) VisitPoint(node *shapes.Point, indent string) string {
	fmt.Fprintf(&p.out, "%s%s\n", indent, key(node))
	return key(node)
}

func (p *printer) VisitTag(node *other.Tag, indent string) string {
	fmt.Fprintf(&p.out, "%stag %s\n", indent, node.Label)
	return "tag " + node.Label
}

// xValue is an R-only visitor: VisitPoint returns the point's X.
type xValue struct{}

var (
	_ shapes.VisitorR_Point[unit, int32] = (*xValue)(nil)
)

func (*xValue) VisitPoint(node *shapes.Point) int32 {
	return node.X
}

// stopAtPoint is the E variant: fails on the point whose X == stopX.
type stopAtPoint struct {
	stopX int32
	seen  []string
}

var (
	_ shapes.VisitorE_Point[unit, unit] = (*stopAtPoint)(nil)
)

func (s *stopAtPoint) VisitPoint(node *shapes.Point) error {
	s.seen = append(s.seen, key(node))
	if node.X == s.stopX {
		return errBoom
	}
	return nil
}

// stopAtShape fails (PRE) on the shape with the given name and counts
// points (PE) beneath the shapes it did walk.
type stopAtShape struct {
	stopName string
	shapes   []string
	points   int
}

var (
	_ shapes.VisitorPRE_Shape[int, int] = (*stopAtShape)(nil)
	_ shapes.VisitorPE_Point[int, int]  = (*stopAtShape)(nil)
)

func (s *stopAtShape) VisitShape(node *shapes.Shape, depth int) (int, error) {
	s.shapes = append(s.shapes, node.Name)
	if node.Name == s.stopName {
		return 0, errBoom
	}
	return node.DefaultAccept[int, int](s, depth+1)
}

func (s *stopAtShape) VisitPoint(node *shapes.Point, depth int) error {
	s.points++
	return nil
}

// wrongPayload has a VisitPoint whose payload type is string. Used with
// (int, int) none of the eight probes match and TypeCheckMethod must
// panic; used with (string, int) it is the PR variant.
type wrongPayload struct {
	calls int
}

func (w *wrongPayload) VisitPoint(node *shapes.Point, payload string) int {
	w.calls++
	return 7
}

// wrongPayloadSkipped is wrongPayload opted out of the check.
type wrongPayloadSkipped struct {
	calls int
}

var (
	_ visit.SkipCheckName = (*wrongPayloadSkipped)(nil)
)

func (w *wrongPayloadSkipped) VisitPoint(node *shapes.Point, payload string) int {
	w.calls++
	return 7
}

func (*wrongPayloadSkipped) SkipCheckName() {}

// wrongResult has the right payload but the wrong result type.
type wrongResult struct{}

func (*wrongResult) VisitPoint(node *shapes.Point, payload int) string {
	return "nope"
}

// One visitor per Accept probe, for the dispatch table test.
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
	_ shapes.Visitor_Point[int, int]    = (*vPlain)(nil)
	_ shapes.VisitorE_Point[int, int]   = (*vE)(nil)
	_ shapes.VisitorP_Point[int, int]   = (*vP)(nil)
	_ shapes.VisitorPE_Point[int, int]  = (*vPE)(nil)
	_ shapes.VisitorR_Point[int, int]   = (*vR)(nil)
	_ shapes.VisitorRE_Point[int, int]  = (*vRE)(nil)
	_ shapes.VisitorPR_Point[int, int]  = (*vPR)(nil)
	_ shapes.VisitorPRE_Point[int, int] = (*vPRE)(nil)
)

func (v *vPlain) VisitPoint(node *shapes.Point) { v.n++ }
func (v *vE) VisitPoint(node *shapes.Point) error {
	v.n++
	return errBoom
}
func (v *vP) VisitPoint(node *shapes.Point, payload int) {
	v.n++
	v.p = payload
}
func (v *vPE) VisitPoint(node *shapes.Point, payload int) error {
	v.n++
	v.p = payload
	return errBoom
}
func (v *vR) VisitPoint(node *shapes.Point) int {
	v.n++
	return 5
}
func (v *vRE) VisitPoint(node *shapes.Point) (int, error) {
	v.n++
	return 5, errBoom
}
func (v *vPR) VisitPoint(node *shapes.Point, payload int) int {
	v.n++
	v.p = payload
	return payload * 2
}
func (v *vPRE) VisitPoint(node *shapes.Point, payload int) (int, error) {
	v.n++
	v.p = payload
	return payload * 2, errBoom
}

// ---------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------

// TestAcceptDispatchesEachVariant: Accept picks the one probe a visitor
// satisfies, passes the payload only to the P variants, takes the result
// only from the R variants and the error only from the E variants.
func TestAcceptDispatchesEachVariant(t *testing.T) {
	var (
		p   = pt(1, 2)
		r   int
		err error
	)

	var plain vPlain
	r, err = p.Accept[int, int](&plain, 9)
	eq(t, "plain", []any{plain.n, r, err}, []any{1, 0, error(nil)})

	var e vE
	r, err = p.Accept[int, int](&e, 9)
	eq(t, "E", []any{e.n, r, err}, []any{1, 0, errBoom})

	var pp vP
	r, err = p.Accept[int, int](&pp, 9)
	eq(t, "P", []any{pp.n, pp.p, r, err}, []any{1, 9, 0, error(nil)})

	var pe vPE
	r, err = p.Accept[int, int](&pe, 9)
	eq(t, "PE", []any{pe.n, pe.p, r, err}, []any{1, 9, 0, errBoom})

	var rr vR
	r, err = p.Accept[int, int](&rr, 9)
	eq(t, "R", []any{rr.n, r, err}, []any{1, 5, error(nil)})

	var re vRE
	r, err = p.Accept[int, int](&re, 9)
	eq(t, "RE", []any{re.n, r, err}, []any{1, 5, errBoom})

	var pr vPR
	r, err = p.Accept[int, int](&pr, 9)
	eq(t, "PR", []any{pr.n, pr.p, r, err}, []any{1, 9, 18, error(nil)})

	var pre vPRE
	r, err = p.Accept[int, int](&pre, 9)
	eq(t, "PRE", []any{pre.n, pre.p, r, err}, []any{1, 9, 18, errBoom})
}

// TestDefaultAcceptFallThrough: a visitor with only VisitPoint still
// reaches every Point because Scene, Shape, Segment, Empty and Tag fall
// through to DefaultAccept. The order pins down field order, Vector
// index order, sorted StringMap keys, alias expansion, nested
// Vector<Nullable>/Vector<Vector>, and cross-module descent of the
// non-Point structs.
func TestDefaultAcceptFallThrough(t *testing.T) {
	var (
		scene  = newScene()
		lister pointLister
		r      unit
		err    error
	)
	if r, err = scene.Accept[unit, unit](&lister, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "result", r, unit{})
	eq(t, "points in walk order", lister.seen, allPoints)
}

// TestCounterPayloadDownResultUp: the depth payload is incremented by each
// Visit method that calls DefaultAccept, so every node records the depth
// it sits at; the count result flows back up through DefaultAccept.
func TestCounterPayloadDownResultUp(t *testing.T) {
	var (
		scene = newScene()
		c     = newCounter()
		r     int
		err   error
	)
	if r, err = scene.Accept[int, int](c, 0); err != nil {
		t.Fatal(err)
	}

	// Counts: every node of each type was visited exactly once.
	eq(t, "scenes", c.scenes, 1)
	eq(t, "shapes", c.shapes, 3)
	eq(t, "segments", c.segments, 1)
	eq(t, "points", c.points, len(allPoints))
	eq(t, "tags", c.tags, 3)

	// Payload: Scene at 0, its Shapes and Segment at 1, everything inside
	// those at 2.
	eq(t, "scene depth", c.depths["scene"], []int{0})
	eq(t, "segment depth", c.depths["segment"], []int{1})
	for _, name := range []string{"tri", "dot", "foc"} {
		eq(t, "shape "+name+" depth", c.depths["shape "+name], []int{1})
	}
	for _, label := range []string{"t0", "t1", "t2"} {
		eq(t, "tag "+label+" depth", c.depths["tag "+label], []int{2})
	}
	for _, k := range allPoints {
		eq(t, "point "+k+" depth", c.depths[k], []int{2})
	}

	// Result: Scene = 1 + its last field (diagonal Segment = 1 + its last
	// field (Point b = 1)) = 3. The Shapes contribute nothing to the
	// result because Scene's last field is the Segment.
	eq(t, "result", r, 3)
}

// TestLastFieldResult: DefaultAccept returns the last descended field's
// result; leaf fields keep their Accept<Field> helper but are not called.
func TestLastFieldResult(t *testing.T) {
	var (
		seg = newDiagonal()
		lab = shapes.MakeAll_Labelled(pt(7, 0), "seven")
		tri = newTri()
		xv  xValue
		r   int32
		err error
	)

	// Segment: last field b.
	if r, err = seg.DefaultAccept[unit, int32](&xv, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "segment DefaultAccept = b.X", r, int32(41))
	if r, err = seg.Accept[unit, int32](&xv, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "segment Accept falls through to the same", r, int32(41))
	if r, err = seg.AcceptA[unit, int32](&xv, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "segment AcceptA = a.X", r, int32(40))

	// Labelled: text is a leaf, which DefaultAccept does not call, so the
	// result is at's (the last descended field's).
	if r, err = lab.AcceptAt[unit, int32](&xv, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "labelled AcceptAt = at.X", r, int32(7))
	if r, err = lab.DefaultAccept[unit, int32](&xv, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "labelled DefaultAccept = at.X (last descended field)", r, int32(7))

	// Shape: the last descended field is Tag, which has no VisitTag here
	// and no descended fields of its own, so its DefaultAccept returns zero
	// and the Points' results do not surface.
	if r, err = tri.Accept[unit, int32](&xv, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "shape result = zero", r, int32(0))
	if r, err = tri.AcceptRings[unit, int32](&xv, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "shape AcceptRings = last ring's last point", r, int32(11))
}

// TestErrorStopsWalk: the E variants' error stops the walk at once; later
// siblings, later fields and later elements of nested collections are not
// visited, and the error comes out of the top-level Accept unchanged.
func TestErrorStopsWalk(t *testing.T) {
	var (
		scene = newScene()
		err   error
	)

	// E variant on Point, failing in the middle of a Vector.
	var atOutline = stopAtPoint{
		stopX: 2,
	}
	if _, err = scene.Accept[unit, unit](&atOutline, unit{}); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
	eq(t, "stops after (2,2)", atOutline.seen, allPoints[:3])

	// Failing inside the inner loop of Vector<Vector<Point>>.
	var atRings = stopAtPoint{
		stopX: 10,
	}
	if _, err = scene.Accept[unit, unit](&atRings, unit{}); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
	eq(t, "stops after (10,10)", atRings.seen, allPoints[:11])

	// Failing on the very last point still reports the error.
	var atLast = stopAtPoint{
		stopX: 41,
	}
	if _, err = scene.Accept[unit, unit](&atLast, unit{}); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
	eq(t, "every point seen", atLast.seen, allPoints)

	// Never failing: the E variant returns nil and the walk completes.
	var never = stopAtPoint{
		stopX: -1,
	}
	if _, err = scene.Accept[unit, unit](&never, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "every point seen", never.seen, allPoints)

	// PRE variant on Shape failing on shapes[1]: shapes[0]'s 12 points are
	// counted, then nothing (not focus, not the diagonal).
	var atDot = stopAtShape{
		stopName: "dot",
	}
	if _, err = scene.Accept[int, int](&atDot, 0); !errors.Is(err, errBoom) {
		t.Fatalf("want errBoom, got %v", err)
	}
	eq(t, "shapes seen", atDot.shapes, []string{"tri", "dot"})
	eq(t, "points seen", atDot.points, 12)
}

// TestStringMapSortedKeyOrder: StringMap values are visited in sorted key
// order regardless of insertion order, and the walk is deterministic.
func TestStringMapSortedKeyOrder(t *testing.T) {
	var (
		shape = newTri()
		err   error
	)
	for i := 0; i < 20; i++ {
		var lister pointLister
		if _, err = shape.AcceptNamed[unit, unit](&lister, unit{}); err != nil {
			t.Fatal(err)
		}
		eq(t, "sorted keys alpha, mid, zeta", lister.seen, []string{"(6,6)", "(7,7)", "(5,5)"})
	}
}

// TestNullableNilSkipped: nil Nullables are skipped at every nesting.
func TestNullableNilSkipped(t *testing.T) {
	var (
		tri   = newTri()
		dot   = newDot()
		scene = newSmallScene()
		err   error
	)

	var pivotSet pointLister
	if _, err = tri.AcceptPivot[unit, unit](&pivotSet, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "non-nil pivot visited", pivotSet.seen, []string{"(4,4)"})

	var pivotNil pointLister
	if _, err = dot.AcceptPivot[unit, unit](&pivotNil, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "nil pivot skipped", len(pivotNil.seen), 0)

	var sparse pointLister
	if _, err = tri.AcceptSparse[unit, unit](&sparse, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "Vector<Nullable>: nil elements skipped", sparse.seen, []string{"(8,8)"})

	scene.Focus = nil
	var focusNil pointLister
	if _, err = scene.Accept[unit, unit](&focusNil, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "nil focus skipped, diagonal still walked", focusNil.seen, []string{"(40,40)", "(41,41)"})
}

// TestVectorNesting: Vector<Point>, Vector<Nullable<Point>> and
// Vector<Vector<Point>> (including an empty inner vector) are descended
// element by element.
func TestVectorNesting(t *testing.T) {
	var (
		tri = newTri()
		dot = newDot()
		err error
	)

	var outline pointLister
	if _, err = tri.AcceptOutline[unit, unit](&outline, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "outline", outline.seen, []string{"(1,1)", "(2,2)", "(3,3)"})

	var rings pointLister
	if _, err = tri.AcceptRings[unit, unit](&rings, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "rings", rings.seen, []string{"(9,9)", "(10,10)", "(11,11)"})

	var empties pointLister
	if _, err = dot.AcceptOutline[unit, unit](&empties, unit{}); err != nil {
		t.Fatal(err)
	}
	if _, err = dot.AcceptRings[unit, unit](&empties, unit{}); err != nil {
		t.Fatal(err)
	}
	if _, err = dot.AcceptSparse[unit, unit](&empties, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "nil and empty vectors visit nothing", len(empties.seen), 0)
}

// TestAliasFieldDescended: Origin is `type Origin = Point`, so the field is
// descended exactly as a Point field.
func TestAliasFieldDescended(t *testing.T) {
	var (
		tri    = newTri()
		lister pointLister
		err    error
		_      *shapes.Origin = (*shapes.Point)(nil)
	)
	if _, err = tri.AcceptOrigin[unit, unit](&lister, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "origin visited as a Point", lister.seen, []string{"(0,0)"})
}

// TestCrossModuleDescent: a visitor on m1.other.Tag alone is reached from
// a walk that starts in m1.shapes.
func TestCrossModuleDescent(t *testing.T) {
	var (
		scene  = newScene()
		lister tagLister
		err    error
		_      other.Visitor_Tag[unit, unit] = (*tagLister)(nil)
	)
	if _, err = scene.Accept[unit, unit](&lister, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "tags in shape order", lister.seen, []string{"t0", "t1", "t2"})
}

// TestLeafFields: the Pair<Int64,Int64>, String and Vector<String> fields
// have an Accept<Field> that returns the zero result and visits nothing.
func TestLeafFields(t *testing.T) {
	var (
		tri = newTri()
		c   = newCounter()
		r   int
		err error
	)
	if r, err = tri.AcceptExtent[int, int](c, 0); err != nil {
		t.Fatal(err)
	}
	eq(t, "Pair leaf result", r, 0)
	if r, err = tri.AcceptName[int, int](c, 0); err != nil {
		t.Fatal(err)
	}
	eq(t, "String leaf result", r, 0)
	if r, err = tri.AcceptLabels[int, int](c, 0); err != nil {
		t.Fatal(err)
	}
	eq(t, "Vector<String> leaf result", r, 0)
	eq(t, "nothing visited", *c, *newCounter())
}

// TestUnionFieldDescended: from M2 on a union gets an Accept, so the
// Colour field of Shape is descended: AcceptColour dispatches to a
// VisitColour, and Shape.DefaultAccept calls it (between rings and
// empty). Colour's own branches (Void, Void, String) are all leaves, so
// with no VisitColour the union visits nothing and returns zero.
func TestUnionFieldDescended(t *testing.T) {
	var (
		tri = newTri()
		dot = newDot()
		cv  colourVisitor
		c   = newCounter()
		r   string
		n   int
		err error
	)
	if r, err = tri.AcceptColour[unit, string](&cv, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "AcceptColour dispatches to VisitColour", r, "red")
	if r, err = dot.AcceptColour[unit, string](&cv, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "custom branch", r, "custom:teal")
	if _, err = tri.Accept[unit, string](&cv, unit{}); err != nil {
		t.Fatal(err)
	}
	eq(t, "Shape.DefaultAccept reaches the union once", cv.seen, []string{"red", "custom:teal", "red"})

	// Every branch is a leaf: nothing beneath the union is visited and the
	// result is zero.
	if n, err = tri.AcceptColour[int, int](c, 0); err != nil {
		t.Fatal(err)
	}
	eq(t, "leaf branches result", n, 0)
	if n, err = tri.Colour.DefaultAccept[int, int](c, 0); err != nil {
		t.Fatal(err)
	}
	eq(t, "leaf branches DefaultAccept result", n, 0)
	eq(t, "nothing visited", *c, *newCounter())
}

type colourVisitor struct {
	seen []string
}

var (
	_ shapes.VisitorR_Colour[unit, string] = (*colourVisitor)(nil)
)

func (v *colourVisitor) VisitColour(node *shapes.Colour) (result string) {
	var (
		custom string
		ok     bool
	)
	result = "red"
	if _, ok = node.Cast_green(); ok {
		result = "green"
	}
	if custom, ok = node.Cast_custom(); ok {
		result = "custom:" + custom
	}
	v.seen = append(v.seen, result)
	return
}

// TestEmptyStruct: an Empty has a DefaultAccept that just returns, and a
// VisitEmpty is still dispatched to.
func TestEmptyStruct(t *testing.T) {
	var (
		e      = shapes.MakeAll_Empty()
		tri    = newTri()
		lister pointLister
		r      int
		err    error
	)
	if r, err = e.Accept[int, int](&lister, 0); err != nil {
		t.Fatal(err)
	}
	eq(t, "result", r, 0)
	if r, err = e.DefaultAccept[int, int](&lister, 0); err != nil {
		t.Fatal(err)
	}
	eq(t, "result", r, 0)
	eq(t, "nothing visited", len(lister.seen), 0)

	var ev emptyVisitor
	if r, err = tri.AcceptEmpty[int, int](&ev, 0); err != nil {
		t.Fatal(err)
	}
	eq(t, "VisitEmpty result", r, 42)
	eq(t, "VisitEmpty called", ev.n, 1)
}

type emptyVisitor struct {
	n int
}

var (
	_ shapes.VisitorR_Empty[int, int] = (*emptyVisitor)(nil)
)

func (v *emptyVisitor) VisitEmpty(node *shapes.Empty) int {
	v.n++
	return 42
}

// TestTypeCheckMethod: a VisitPoint with the wrong P or R matches no probe
// and TypeCheckMethod panics rather than silently falling through; the
// same method with matching (P, R) is dispatched; SkipCheckName turns the
// panic into a fall-through.
func TestTypeCheckMethod(t *testing.T) {
	var (
		p   = pt(1, 1)
		w   wrongPayload
		ws  wrongPayloadSkipped
		wr  wrongResult
		r   int
		err error
	)

	eq(t, "wrong payload panics", panics(func() {
		_, _ = p.Accept[int, int](&w, 0)
	}), "VisitPoint")
	eq(t, "not called", w.calls, 0)

	eq(t, "wrong result panics", panics(func() {
		_, _ = p.Accept[int, int](&wr, 0)
	}), "VisitPoint")

	if r, err = p.Accept[string, int](&w, "x"); err != nil {
		t.Fatal(err)
	}
	eq(t, "matching (P, R) dispatches", []int{r, w.calls}, []int{7, 1})

	eq(t, "SkipCheckName: no panic", panics(func() {
		r, err = p.Accept[int, int](&ws, 0)
	}), "")
	eq(t, "SkipCheckName: falls through to DefaultAccept", []any{r, err, ws.calls}, []any{0, error(nil), 0})
}

// panics runs f and returns the panic message's substring, or "" if f
// returned normally.
func panics(f func()) (msg string) {
	defer func() {
		var (
			rec = recover()
		)
		if rec == nil {
			return
		}
		msg = fmt.Sprint(rec)
		if strings.Contains(msg, "VisitPoint") {
			msg = "VisitPoint"
		}
	}()
	f()
	return ""
}

// TestSameNodeTwoPairs: the one set of nodes is visited by a counter
// (int, int) and a printer (string, string).
func TestSameNodeTwoPairs(t *testing.T) {
	var (
		scene = newSmallScene()
		c     = newCounter()
		p     printer
		n     int
		s     string
		err   error
	)

	if n, err = scene.Accept[int, int](c, 0); err != nil {
		t.Fatal(err)
	}
	eq(t, "counter result", n, 3)
	eq(t, "counter counts", []int{c.scenes, c.shapes, c.segments, c.points, c.tags}, []int{1, 1, 1, 4, 1})

	if s, err = scene.Accept[string, string](&p, ""); err != nil {
		t.Fatal(err)
	}
	eq(t, "printer result is the last field's", s, "segment")
	eq(t, "printer output", p.out.String(), strings.Join([]string{
		"scene",
		"  shape foc",
		"    (30,30)",
		"    (31,31)",
		"    tag t2",
		"  segment",
		"    (40,40)",
		"    (41,41)",
		"",
	}, "\n"))

	// The printer's result for a Shape alone is its last field's: the Tag.
	var foc = newFoc()
	if s, err = foc.Accept[string, string](&p, ""); err != nil {
		t.Fatal(err)
	}
	eq(t, "shape result is the tag's", s, "tag t2")
}
