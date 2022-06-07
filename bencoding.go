// Package bencode implements encoding and decoding of values as defined in BEP	3.
package bencode

import (
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unsafe"
)

// Encoder
type Encoder struct {
	w io.Writer
}

// Marshaler
type Marshaler interface {
	MarshalBEncoding() (string, error)
}

// An UnsupportedTypeError is returned by Marshal when attempting to encode an unsupported value type.
type UnsupportedTypeError struct {
	Type reflect.Type
}

func (e *UnsupportedTypeError) Error() string {
	return "bencode: unsupported type: " + e.Type.String()
}

// Marshal
func Marshal(v any) (string, error) {
	var b strings.Builder
	enc := NewEncoder(&b)
	err := enc.Encode(v)
	return b.String(), err
}

// NewEncoder
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{
		w: w,
	}
}

// Encode
func (e *Encoder) Encode(v any) error {
	val := reflect.ValueOf(v)
	switch val.Kind() {
	case reflect.String:
		return e.encodeString(val)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return e.encodeInt(val)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return e.encodeUint(val)
	default:
		return &UnsupportedTypeError{Type: reflect.TypeOf(v)}
	}
}

func (e *Encoder) writeString(s string) error {
	_, err := io.WriteString(e.w, s)
	if err != nil {
		return err
	}

	// TODO: check num of bytes written
	return nil
}

func (e *Encoder) encodeString(val reflect.Value) error {
	s := val.String()
	n := strconv.Itoa(len(s))
	prefix := n + ":"

	ts := prefix + s
	return e.writeString(ts)
}

func (e *Encoder) encodeInt(val reflect.Value) error {
	ns := strconv.FormatInt(val.Int(), 10)
	ts := "i" + ns + "e"
	return e.writeString(ts)
}

func (e *Encoder) encodeUint(val reflect.Value) error {
	ns := strconv.FormatUint(val.Uint(), 10)
	ts := "i" + ns + "e"
	return e.writeString(ts)
}

// Decoder
type Decoder struct {
	src io.Reader

	minBufSize int
	buf        []byte
	start, cur int   // idxs in buf
	offset     int64 // global offset in src
}

// Unmarshaler
type Unmarshaler interface {
	UnmarshalBEncoding(string) error
}

// Unmarshal
func Unmarshal(s string, v any) error {
	dec := NewDecoder(strings.NewReader(s))
	return dec.Decode(v)
}

// An UnmarshalTypeError describes a bencoding value that was not appropriate for a value of a specific Go type.
type UnmarshalTypeError struct {
	Value string       // description of bencoding value
	Type  reflect.Type // type of Go value if could not be assigned to
}

func (e *UnmarshalTypeError) Error() string {
	return "bencode: cannot unmarshal " + e.Value + " into Go value of type " + e.Type.String()
}

// A SyntaxError is a description of a bencoding syntax error.
type SyntaxError struct {
	Offset int64  // error occurred after read Offset byts
	msg    string // description of error
}

func (e *SyntaxError) Error() string {
	return e.msg
}

// NewDecoder
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{
		src:        r,
		minBufSize: 512,
		buf:        make([]byte, 0, 512), // same initial buffer size used in io.ReadAll
	}
}

func (d *Decoder) next() (byte, error) {
	if d.cur == len(d.buf) {
		if d.start == d.cur {
			d.resetBuffer()
		}
		if len(d.buf) == cap(d.buf) || cap(d.buf)-len(d.buf) < d.minBufSize {
			// Add more capacity (let append pick how much).
			d.buf = append(d.buf, 0)[:len(d.buf)]
		}

		n, err := io.ReadFull(d.src, d.buf[len(d.buf):cap(d.buf)])
		if err == io.EOF {
			return 0, io.EOF
		}
		if err != nil && err != io.ErrUnexpectedEOF {
			return 0, err
		}
		d.buf = d.buf[:len(d.buf)+n]
	}
	char := d.buf[d.cur]
	d.cur += 1
	return char, nil
}

func (d *Decoder) backup() {
	d.cur -= 1
}

func (d *Decoder) ignore() {
	d.start = d.cur
}

func (d *Decoder) resetBuffer() {
	d.buf = d.buf[:0]
	d.start = 0
	d.cur = 0
}

func (d *Decoder) slice() []byte {
	b := d.buf[d.start:d.cur]
	d.buf = d.buf[d.cur:]
	d.start = 0
	d.cur = 0
	return b
}

// Decode
func (d *Decoder) Decode(v any) error {
	defer d.resetBuffer()

	char, err := d.next()
	if err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	if err != nil {
		return err
	}

	switch char {
	case 'i':
		return d.decodeInt(v)
	case 'l':
		return d.decodeList(v)
	case 'd':
		return nil
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return d.decodeString(v)
	default:
		return &SyntaxError{
			msg: fmt.Sprintf("unexpected byte: %d", char),
		}
	}
}

func (d *Decoder) decodeString(v any) error {
	// TODO: support []byte since technically UTF8 is not gauranteed
	var s *string
	switch x := v.(type) {
	case *string:
		s = x
	default:
		return &UnmarshalTypeError{
			Value: "string " + string(d.buf),
			Type:  reflect.TypeOf(v),
		}
	}

	length, err := d.scanLength()
	if err != nil {
		return err
	}
	// consume colon
	d.next()
	d.ignore() // ignore colon
	for i := uint64(0); i < length; i++ {
		_, err := d.next()
		if err != nil {
			return err
		}
	}

	*s = bytesToString(d.slice())
	return nil
}

func (d *Decoder) scanLength() (uint64, error) {
	for {
		c, err := d.next()
		if err != nil {
			return 0, err
		}
		if c == ':' {
			d.backup()
			break
		}
		if !isDigit(c) {
			return 0, &SyntaxError{
				Offset: d.offset,
				msg:    fmt.Sprintf("expected ':' after string length, not: %d", c),
			}
		}
	}
	b := d.slice()
	length, err := strconv.ParseUint(bytesToString(b), 10, 0)
	if err != nil {
		return 0, &SyntaxError{
			Offset: d.offset,
			msg:    err.Error(),
		}
	}
	return length, nil
}

func (d *Decoder) decodeInt(v any) error {
	var i *int
	switch x := v.(type) {
	case *int:
		i = x
	default:
		return &UnmarshalTypeError{
			Value: "int " + string(d.buf),
			Type:  reflect.TypeOf(v),
		}
	}

	d.ignore() // ignore 'i'

	isNegative := false
	hasLeadingZero := false
intLoop:
	for {
		c, err := d.next()
		if err != nil {
			return err
		}
		switch {
		case c == '-':
			isNegative = true
		case c == 'e':
			d.backup()
			break intLoop
		case isDigit(c):
			if c == '0' && !isNegative {
				hasLeadingZero = true
				break
			}
			if c == '0' && isNegative {
				return &SyntaxError{
					Offset: d.offset,
					msg:    "negative zero is an invalid integer",
				}
			}
			if hasLeadingZero {
				return &SyntaxError{
					Offset: d.offset,
					msg:    "integer can not start with a leading zero",
				}
			}
		default:
			return &SyntaxError{
				Offset: d.offset,
				msg:    fmt.Sprintf("invalid byte in integer: %d", c),
			}
		}
	}

	b := d.slice()
	n, err := strconv.ParseInt(bytesToString(b), 10, 0)
	if err != nil {
		return err
	}

	// ignore 'e'
	d.next()
	d.ignore()

	*i = int(n)
	return nil
}

func (d *Decoder) decodeList(v any) error {
	d.ignore() // ignore 'l'

	return nil
}

func isDigit(b byte) bool {
	return b == '0' || b == '1' || b == '2' || b == '3' || b == '4' || b == '5' || b == '6' || b == '7' || b == '8' || b == '9'
}

// avoid a allocation when converting []byte to string
func bytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}
