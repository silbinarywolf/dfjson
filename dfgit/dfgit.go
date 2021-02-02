package dfgit

import (
	"bytes"
	"errors"
	"io/ioutil"
	"os/exec"
	"strings"

	"github.com/silbinarywolf/sweditor/internal/dfjson/dfvcs"
)

type GitDriver struct {
	gitPath           string
	gitTopPath        string
	conflictedFileMap map[string]bool
}

var _ dfvcs.VCSDriver = new(GitDriver)

func (vcs *GitDriver) Init() error {
	// Get time taken
	//startTime := time.Now()
	//defer func() {
	//	panic(time.Since(startTime))
	//}()

	// Reset
	vcs.conflictedFileMap = make(map[string]bool)

	// Check if we have git
	{
		path, err := exec.LookPath("git")
		if err != nil {
			return errors.New("unable to locate \"git\". Is Git installed?")
		}
		vcs.gitPath = path
	}

	// Get the top level directory
	{
		topPath, err := execCommand(vcs.gitPath, "rev-parse", "--show-toplevel")
		if err != nil {
			return err
		}
		// trim newline from execCommand
		topPath = topPath[:len(topPath)-1]
		vcs.gitTopPath = topPath
	}

	// Get the files changed
	{
		cmd := exec.Command(vcs.gitPath, "--no-pager", "diff", "--name-status")
		cmdOut, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		cmdErr, err := cmd.StderrPipe()
		if err != nil {
			return err
		}
		if err := cmd.Start(); err != nil {
			return err
		}
		errOutput, err := ioutil.ReadAll(cmdErr)
		if err != nil {
			return err
		}
		stdOutput, err := ioutil.ReadAll(cmdOut)
		if err != nil {
			return err
		}
		if len(errOutput) > 0 {
			return errors.New(string(errOutput))
		}
		changedFileList := strings.Split(string(stdOutput), "\n")
		// the last split entry is an empty line, so we cut it off
		changedFileList = changedFileList[:len(changedFileList)-1]
		for _, changedFile := range changedFileList {
			if changedFile[0] != 'M' {
				continue
			}
			// skip first letter and whitespace, just get relative path
			changedFile = changedFile[2:]
			absPath := vcs.gitTopPath + "/" + changedFile
			vcs.conflictedFileMap[absPath] = true
		}
	}
	return nil
}

func (vcs *GitDriver) HandleFile(path string, oursBuffer *bytes.Buffer, theirsBuffer *bytes.Buffer) (bool, error) {
	if _, ok := vcs.conflictedFileMap[path]; ok {
		path = path[len(vcs.gitTopPath)+1:]

		oursData, err := execCommand(vcs.gitPath, "--no-pager", "show", "HEAD:"+path)
		if err != nil {
			return false, err
		}
		if _, err := oursBuffer.WriteString(oursData); err != nil {
			return false, err
		}
		theirsData, err := execCommand(vcs.gitPath, "--no-pager", "show", "MERGE_HEAD:"+path)
		if err != nil {
			return false, err
		}
		if _, err := theirsBuffer.WriteString(theirsData); err != nil {
			return false, err
		}
		return true, nil
	}
	// Fallback to default behaviour
	return false, nil
}

func execCommand(path string, arguments ...string) (string, error) {
	cmd := exec.Command(path, arguments...)
	cmdOut, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	cmdErr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	errOutput, err := ioutil.ReadAll(cmdErr)
	if err != nil {
		return "", err
	}
	stdOutput, err := ioutil.ReadAll(cmdOut)
	if err != nil {
		return "", err
	}
	if len(errOutput) > 0 {
		return "", errors.New(string(errOutput))
	}
	return string(stdOutput), nil
}
