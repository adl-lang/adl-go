// Package gojson is the runtime of goadlc's GoJson backend: a JSON writer
// and reader with no reflect, and the codecs of the primitives and of the
// sys.types decls the backend does not generate. The backend writes one
// Encode_, Decode_ and Equal_ func per decl, over this package, that read
// and write the same ADL JSON as the reflective codec in package adl.
//
// It imports neither reflect nor encoding/json, so a program built by
// TinyGo, whose reflect is partial, can link it.
package gojson

import (
	"encoding/base64"
	"math"
	"strconv"
	"unicode/utf8"
)

// Writer appends JSON to a buffer.
type Writer struct {
	buf []byte
}

// NewWriter is a Writer with an empty buffer.
func NewWriter() *Writer {
	return &Writer{}
}

// Bytes is what was written. The slice is the Writer's own.
func (w *Writer) Bytes() []byte {
	return w.buf
}

// Reset empties the buffer and keeps its memory.
func (w *Writer) Reset() {
	w.buf = w.buf[:0]
}

// Byte writes one byte as it is.
func (w *Writer) Byte(b byte) {
	w.buf = append(w.buf, b)
}

// Raw writes s as it is.
func (w *Writer) Raw(s string) {
	w.buf = append(w.buf, s...)
}

// Key writes a field's name and the colon after it.
func (w *Writer) Key(k string) {
	w.buf = appendString(w.buf, k)
	w.buf = append(w.buf, ':')
}

// String writes s as a JSON string, escaped as package adl escapes it.
func (w *Writer) String(s string) {
	w.buf = appendString(w.buf, s)
}

// Int writes v in decimal.
func (w *Writer) Int(v int64) {
	w.buf = strconv.AppendInt(w.buf, v, 10)
}

// Uint writes v in decimal.
func (w *Writer) Uint(v uint64) {
	w.buf = strconv.AppendUint(w.buf, v, 10)
}

// Bool writes true or false.
func (w *Writer) Bool(v bool) {
	w.buf = strconv.AppendBool(w.buf, v)
}

// Null writes null.
func (w *Writer) Null() {
	w.buf = append(w.buf, "null"...)
}

// Float writes f as package adl writes a Float or a Double: as an ES6
// number, %f below 1e21 and from 1e-6, %e outside, with no padded
// exponent.
func (w *Writer) Float(f float64) {
	var (
		abs = math.Abs(f)
		fmt = byte('f')
		b   = w.buf
	)
	if abs != 0 && (abs < 1e-6 || abs >= 1e21) {
		fmt = 'e'
	}
	b = strconv.AppendFloat(b, f, fmt, -1, 64)
	if fmt == 'e' {
		var n = len(b)
		if n >= 4 && b[n-4] == 'e' && b[n-3] == '-' && b[n-2] == '0' {
			b[n-2] = b[n-1]
			b = b[:n-1]
		}
	}
	w.buf = b
}

// Bytes64 writes v as a base64 string, null for nil.
func (w *Writer) Bytes64(v []byte) {
	if v == nil {
		w.Null()
		return
	}
	w.buf = append(w.buf, '"')
	w.buf = base64.StdEncoding.AppendEncode(w.buf, v)
	w.buf = append(w.buf, '"')
}

const hex = "0123456789abcdef"

// appendString is package adl's appendString with escapeHTML false: the
// two codecs write the same bytes for one string.
func appendString(dst []byte, src string) []byte {
	dst = append(dst, '"')
	var start = 0
	for i := 0; i < len(src); {
		if b := src[i]; b < utf8.RuneSelf {
			if safeSet[b] {
				i++
				continue
			}
			dst = append(dst, src[start:i]...)
			switch b {
			case '\\', '"':
				dst = append(dst, '\\', b)
			case '\b':
				dst = append(dst, '\\', 'b')
			case '\f':
				dst = append(dst, '\\', 'f')
			case '\n':
				dst = append(dst, '\\', 'n')
			case '\r':
				dst = append(dst, '\\', 'r')
			case '\t':
				dst = append(dst, '\\', 't')
			default:
				dst = append(dst, '\\', 'u', '0', '0', hex[b>>4], hex[b&0xF])
			}
			i++
			start = i
			continue
		}
		var c, size = utf8.DecodeRuneInString(src[i:])
		if c == utf8.RuneError && size == 1 {
			dst = append(dst, src[start:i]...)
			dst = append(dst, `\ufffd`...)
			i += size
			start = i
			continue
		}
		if c == '\u2028' || c == '\u2029' {
			dst = append(dst, src[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', hex[c&0xF])
			i += size
			start = i
			continue
		}
		i += size
	}
	dst = append(dst, src[start:]...)
	dst = append(dst, '"')
	return dst
}

// safeSet is the ASCII bytes a JSON string holds as they are: every
// byte from 0x20 but the double quote and the backslash.
var safeSet = func() (s [utf8.RuneSelf]bool) {
	for b := 0x20; b < utf8.RuneSelf; b++ {
		s[b] = b != '"' && b != '\\'
	}
	return s
}()
