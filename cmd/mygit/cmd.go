package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"errors"
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func initCmd() {
	for _, dir := range []string{".git", ".git/objects", ".git/refs"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directory: %s\n", err)
		}
	}

	headFileContents := []byte("ref: refs/heads/master\n")
	if err := os.WriteFile(".git/HEAD", headFileContents, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing file: %s\n", err)
	}

	fmt.Println("Initialized git directory")
}

func catFileCmd() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "specify the sha of blob object: cat-file -p <blob_sha>\n")
		os.Exit(1)
	}

	fullSha := os.Args[3]
	content, err := os.ReadFile(fmt.Sprintf(".git/objects/%s/%s", fullSha[:2], fullSha[2:]))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading blob object (binary data): %s\n", err)
	}

	objectBuffer := bytes.NewBuffer(content)
	reader, err := zlib.NewReader(objectBuffer)
	defer reader.Close()

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error decompressing blob object (binary data): %s\n", err)
		os.Exit(1)
	}

	stringBuffer := new(bytes.Buffer)
	stringBuffer.ReadFrom(reader)
	fmt.Print(strings.Split(stringBuffer.String(), "\000")[1])
}

func hashObjectCmd() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "pass the content of file: hash-object -w <file>\n")
		os.Exit(1)
	}

	// read data from file
	filePath := os.Args[3]
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading file content")
		os.Exit(1)
	}

	// create hash from tempBuffer
	hasher := sha1.New()
	header := []byte(fmt.Sprintf("blob %d\u0000", len(content)))
	if _, err := hasher.Write(header); err != nil {
		fmt.Fprintf(os.Stderr, "error writing header to create hash")
		os.Exit(1)
	}
	if _, err := hasher.Write(content); err != nil {
		fmt.Fprintf(os.Stderr, "error writing content to create hash")
		os.Exit(1)
	}
	hash := fmt.Sprintf("%x", hasher.Sum(nil))
	fmt.Println(hash)

	// create dir if dir doesn't exist
	objectDir := fmt.Sprintf(".git/objects/%s", hash[:2])
	if _, err := os.Stat(objectDir); errors.Is(err, os.ErrNotExist) {
		err := os.MkdirAll(objectDir, 0755)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directory: %s\n", err)
		}
	}

	// write data to file under .git/objects
	objectFilepath := fmt.Sprintf(".git/objects/%s/%s", hash[:2], hash[2:])
	object, err := os.OpenFile(objectFilepath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening file")
		os.Exit(1)
	}

	writer := zlib.NewWriter(object)
	if _, err := writer.Write(header); err != nil {
		fmt.Fprintf(os.Stderr, "error writing header to create compressed data")
		os.Exit(1)
	}
	if _, err := writer.Write(content); err != nil {
		fmt.Fprintf(os.Stderr, "error writing content to create compressed data")
		os.Exit(1)
	}
	writer.Close()
	object.Close()
}

func lsTreeCmd() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "pass the tree object hash: ls-tree --name-only <tree_sha>\n")
		os.Exit(1)
	}

	fullSha := os.Args[3]
	content, err := os.ReadFile(fmt.Sprintf(".git/objects/%s/%s", fullSha[:2], fullSha[2:]))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading blob object (binary data): %s\n", err)
	}

	objectBuffer := bytes.NewBuffer(content)
	reader, err := zlib.NewReader(objectBuffer)
	defer reader.Close()

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error decompressing blob object (binary data): %s\n", err)
		os.Exit(1)
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
}

func writeTreeCmd() {
	workDir, err := filepath.Abs(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading directory")
		os.Exit(1)
	}
	sha, err := WriteTreeObject(workDir)
	fmt.Printf("%x\n", sha)
}

func WriteTreeObject(dir string) (sha [20]byte, _ error) {
	// read info to create git tree object
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading directory")
		os.Exit(1)
	}

	var result []map[string]string
	for _, entry := range entries {
		var tempEntry = make(map[string]string)
		tempEntry["name"] = entry.Name()

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
			// fmt.Println(entry.Name())
			tempEntry["type"] = "40"
			tempEntry["permission"] = "000"
			sha, err = WriteTreeObject(filepath.Join(dir, entry.Name()))
		} else if entry.Type().IsRegular() {
			// fmt.Println(entry.Name())
			tempEntry["type"] = "100"
			tempEntry["permission"] = fmt.Sprintf("%3o", info.Mode())
			sha, err = WriteBlobObject(filepath.Join(dir, entry.Name()), info.Mode())
		}

		result = append(result, tempEntry)
	}

	// create plain git tree object
	var treeBuffer bytes.Buffer
	for _, entry := range result {
		log.Printf(fmt.Sprintf("%s%s %s\x00%s", entry["type"], entry["permission"], entry["name"], entry["hash"]))
		treeBuffer.WriteString(fmt.Sprintf("%s%s %s\x00%s", entry["type"], entry["permission"], entry["name"], entry["hash"]))
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
