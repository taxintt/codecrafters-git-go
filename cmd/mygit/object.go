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

func objectPath(sha string) string {
	return filepath.Join(".git", "objects", sha[:2], sha[2:])
}
