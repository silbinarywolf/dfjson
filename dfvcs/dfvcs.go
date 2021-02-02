package dfvcs

import "bytes"

type VCSDriver interface {
	Init() error
	HandleFile(path string, oursBuffer *bytes.Buffer, theirsBuffer *bytes.Buffer) (bool, error)
}
