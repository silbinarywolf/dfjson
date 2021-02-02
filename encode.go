package dfjson

import (
	"bytes"
	"encoding"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
)

// JSONFile is data structure returned from Marshal
type JSONFile struct {
	Path string
	Data []byte
}

type encodeState struct {
	Directory string
	Paths     []JSONFile
}

// Marshal returns the JSON encoding of v but differs from the standard library encoding/json
// by returning different nested file paths depending on if a struct field was tagged
// with "dfjson:distributable" or not.
//
// The purpose of this implementation is to spread out data in a way that makes
// concurrent data editing with most version control systems easier, at the cost of more hard drive reads.
//
// Data in production should not be written or read this way.
func Marshal(entryFilename string, v interface{}) ([]JSONFile, error) {
	list, err := marshalIndent(entryFilename, v, "", "\t")
	return list, err
}

func marshal(entryFilename string, v interface{}) ([]JSONFile, error) {
	var state encodeState
	if err := state.encode(entryFilename, v); err != nil {
		return nil, err
	}
	return state.Paths, nil
}

// marshalIndent applies Indent to format the output of each JSON file.
// Each JSON element in the output will begin on a new line beginning with prefix
// followed by one or more copies of indent according to the indentation nesting.
func marshalIndent(entryFilename string, v interface{}, prefix, indent string) ([]JSONFile, error) {
	list, err := marshal(entryFilename, v)
	if err != nil {
		return nil, err
	}
	for i := 0; i < len(list); i++ {
		buf := bytes.Buffer{}
		item := &list[i]
		if err := json.Indent(&buf, item.Data, prefix, indent); err != nil {
			return nil, err
		}
		item.Data = buf.Bytes()
	}
	return list, nil
}

func (state *encodeState) encode(path string, value interface{}) error {
	switch kind := reflect.TypeOf(value).Kind(); kind {
	case reflect.Struct:
		panic("Unexpected error. Must transform struct to pointer before calling encode")
	case reflect.Map:
		//mapType := reflect.TypeOf(value).Elem()
		topMapValue := reflect.ValueOf(value)
		for _, mapKey := range topMapValue.MapKeys() {
			mapValue := topMapValue.MapIndex(mapKey)
			var keyStringValue string
			if m, ok := mapKey.Interface().(encoding.TextMarshaler); ok {
				marshalText, err := m.MarshalText()
				if err != nil {
					return err
				}
				//keyStringValue = stringBytes(marshalText, true)
				keyStringValue = string(marshalText)
			} else {
				keyStringValue = fmt.Sprintf("%v", mapKey.Interface())
			}
			var data interface{}
			if mapValue.Kind() == reflect.Struct {
				data = mapValue.Addr().Interface()
			} else {
				data = mapValue.Interface()
			}
			dir := strings.ReplaceAll(filepath.Dir(path), "\\", "/")
			if err := state.encode(dir+"/"+keyStringValue+"/index.json", data); err != nil {
				return err
			}
		}
		return nil
	case reflect.Ptr:
		buf := bytes.Buffer{}
		buf.WriteRune('{')
		hasWrittenFirstField := false

		el := reflect.ValueOf(value).Elem()
		for i := 0; i < el.NumField(); i++ {
			field := el.Field(i)
			fieldType := el.Type().Field(i)

			// Ignore unexported field
			// (copy-pasted out of encoder/json package)
			{
				isUnexported := fieldType.PkgPath != ""
				if fieldType.Anonymous {
					t := fieldType.Type
					if t.Kind() == reflect.Ptr {
						t = t.Elem()
					}
					if isUnexported && t.Kind() != reflect.Struct {
						// Ignore embedded fields of unexported non-struct types.
						continue
					}
					// Do not ignore embedded fields of unexported struct types
					// since they may have exported fields.
				} else if isUnexported {
					// Ignore unexported non-embedded fields.
					continue
				}
			}
			tag := fieldType.Tag.Get("json")
			if tag == "-" {
				continue
			}
			var jsonOptions string
			jsonFieldName := tag
			if idx := strings.Index(tag, ","); idx != -1 {
				jsonFieldName = tag[:idx]
				jsonOptions = tag[idx+1:]
			}
			if jsonFieldName == "" {
				// Default to Golang struct field name
				jsonFieldName = fieldType.Name
			}

			// NOTE(Jae): 2020-01-06
			// "encoder/json" does a more robust job here checking for a ","
			// but we don't bother
			if strings.Contains(jsonOptions, "omitempty") {
				continue
			}
			if strings.Contains(jsonOptions, "string") {
				// todo(Jae): 2020-01-06
				// add support for quoting non-string values with "string" option
				// as its supported by the encoder/json package
				panic("No support for \"string\" in DFJSON.")
			}
			if tagValue, ok := fieldType.Tag.Lookup("dfjson"); ok && tagValue == "distributable" {
				var data interface{}
				if field.Kind() == reflect.Struct {
					data = field.Addr().Interface()
				} else {
					data = field.Interface()
				}
				if err := state.encode(filepath.Dir(path)+"/"+jsonFieldName+"/", data); err != nil {
					return err
				}
				continue
			}
			fieldValue, err := json.Marshal(field.Interface())
			if err != nil {
				return err
			}
			if hasWrittenFirstField {
				buf.WriteString(",")
			}
			buf.WriteString("\"" + jsonFieldName + "\":")
			buf.Write(fieldValue)
			hasWrittenFirstField = true
		}
		if hasWrittenFirstField {
			buf.WriteRune('}')
		}
		state.Paths = append(state.Paths, JSONFile{
			Path: path,
			Data: buf.Bytes(),
		})
	default:
		panic("Unhandled kind: " + kind.String())
	}
	return nil
}

// stringBytes was copied from json encoder in standard lib
// It's used to encode MarshalText properly for JSON
/* func stringBytes(s []byte, escapeHTML bool) string {
	e := bytes.Buffer{}
	e.WriteByte('"')
	start := 0
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			if htmlSafeSet[b] || (!escapeHTML && safeSet[b]) {
				i++
				continue
			}
			if start < i {
				e.Write(s[start:i])
			}
			e.WriteByte('\\')
			switch b {
			case '\\', '"':
				e.WriteByte(b)
			case '\n':
				e.WriteByte('n')
			case '\r':
				e.WriteByte('r')
			case '\t':
				e.WriteByte('t')
			default:
				// This encodes bytes < 0x20 except for \t, \n and \r.
				// If escapeHTML is set, it also escapes <, >, and &
				// because they can lead to security holes when
				// user-controlled strings are rendered into JSON
				// and served to some browsers.
				e.WriteString(`u00`)
				e.WriteByte(hex[b>>4])
				e.WriteByte(hex[b&0xF])
			}
			i++
			start = i
			continue
		}
		c, size := utf8.DecodeRune(s[i:])
		if c == utf8.RuneError && size == 1 {
			if start < i {
				e.Write(s[start:i])
			}
			e.WriteString(`\ufffd`)
			i += size
			start = i
			continue
		}
		// U+2028 is LINE SEPARATOR.
		// U+2029 is PARAGRAPH SEPARATOR.
		// They are both technically valid characters in JSON strings,
		// but don't work in JSONP, which has to be evaluated as JavaScript,
		// and can lead to security holes there. It is valid JSON to
		// escape them, so we do so unconditionally.
		// See http://timelessrepo.com/json-isnt-a-javascript-subset for discussion.
		if c == '\u2028' || c == '\u2029' {
			if start < i {
				e.Write(s[start:i])
			}
			e.WriteString(`\u202`)
			e.WriteByte(hex[c&0xF])
			i += size
			start = i
			continue
		}
		i += size
	}
	if start < len(s) {
		e.Write(s[start:])
	}
	e.WriteByte('"')
	return e.String()
}

var hex = "0123456789abcdef"

// safeSet holds the value true if the ASCII character with the given array
// position can be represented inside a JSON string without any further
// escaping.
//
// All values are true except for the ASCII control characters (0-31), the
// double quote ("), and the backslash character ("\").
var safeSet = [utf8.RuneSelf]bool{
	' ':      true,
	'!':      true,
	'"':      false,
	'#':      true,
	'$':      true,
	'%':      true,
	'&':      true,
	'\'':     true,
	'(':      true,
	')':      true,
	'*':      true,
	'+':      true,
	',':      true,
	'-':      true,
	'.':      true,
	'/':      true,
	'0':      true,
	'1':      true,
	'2':      true,
	'3':      true,
	'4':      true,
	'5':      true,
	'6':      true,
	'7':      true,
	'8':      true,
	'9':      true,
	':':      true,
	';':      true,
	'<':      true,
	'=':      true,
	'>':      true,
	'?':      true,
	'@':      true,
	'A':      true,
	'B':      true,
	'C':      true,
	'D':      true,
	'E':      true,
	'F':      true,
	'G':      true,
	'H':      true,
	'I':      true,
	'J':      true,
	'K':      true,
	'L':      true,
	'M':      true,
	'N':      true,
	'O':      true,
	'P':      true,
	'Q':      true,
	'R':      true,
	'S':      true,
	'T':      true,
	'U':      true,
	'V':      true,
	'W':      true,
	'X':      true,
	'Y':      true,
	'Z':      true,
	'[':      true,
	'\\':     false,
	']':      true,
	'^':      true,
	'_':      true,
	'`':      true,
	'a':      true,
	'b':      true,
	'c':      true,
	'd':      true,
	'e':      true,
	'f':      true,
	'g':      true,
	'h':      true,
	'i':      true,
	'j':      true,
	'k':      true,
	'l':      true,
	'm':      true,
	'n':      true,
	'o':      true,
	'p':      true,
	'q':      true,
	'r':      true,
	's':      true,
	't':      true,
	'u':      true,
	'v':      true,
	'w':      true,
	'x':      true,
	'y':      true,
	'z':      true,
	'{':      true,
	'|':      true,
	'}':      true,
	'~':      true,
	'\u007f': true,
}

// htmlSafeSet holds the value true if the ASCII character with the given
// array position can be safely represented inside a JSON string, embedded
// inside of HTML <script> tags, without any additional escaping.
//
// All values are true except for the ASCII control characters (0-31), the
// double quote ("), the backslash character ("\"), HTML opening and closing
// tags ("<" and ">"), and the ampersand ("&").
var htmlSafeSet = [utf8.RuneSelf]bool{
	' ':      true,
	'!':      true,
	'"':      false,
	'#':      true,
	'$':      true,
	'%':      true,
	'&':      false,
	'\'':     true,
	'(':      true,
	')':      true,
	'*':      true,
	'+':      true,
	',':      true,
	'-':      true,
	'.':      true,
	'/':      true,
	'0':      true,
	'1':      true,
	'2':      true,
	'3':      true,
	'4':      true,
	'5':      true,
	'6':      true,
	'7':      true,
	'8':      true,
	'9':      true,
	':':      true,
	';':      true,
	'<':      false,
	'=':      true,
	'>':      false,
	'?':      true,
	'@':      true,
	'A':      true,
	'B':      true,
	'C':      true,
	'D':      true,
	'E':      true,
	'F':      true,
	'G':      true,
	'H':      true,
	'I':      true,
	'J':      true,
	'K':      true,
	'L':      true,
	'M':      true,
	'N':      true,
	'O':      true,
	'P':      true,
	'Q':      true,
	'R':      true,
	'S':      true,
	'T':      true,
	'U':      true,
	'V':      true,
	'W':      true,
	'X':      true,
	'Y':      true,
	'Z':      true,
	'[':      true,
	'\\':     false,
	']':      true,
	'^':      true,
	'_':      true,
	'`':      true,
	'a':      true,
	'b':      true,
	'c':      true,
	'd':      true,
	'e':      true,
	'f':      true,
	'g':      true,
	'h':      true,
	'i':      true,
	'j':      true,
	'k':      true,
	'l':      true,
	'm':      true,
	'n':      true,
	'o':      true,
	'p':      true,
	'q':      true,
	'r':      true,
	's':      true,
	't':      true,
	'u':      true,
	'v':      true,
	'w':      true,
	'x':      true,
	'y':      true,
	'z':      true,
	'{':      true,
	'|':      true,
	'}':      true,
	'~':      true,
	'\u007f': true,
}
*/
