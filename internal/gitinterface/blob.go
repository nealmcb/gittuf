// SPDX-License-Identifier: Apache-2.0

package gitinterface

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
)

var ErrWrittenBlobLengthMismatch = errors.New("length of blob written does not match length of contents")

// ReadBlob returns the contents of a the blob referenced by blobID.
func ReadBlob(repo *git.Repository, blobID plumbing.Hash) ([]byte, error) {
	blob, err := GetBlob(repo, blobID)
	if err != nil {
		return nil, err
	}

	reader, err := blob.Reader()
	if err != nil {
		return nil, err
	}

	return io.ReadAll(reader)
}

func (r *Repository) ReadBlob(blobID string) ([]byte, error) {
	// TODO: check with cat-file -t that it's a blob
	stdOut, stdErr, err := r.executeGitCommand("cat-file", "-p", blobID)
	if err != nil {
		return nil, fmt.Errorf("unable to read blob: %s", stdErr)
	}

	return []byte(stdOut), nil
}

// WriteBlob creates a blob object with the specified contents and returns the
// ID of the resultant blob.
func WriteBlob(repo *git.Repository, contents []byte) (plumbing.Hash, error) {
	obj := repo.Storer.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)

	writer, err := obj.Writer()
	if err != nil {
		return plumbing.ZeroHash, err
	}

	length, err := writer.Write(contents)
	if err != nil {
		return plumbing.ZeroHash, err
	}

	if length != len(contents) {
		return plumbing.ZeroHash, ErrWrittenBlobLengthMismatch
	}

	return repo.Storer.SetEncodedObject(obj)
}

func (r *Repository) WriteBlob(contents []byte) (string, error) {
	stdOut, stdErr, err := r.executeGitCommandWithStdIn(contents, "hash-object", "-t", "blob", "-w", "--stdin")
	if err != nil {
		return "", fmt.Errorf("unable to write blob: %s", stdErr)
	}

	return strings.TrimSpace(stdOut), nil
}

// GetBlob returns the requested blob object.
func GetBlob(repo *git.Repository, blobID plumbing.Hash) (*object.Blob, error) {
	return repo.BlobObject(blobID)
}

// EmptyBlob returns the hash of an empty blob in a Git repository.
// Note: it is generated on the fly rather than stored as a constant to support
// SHA-256 repositories in future.
func EmptyBlob() plumbing.Hash {
	obj := memory.NewStorage().NewEncodedObject()
	obj.SetType(plumbing.BlobObject)

	return obj.Hash()
}
