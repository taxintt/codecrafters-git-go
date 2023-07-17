package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
)

type TreeChild struct {
	mode string // 100XXX for blob, 40000 for tree.
	name string
	sha  string
}
type Tree struct {
	children []TreeChild
}

func traverseTree(repoPath, curDir, treeSha string) error {
	treeBuf, err := readObjectContent(repoPath, treeSha)
	if err != nil {
		return err
	}
	tree, err := parseTree(treeBuf)
	if err != nil {
		return err
	}
	log.Printf("[Debug] tree: %+v\n", tree)
	for _, child := range tree.children {
		if isBlob(child.mode) {
			// Create a file
			blobBuf, err := readObjectContent(repoPath, child.sha)
			if err != nil {
				return err
			}
			filePath := path.Join(repoPath, curDir, child.name)
			log.Printf("[Debug] write file: %s\n", filePath)
			if err := os.MkdirAll(path.Dir(filePath), 0750); err != nil && !os.IsExist(err) {
				return err
			}
			perm, err := getPerm(child.mode)
			if err != nil {
				return err
			}
			if err := ioutil.WriteFile(filePath, blobBuf, perm); err != nil {
				return err
			}
		} else {
			// traverse recursively.
			childDir := path.Join(curDir, child.name)
			if err := traverseTree(repoPath, childDir, child.sha); err != nil {
				return err
			}
		}
	}
	return nil
}

func parseTree(treeBuf []byte) (*Tree, error) {
	children := make([]TreeChild, 0)
	contentsReader := bufio.NewReader(bytes.NewReader(treeBuf))
	for {
		// Read the mode of the entry (including the space character after)
		mode, err := contentsReader.ReadString(' ')
		if err == io.EOF {
			break // We've reached the end of the file
		} else if err != nil {
			return nil, err
		}
		mode = mode[:len(mode)-1] // Trim the space suffix.
		// Read the name of the entry (including the null-byte character after)
		entryName, err := contentsReader.ReadString(0)
		if err != nil {
			return nil, err
		}
		entryName = entryName[:len(entryName)-1] // Trim the null-byte character suffix.
		sha := make([]byte, 20)
		_, err = contentsReader.Read(sha)
		if err != nil {
			return nil, err
		}
		children = append(children, TreeChild{
			name: entryName,
			mode: mode,
			sha:  fmt.Sprintf("%x", sha),
		})
	}
	tree := Tree{
		children: children,
	}
	return &tree, nil
}

func isBlob(mode string) bool {
	return strings.HasPrefix(mode, "100")
}

func getPerm(mode string) (os.FileMode, error) {
	if !isBlob(mode) {
		return 0, errors.New(fmt.Sprintf("Invalid mode: %s", mode))
	}
	perm, err := strconv.ParseInt(mode[3:], 8, 64)
	if err != nil {
		return 0, err
	}
	return os.FileMode(perm), nil
}
