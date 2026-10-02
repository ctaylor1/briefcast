package transcription

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

func Digest(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
func MarshalCanonical(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return CanonicalJSON(b)
}

// CanonicalJSON implements RFC 8785 using IEEE-754 shortest decimal formatting,
// UTF-16 property order and literal Unicode. Duplicate keys and lone surrogates
// are rejected before Go's decoder could silently replace or merge them.
func CanonicalJSON(raw []byte) ([]byte, error) {
	if !utf8.Valid(raw) {
		return nil, errors.New("invalid UTF-8")
	}
	if err := validEscapes(raw); err != nil {
		return nil, err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	value, err := parseValue(d, 0)
	if err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, errors.New("trailing JSON")
	}
	var out bytes.Buffer
	if err = writeCanonical(&out, value); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
func parseValue(d *json.Decoder, depth int) (any, error) {
	if depth > 64 {
		return nil, errors.New("JSON nesting exceeds limit")
	}
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := token.(json.Delim); ok {
		switch delim {
		case '{':
			obj := map[string]any{}
			for d.More() {
				keyToken, err := d.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, errors.New("invalid property")
				}
				if _, exists := obj[key]; exists {
					return nil, errors.New("duplicate property")
				}
				v, err := parseValue(d, depth+1)
				if err != nil {
					return nil, err
				}
				obj[key] = v
			}
			end, err := d.Token()
			if err != nil || end != json.Delim('}') {
				return nil, errors.New("invalid object")
			}
			return obj, nil
		case '[':
			array := []any{}
			for d.More() {
				v, err := parseValue(d, depth+1)
				if err != nil {
					return nil, err
				}
				array = append(array, v)
			}
			end, err := d.Token()
			if err != nil || end != json.Delim(']') {
				return nil, errors.New("invalid array")
			}
			return array, nil
		default:
			return nil, errors.New("unexpected delimiter")
		}
	}
	if n, ok := token.(json.Number); ok {
		f, err := strconv.ParseFloat(string(n), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, errors.New("invalid number")
		}
		return f, nil
	}
	return token, nil
}
func writeCanonical(out *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case nil:
		out.WriteString("null")
	case bool:
		out.WriteString(strconv.FormatBool(x))
	case string:
		writeString(out, x)
	case float64:
		if x == 0 {
			out.WriteByte('0')
			break
		}
		format := byte('f')
		if math.Abs(x) < 1e-6 || math.Abs(x) >= 1e21 {
			format = 'e'
		}
		s := strconv.FormatFloat(x, format, -1, 64)
		if i := strings.IndexByte(s, 'e'); i >= 0 {
			exponent, _ := strconv.Atoi(s[i+1:])
			s = s[:i] + "e"
			if exponent >= 0 {
				s += "+"
			}
			s += strconv.Itoa(exponent)
		}
		out.WriteString(s)
	case []any:
		out.WriteByte('[')
		for i, item := range x {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := writeCanonical(out, item); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(x))
		for key := range x {
			keys = append(keys, key)
		}
		slices.SortFunc(keys, func(a, b string) int { return slices.Compare(utf16.Encode([]rune(a)), utf16.Encode([]rune(b))) })
		out.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				out.WriteByte(',')
			}
			writeString(out, key)
			out.WriteByte(':')
			if err := writeCanonical(out, x[key]); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	default:
		return errors.New("unsupported JSON value")
	}
	return nil
}
func writeString(out *bytes.Buffer, s string) {
	out.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			out.WriteByte('\\')
			out.WriteRune(r)
		case '\b':
			out.WriteString(`\b`)
		case '\f':
			out.WriteString(`\f`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			if r < 32 {
				out.WriteString(`\u00`)
				out.WriteByte("0123456789abcdef"[byte(r)>>4])
				out.WriteByte("0123456789abcdef"[byte(r)&15])
			} else {
				out.WriteRune(r)
			}
		}
	}
	out.WriteByte('"')
}
func validEscapes(raw []byte) error {
	inString := false
	for i := 0; i < len(raw); i++ {
		if raw[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return errors.New("invalid string")
		}
		if raw[i] != 'u' {
			continue
		}
		if i+4 >= len(raw) {
			return errors.New("invalid escape")
		}
		n, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if err != nil {
			return err
		}
		i += 4
		if n >= 0xdc00 && n <= 0xdfff {
			return errors.New("lone low surrogate")
		}
		if n >= 0xd800 && n <= 0xdbff {
			if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
				return errors.New("lone high surrogate")
			}
			low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return errors.New("invalid surrogate pair")
			}
			i += 6
		}
	}
	return nil
}
