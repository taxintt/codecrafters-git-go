package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"
)

func WriteTreeObject(dir string) (sha [20]byte, _ error) {
	// read info to create git tree object
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading directory")
		os.Exit(1)
	}

	var treeBuffer bytes.Buffer
	for _, entry := range entries {
		var mode, permission string
		var sha [20]byte
		info, err := entry.Info()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading file info")
			os.Exit(1)
		}

		if entry.Type().IsDir() {
			if entry.Name() == ".git" { // Skip .git directory
				log.Println("skip .git directory")
				continue
			}
			mode = "40"
			permission = "000"
			sha, err = WriteTreeObject(filepath.Join(dir, entry.Name()))
		} else if entry.Type().IsRegular() {
			mode = "100"
			permission = fmt.Sprintf("%3o", info.Mode())
			sha, err = WriteBlobObject(filepath.Join(dir, entry.Name()), info.Mode())
		}

		treeBuffer.WriteString(fmt.Sprintf("%s%s %s\x00%s", mode, permission, entry.Name(), sha))
	}

	header := fmt.Sprintf("tree %d\x00", treeBuffer.Len())
	return writeObject(header, treeBuffer.Bytes())
}

func WriteBlobObject(file string, mode fs.FileMode) (sha [20]byte, _ error) {
	content, err := ioutil.ReadFile(file)
	if err != nil {
		return sha, err
	}
	// header format = "blob #{content.bytesize}\0"
	// see https://git-scm.com/book/en/v2/Git-Internals-Git-Objects for details.
	header := fmt.Sprintf("blob %d\x00", len(content))
	log.Printf("Write blob: %s", file)
	return writeObject(header, content)
}

func WriteCommitObject(treeSha string, commit_sha string, message string) (sha [20]byte, _ error) {
	now := time.Now().Local()
	timestamp := fmt.Sprintf("%d %s", now.Unix(), now.Format("-0700"))

	content := fmt.Sprintf("tree %s\n", treeSha)
	content += fmt.Sprintf("parent %s\n", commit_sha)
	content += fmt.Sprintf("author %s <dummy@example.com> %s\n", "test", timestamp)
	content += fmt.Sprintf("committer %s <dummy@example.com> %s\n\n", "test", timestamp)
	content += fmt.Sprintf("%s\n", message)
	return writeObject(fmt.Sprintf("commit %d\x00", len(message)), []byte(content))
}

func writeObject(header string, content []byte) (sha [20]byte, _ error) {
	var data bytes.Buffer
	if _, err := data.WriteString(header); err != nil {
		return sha, err
	}
	if _, err := data.Write(content); err != nil {
		return sha, err
	}
	// calculate SHA1 from header and content
	sha = sha1.Sum(data.Bytes())
	shaStr := fmt.Sprintf("%x", sha)
	log.Printf("SHA: %x", sha)

	path := objectPath(shaStr)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		// already exists or unexpected errors
		return sha, err
	}
	if err := os.Mkdir(filepath.Dir(path), 0750); err != nil && !os.IsExist(err) {
		return sha, err
	}
	object, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return sha, err
	}
	writer := zlib.NewWriter(object)
	if _, err := writer.Write(data.Bytes()); err != nil {
		return sha, err
	}
	writer.Close()
	object.Close()
	return sha, nil
}

func catObject(sha string) (*bytes.Buffer, error) {
	content, err := os.ReadFile(objectPath(sha))
	if err != nil {
		return nil, fmt.Errorf("Error reading blob object: %s\n", err)
	}

	reader, err := zlib.NewReader(bytes.NewBuffer(content))
	defer reader.Close()

	if err != nil {
		return nil, fmt.Errorf("Error reading blob object: %s\n", err)
	}

	fileContentBuffer := new(bytes.Buffer)
	fileContentBuffer.ReadFrom(reader)
	return fileContentBuffer, nil
}

func objectPath(sha string) string {
	return filepath.Join(".git", "objects", sha[:2], sha[2:])
}

func createHash(content []byte) (string, error) {
	hasher := sha1.New()
	header := []byte(fmt.Sprintf("blob %d\x00", len(content)))
	if _, err := hasher.Write(header); err != nil {
		return "", fmt.Errorf("error writing content to create hash: %s", err)
	}
	if _, err := hasher.Write(content); err != nil {
		return "", fmt.Errorf("error writing content to create hash: %s", err)
	}
	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}
