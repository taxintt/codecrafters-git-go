package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
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
	// create data from directories
	tempMap := getDirectoryInfo(".")
	for fileName, permission := range tempMap {
		var flag string
		if strings.Contains(fileName, ".") {
			flag = "100"
			number, _ := strconv.Atoi(permission)
			permission = fmt.Sprintf("%o", number)
		} else {
			flag = "040"
			permission = "000"
		}
		fmt.Printf("%s%s %s\n", flag, permission, fileName)
	}

	// content := []byte("100644 README.md\u000071468c4444dea8f9d01270065d8fefccdf2a04e3")

	// // create hash from tempBuffer
	// hasher := sha1.New()
	// header := []byte(fmt.Sprintf("blob %d\u0000", len(content)))
	// if _, err := hasher.Write(header); err != nil {
	// 	fmt.Fprintf(os.Stderr, "error writing header to create hash")
	// 	os.Exit(1)
	// }
	// if _, err := hasher.Write(content); err != nil {
	// 	fmt.Fprintf(os.Stderr, "error writing content to create hash")
	// 	os.Exit(1)
	// }
	// hash := fmt.Sprintf("%x", hasher.Sum(nil))
	// fmt.Println(hash)

	// // create dir if dir doesn't exist
	// objectDir := fmt.Sprintf(".git/objects/%s", hash[:2])
	// if _, err := os.Stat(objectDir); errors.Is(err, os.ErrNotExist) {
	// 	err := os.MkdirAll(objectDir, 0755)
	// 	if err != nil {
	// 		fmt.Fprintf(os.Stderr, "Error creating directory: %s\n", err)
	// 	}
	// }

	// // write data to file under .git/objects
	// objectFilepath := fmt.Sprintf(".git/objects/%s/%s", hash[:2], hash[2:])
	// object, err := os.OpenFile(objectFilepath, os.O_CREATE|os.O_WRONLY, 0644)
	// if err != nil {
	// 	fmt.Fprintf(os.Stderr, "error opening file")
	// 	os.Exit(1)
	// }

	// writer := zlib.NewWriter(object)
	// if _, err := writer.Write(header); err != nil {
	// 	fmt.Fprintf(os.Stderr, "error writing header to create compressed data")
	// 	os.Exit(1)
	// }
	// if _, err := writer.Write(content); err != nil {
	// 	fmt.Fprintf(os.Stderr, "error writing content to create compressed data")
	// 	os.Exit(1)
	// }
	// writer.Close()
	// object.Close()
}
