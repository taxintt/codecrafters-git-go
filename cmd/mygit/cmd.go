package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var ExitCodeOK int = 0
var ExitCodeError int = 1

func initCmd() *Status {
	for _, dir := range []string{".git", ".git/objects", ".git/refs"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return &Status{
				exitCode: ExitCodeError,
				err:      fmt.Errorf("Error creating directory: %s\n", err.Error()),
			}
		}
	}

	headFileContents := []byte("ref: refs/heads/master\n")
	if err := os.WriteFile(".git/HEAD", headFileContents, 0644); err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("Error writing file: %s\n", err.Error()),
		}
	}

	fmt.Println("Initialized git directory")
	return &Status{
		exitCode: ExitCodeOK,
		err:      nil,
	}
}

func catFileCmd() *Status {
	if len(os.Args) < 3 {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("specify the sha of blob object: cat-file -p <blob_sha>\n"),
		}
	}

	fullSha := os.Args[3]
	content, err := os.ReadFile(fmt.Sprintf(".git/objects/%s/%s", fullSha[:2], fullSha[2:]))
	if err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("Error reading blob object (binary data): %s\n", err),
		}
	}

	objectBuffer := bytes.NewBuffer(content)
	reader, err := zlib.NewReader(objectBuffer)
	defer reader.Close()

	if err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("Error decompressing blob object (binary data): %s\n", err),
		}
	}

	stringBuffer := new(bytes.Buffer)
	stringBuffer.ReadFrom(reader)
	fmt.Print(strings.Split(stringBuffer.String(), "\000")[1])

	return &Status{
		exitCode: ExitCodeOK,
		err:      nil,
	}
}

func hashObjectCmd() *Status {
	if len(os.Args) < 3 {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("pass the content of file: hash-object -w <file>\n"),
		}
	}

	// read data from file
	filePath := os.Args[3]
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("error reading file content"),
		}
	}

	// create hash from tempBuffer
	hasher := sha1.New()
	header := []byte(fmt.Sprintf("blob %d\u0000", len(content)))
	if _, err := hasher.Write(header); err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("error writing header to create hash"),
		}
	}
	if _, err := hasher.Write(content); err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("error writing content to create hash"),
		}
	}
	hash := fmt.Sprintf("%x", hasher.Sum(nil))
	fmt.Println(hash)

	// create dir if dir doesn't exist
	objectDir := fmt.Sprintf(".git/objects/%s", hash[:2])
	if _, err := os.Stat(objectDir); errors.Is(err, os.ErrNotExist) {
		err := os.MkdirAll(objectDir, 0755)
		if err != nil {
			return &Status{
				exitCode: ExitCodeError,
				err:      fmt.Errorf("Error creating directory: %s\n", err),
			}
		}
	}

	// write data to file under .git/objects
	objectFilepath := fmt.Sprintf(".git/objects/%s/%s", hash[:2], hash[2:])
	object, err := os.OpenFile(objectFilepath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("error opening file"),
		}
	}

	writer := zlib.NewWriter(object)
	if _, err := writer.Write(header); err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("error writing header to create compressed data"),
		}
	}
	if _, err := writer.Write(content); err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("error writing content to create compressed data"),
		}
	}
	writer.Close()
	object.Close()

	return &Status{
		exitCode: ExitCodeOK,
		err:      nil,
	}
}

func lsTreeCmd() *Status {
	if len(os.Args) < 3 {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("pass the tree object hash: ls-tree --name-only <tree_sha>\n"),
		}
	}

	fullSha := os.Args[3]
	content, err := os.ReadFile(fmt.Sprintf(".git/objects/%s/%s", fullSha[:2], fullSha[2:]))
	if err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("Error reading blob object (binary data): %s\n", err),
		}
	}

	objectBuffer := bytes.NewBuffer(content)
	reader, err := zlib.NewReader(objectBuffer)
	defer reader.Close()

	if err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("Error decompressing blob object (binary data): %s\n", err),
		}
	}

	stringBuffer := new(bytes.Buffer)
	stringBuffer.ReadFrom(reader)

	var result []string

	list := strings.Split(stringBuffer.String(), "\x00")[1:]
	for i := 0; i < len(list)-1; i++ {
		temp := strings.Split(list[i], " ")
		result = append(result, temp[len(temp)-1])
	}

	sort.Strings(result)
	for _, item := range result {
		fmt.Println(item)
	}

	// to use ToLower pattern
	// sort.Slice(result, func(i, j int) bool {
	// 	return strings.ToLower(result[i]) < strings.ToLower(result[j])
	// })
	// for _, item := range result {
	// 	fmt.Println(item)
	// }
	return &Status{
		exitCode: ExitCodeOK,
		err:      nil,
	}
}

// ./your_git.sh commit-tree <tree_sha> -p <commit_sha> -m <message>
// func createCommitCmd() int {
// 	if len(os.Args) < 3 {
// 		fmt.Fprintf(os.Stderr, "pass the tree object hash: <tree_sha>\n")
// 		return ExitCodeError
// 	}

// 	tree_sha := os.Args[2]
// 	fmt.Println(tree_sha)

// 	commit_sha := os.Args[4]
// 	fmt.Println(commit_sha)

// 	sha, err := writeObject(header, treeBuffer.Bytes())
// 	if err != nil {
// 		fmt.Fprintf(os.Stderr, "error reading directory")
// 		return ExitCodeError
// 	}
// 	fmt.Printf("%x\n", sha)
// 	return ExitCodeOK
// }

func writeTreeCmd() *Status {
	workDir, err := filepath.Abs(".")
	if err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("error reading directory: %s\n", err),
		}
	}

	sha, err := WriteTreeObject(workDir)
	fmt.Printf("%x\n", sha)

	if err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("error writing tree object: %s\n", err),
		}
	}

	return &Status{
		exitCode: ExitCodeOK,
		err:      nil,
	}
}
