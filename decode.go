package dfjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/karrick/godirwalk"
	"github.com/silbinarywolf/sweditor/internal/dfjson/dfvcs"
)

type decodeState struct {
	buf              bytes.Buffer
	incomingBuf      bytes.Buffer
	vscDriver        dfvcs.VCSDriver
	hasMergeConflict bool
}

// truncateLastBracket returns true if the operation happened.
func (state *decodeState) truncateLastBracket() bool {
	data := state.buf.Bytes()
	i := 0
	lastBracketIndex := -1
	for i < len(data) {
		r, n := utf8.DecodeRune(data[i:])
		if r == '}' {
			lastBracketIndex = i
		}
		i += n
	}
	if lastBracketIndex != -1 {
		state.buf.Truncate(lastBracketIndex)
		state.incomingBuf.Truncate(lastBracketIndex)
		return true
	}
	return false
}

// Unmarshal parses the JSON-encoded data and stores the result
// in the value pointed to by v. If v is nil or not a pointer,
// Unmarshal returns an InvalidUnmarshalError.
//
// It differs from the standard library encoding/json
// by loading data from nested files depending on if a struct field was tagged
// with "dfjson:distributable" or not.
//
// The purpose of this implementation is to spread out data in a way that makes
// concurrent data editing with most version control systems easier, at the cost of more hard drive reads.
//
// Data in production should not be written or read this way.
func Unmarshal(entryFilename string, v interface{}, incomingV interface{}, vcsDriver dfvcs.VCSDriver) (hasMergeConflict bool, err error) {
	decodeType := reflect.TypeOf(v)
	if decodeType.Kind() != reflect.Ptr {
		return false, errors.New("Must provide pointer value")
	}
	var state decodeState
	state.vscDriver = vcsDriver
	if state.vscDriver != nil {
		if err := state.vscDriver.Init(); err != nil {
			return false, err
		}
	}
	absEntryFilename, err := filepath.Abs(entryFilename)
	if err != nil {
		return false, err
	}
	// normalize paths to use / for every OS, even Windows
	absEntryFilename = strings.ReplaceAll(absEntryFilename, "\\", "/")
	state.decode(absEntryFilename)
	bufBytes := state.buf.Bytes()
	if err := json.Unmarshal(bufBytes, v); err != nil {
		// DEBUG: Check state of JSON
		panic("failed to parse:" + string(bufBytes) + "\n" + err.Error())
		return false, err
	}
	if state.hasMergeConflict {
		incomingBufBytes := state.incomingBuf.Bytes()
		if err := json.Unmarshal(incomingBufBytes, incomingV); err != nil {
			// DEBUG: Check state of JSON
			panic("failed to parse incoming:" + state.incomingBuf.String() + "\n" + err.Error())
			return false, err
		}
		// panic("we have a conflict!")
	}
	// DEBUG: Check state of JSON
	//panic("succeeded in parsing:" + state.buf.String())
	return state.hasMergeConflict, nil
}

func (state *decodeState) WriteAll(b []byte) error {
	if _, err := state.buf.Write(b); err != nil {
		return err
	}
	if _, err := state.incomingBuf.Write(b); err != nil {
		return err
	}
	return nil
}

func (state *decodeState) WriteRuneAll(r rune) error {
	if _, err := state.buf.WriteRune(r); err != nil {
		return err
	}
	if _, err := state.incomingBuf.WriteRune(r); err != nil {
		return err
	}
	return nil
}

func (state *decodeState) WriteStringAll(str string) error {
	if _, err := state.buf.WriteString(str); err != nil {
		return err
	}
	if _, err := state.incomingBuf.WriteString(str); err != nil {
		return err
	}
	return nil
}

func (state *decodeState) decode(path string) {
	hasOpenedBracket := false
	hasClosingBracket := false

	// Read JSON entry file (if it exists)
	{
		fileHandledByVCSDriver := false
		if state.vscDriver != nil {
			var err error
			fileHandledByVCSDriver, err = state.vscDriver.HandleFile(path, &state.buf, &state.incomingBuf)
			if err != nil {
				panic(err)
			}
			if fileHandledByVCSDriver {
				state.hasMergeConflict = true

				// We have an entry point file, and so
				// we don't need to insert an opening or closing bracket
				// into the JSON stream
				hasOpenedBracket = true
				hasClosingBracket = true
			}
		}
		if !fileHandledByVCSDriver {
			f, err := os.Open(path)
			if err != nil && !os.IsNotExist(err) {
				// if error is not a "file does not exist" error
				panic(err)
			}
			if f != nil {
				b, err := ioutil.ReadAll(f)
				if err != nil {
					f.Close()
					panic(err)
				}
				f.Close()

				if err := state.WriteAll(b); err != nil {
					panic(err)
				}

				// We have an entry point file, and so
				// we don't need to insert an opening or closing bracket
				// into the JSON stream
				hasOpenedBracket = true
				hasClosingBracket = true
			}
		}
	}

	if !hasOpenedBracket {
		if err := state.WriteRuneAll('{'); err != nil {
			panic(err)
		}
	}

	// Read distributed data
	{
		topDir := filepath.Dir(path)
		topDir = strings.ReplaceAll(topDir, "\\", "/")
		dirList, err := godirwalk.ReadDirents(topDir, nil)
		if err != nil {
			panic(err)
		}
		hasWrittenFirstField := false
		for _, fileOrDir := range dirList {
			if !fileOrDir.IsDir() {
				continue
			}
			if hasWrittenFirstField {
				if err := state.WriteStringAll(","); err != nil {
					panic(err)
				}
			} else {
				if state.truncateLastBracket() {
					if err := state.WriteStringAll(","); err != nil {
						panic(err)
					}
					hasClosingBracket = false
				}
			}

			dir := fileOrDir.Name()

			// Key of map is the directory name
			if err := state.WriteStringAll("\"" + dir + "\":"); err != nil {
				panic(err)
			}
			path := topDir + "/" + strings.ReplaceAll(dir, "\\", "/") + "/index.json"
			state.decode(path)
			hasWrittenFirstField = true
		}
	}

	if !hasClosingBracket {
		if err := state.WriteRuneAll('}'); err != nil {
			panic(err)
		}
	}
}
