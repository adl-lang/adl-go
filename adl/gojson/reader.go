package gojson

import (
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// Reader reads one JSON value from a buffer, token by token. A decoder
// that the backend writes reads the value it decodes and leaves the
// Reader after it.
type Reader struct {
	data []byte
	pos  int
}

// NewReader is a Reader over data.
func NewReader(data []byte) *Reader {
	return &Reader{
		data: data,
	}
}

// SyntaxError is JSON that does not read, and where.
type SyntaxError struct {
	Offset int
	Msg    string
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("json: offset %d: %s", e.Offset, e.Msg)
}

func (r *Reader) errorf(format string, args ...any) error {
	return &SyntaxError{
		Offset: r.pos,
		Msg:    fmt.Sprintf(format, args...),
	}
}

// ws steps over white space.
func (r *Reader) ws() {
	for r.pos < len(r.data) {
		switch r.data[r.pos] {
		case ' ', '\t', '\n', '\r':
			r.pos++
		default:
			return
		}
	}
}

// Peek is the next byte that is not white space, or 0 at the end.
func (r *Reader) Peek() byte {
	r.ws()
	if r.pos >= len(r.data) {
		return 0
	}
	return r.data[r.pos]
}

// End is nil when nothing but white space is left.
func (r *Reader) End() error {
	r.ws()
	if r.pos != len(r.data) {
		return r.errorf("data after the value")
	}
	return nil
}

func (r *Reader) expect(b byte) error {
	r.ws()
	if r.pos >= len(r.data) {
		return r.errorf("want %q, found the end", b)
	}
	if r.data[r.pos] != b {
		return r.errorf("want %q, found %q", b, r.data[r.pos])
	}
	r.pos++
	return nil
}

func (r *Reader) literal(word string) bool {
	r.ws()
	if len(r.data)-r.pos >= len(word) && string(r.data[r.pos:r.pos+len(word)]) == word {
		r.pos += len(word)
		return true
	}
	return false
}

// Null reads null when it is next, and reports whether it was.
func (r *Reader) Null() bool {
	return r.literal("null")
}

// Bool reads true or false.
func (r *Reader) Bool() (bool, error) {
	if r.literal("true") {
		return true, nil
	}
	if r.literal("false") {
		return false, nil
	}
	return false, r.errorf("want a bool")
}

// number is the next number's text.
func (r *Reader) number() (string, error) {
	r.ws()
	var start = r.pos
	for r.pos < len(r.data) {
		var c = r.data[r.pos]
		if (c >= '0' && c <= '9') || c == '-' || c == '+' || c == '.' || c == 'e' || c == 'E' {
			r.pos++
			continue
		}
		break
	}
	if start == r.pos {
		return "", r.errorf("want a number")
	}
	return string(r.data[start:r.pos]), nil
}

// Int reads an integer of bits bits. A number with a fraction or an
// exponent is read as a float and truncated, as package adl converts the
// float64 encoding/json gives it.
func (r *Reader) Int(bits int) (int64, error) {
	var (
		text string
		v    int64
		f    float64
		err  error
	)
	if text, err = r.number(); err != nil {
		return 0, err
	}
	if v, err = strconv.ParseInt(text, 10, bits); err == nil {
		return v, nil
	}
	if f, err = strconv.ParseFloat(text, 64); err != nil {
		return 0, r.errorf("%q is no number", text)
	}
	return int64(f), nil
}

// Uint reads an unsigned integer of bits bits, as Int does.
func (r *Reader) Uint(bits int) (uint64, error) {
	var (
		text string
		v    uint64
		f    float64
		err  error
	)
	if text, err = r.number(); err != nil {
		return 0, err
	}
	if v, err = strconv.ParseUint(text, 10, bits); err == nil {
		return v, nil
	}
	if f, err = strconv.ParseFloat(text, 64); err != nil || f < 0 {
		return 0, r.errorf("%q is no unsigned number", text)
	}
	return uint64(f), nil
}

// Float reads a number as a float64.
func (r *Reader) Float() (float64, error) {
	var (
		text string
		f    float64
		err  error
	)
	if text, err = r.number(); err != nil {
		return 0, err
	}
	if f, err = strconv.ParseFloat(text, 64); err != nil && !errors.Is(err, strconv.ErrRange) {
		return 0, r.errorf("%q is no number", text)
	}
	if math.IsInf(f, 0) {
		return 0, r.errorf("%q is out of range", text)
	}
	return f, nil
}

// String reads a string and undoes its escapes.
func (r *Reader) String() (string, error) {
	var (
		start int
		buf   []byte
	)
	if err := r.expect('"'); err != nil {
		return "", err
	}
	start = r.pos
	// The common case: no escape, one slice.
	for r.pos < len(r.data) {
		var c = r.data[r.pos]
		if c == '"' {
			var s = string(r.data[start:r.pos])
			r.pos++
			return s, nil
		}
		if c == '\\' || c < 0x20 {
			break
		}
		r.pos++
	}
	buf = append(buf, r.data[start:r.pos]...)
	for r.pos < len(r.data) {
		var c = r.data[r.pos]
		switch {
		case c == '"':
			r.pos++
			return string(buf), nil
		case c < 0x20:
			return "", r.errorf("a control byte in a string")
		case c == '\\':
			r.pos++
			if r.pos >= len(r.data) {
				return "", r.errorf("a string ends in a backslash")
			}
			var e = r.data[r.pos]
			r.pos++
			switch e {
			case '"', '\\', '/':
				buf = append(buf, e)
			case 'b':
				buf = append(buf, '\b')
			case 'f':
				buf = append(buf, '\f')
			case 'n':
				buf = append(buf, '\n')
			case 'r':
				buf = append(buf, '\r')
			case 't':
				buf = append(buf, '\t')
			case 'u':
				var (
					r1  rune
					err error
				)
				if r1, err = r.hex4(); err != nil {
					return "", err
				}
				if utf16.IsSurrogate(r1) {
					var r2 = utf8.RuneError
					if r.pos+1 < len(r.data) && r.data[r.pos] == '\\' && r.data[r.pos+1] == 'u' {
						var save = r.pos
						r.pos += 2
						if r2, err = r.hex4(); err != nil {
							return "", err
						}
						if dec := utf16.DecodeRune(r1, r2); dec != utf8.RuneError {
							buf = utf8.AppendRune(buf, dec)
							continue
						}
						r.pos = save
					}
					buf = utf8.AppendRune(buf, utf8.RuneError)
					continue
				}
				buf = utf8.AppendRune(buf, r1)
			default:
				return "", r.errorf("the escape \\%c", e)
			}
		default:
			buf = append(buf, c)
			r.pos++
		}
	}
	return "", r.errorf("a string with no end")
}

func (r *Reader) hex4() (rune, error) {
	var v rune
	if r.pos+4 > len(r.data) {
		return 0, r.errorf("a short \\u escape")
	}
	for i := 0; i < 4; i++ {
		var c = r.data[r.pos+i]
		switch {
		case c >= '0' && c <= '9':
			v = v<<4 | rune(c-'0')
		case c >= 'a' && c <= 'f':
			v = v<<4 | rune(c-'a'+10)
		case c >= 'A' && c <= 'F':
			v = v<<4 | rune(c-'A'+10)
		default:
			return 0, r.errorf("a bad \\u escape")
		}
	}
	r.pos += 4
	return v, nil
}

// Bytes64 reads a base64 string, or null as nil.
func (r *Reader) Bytes64() ([]byte, error) {
	var (
		s   string
		b   []byte
		err error
	)
	if r.Null() {
		return nil, nil
	}
	if s, err = r.String(); err != nil {
		return nil, err
	}
	if b, err = base64.StdEncoding.DecodeString(s); err != nil {
		return nil, r.errorf("no base64: %v", err)
	}
	return b, nil
}

// Object reads an object: fn is called with each key, and must read the
// value after it.
func (r *Reader) Object(fn func(key string) error) error {
	var (
		key string
		err error
	)
	if err = r.expect('{'); err != nil {
		return err
	}
	if r.Peek() == '}' {
		r.pos++
		return nil
	}
	for {
		if key, err = r.String(); err != nil {
			return err
		}
		if err = r.expect(':'); err != nil {
			return err
		}
		if err = fn(key); err != nil {
			return err
		}
		switch r.Peek() {
		case ',':
			r.pos++
		case '}':
			r.pos++
			return nil
		default:
			return r.errorf("want , or } in an object")
		}
	}
}

// Array reads an array: fn is called once per element, and must read it.
func (r *Reader) Array(fn func() error) error {
	var err error
	if err = r.expect('['); err != nil {
		return err
	}
	if r.Peek() == ']' {
		r.pos++
		return nil
	}
	for {
		if err = fn(); err != nil {
			return err
		}
		switch r.Peek() {
		case ',':
			r.pos++
		case ']':
			r.pos++
			return nil
		default:
			return r.errorf("want , or ] in an array")
		}
	}
}

// Union reads the start of a union: a string, the branch alone, or an
// object of one key, the branch and its value. inObject says which; when
// it is set, the caller reads the value and then calls UnionEnd.
func (r *Reader) Union() (branch string, inObject bool, err error) {
	switch r.Peek() {
	case '"':
		branch, err = r.String()
		return branch, false, err
	case '{':
		r.pos++
		if branch, err = r.String(); err != nil {
			return "", false, err
		}
		if err = r.expect(':'); err != nil {
			return "", false, err
		}
		return branch, true, nil
	}
	return "", false, r.errorf("want a union: a string or an object")
}

// UnionEnd reads the end of a union that Union read as an object.
func (r *Reader) UnionEnd(inObject bool) error {
	if !inObject {
		return nil
	}
	if r.Peek() != '}' {
		return r.errorf("a union's object has one key")
	}
	r.pos++
	return nil
}

// Skip reads one value of any kind, and drops it.
func (r *Reader) Skip() error {
	var err error
	switch r.Peek() {
	case '{':
		return r.Object(func(string) error {
			return r.Skip()
		})
	case '[':
		return r.Array(r.Skip)
	case '"':
		_, err = r.String()
		return err
	case 't', 'f':
		_, err = r.Bool()
		return err
	case 'n':
		if r.Null() {
			return nil
		}
		return r.errorf("want null")
	}
	_, err = r.number()
	return err
}

// FieldError is a struct's field that the JSON lacks or has too many of.
type FieldError struct {
	Type  string
	Field string
	Msg   string
}

func (e *FieldError) Error() string {
	return fmt.Sprintf("%s: %s '%s'", e.Type, e.Msg, e.Field)
}

// Missing is the error of a required field the JSON lacks.
func Missing(typ, field string) error {
	return &FieldError{
		Type:  typ,
		Field: field,
		Msg:   "required field missing",
	}
}

// Unexpected is the error of a field the struct does not have.
func Unexpected(typ, field string) error {
	return &FieldError{
		Type:  typ,
		Field: field,
		Msg:   "unexpected field",
	}
}

// BranchError is a union's branch the decl does not have, or one that
// needs a value and was given as a string.
type BranchError struct {
	Type   string
	Branch string
	Msg    string
}

func (e *BranchError) Error() string {
	return fmt.Sprintf("%s: %s '%s'", e.Type, e.Msg, e.Branch)
}

// UnknownBranch is the error of a branch the union does not have.
func UnknownBranch(typ, branch string) error {
	return &BranchError{
		Type:   typ,
		Branch: branch,
		Msg:    "unexpected branch",
	}
}

// NeedsValue is the error of a branch written as a string whose type is
// not Void.
func NeedsValue(typ, branch string) error {
	return &BranchError{
		Type:   typ,
		Branch: branch,
		Msg:    "a branch with a value written as a string",
	}
}

// Incomplete is the error of a union with no branch set, which package
// adl refuses to encode too.
func Incomplete(typ string) error {
	return fmt.Errorf("%s: cannot encode incomplete value", typ)
}
