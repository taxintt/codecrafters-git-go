package main

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"os"
	// Uncomment this block to pass the first stage!
	// "os"
)

// Usage: your_git.sh <command> <arg1> <arg2> ...
func main() {
	// You can use print statements as follows for debugging, they'll be visible when running tests.
	fmt.Println("Logs from your program will appear here!")

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: mygit <command> [<args>...]\n")
		os.Exit(1)
	}

	switch command := os.Args[1]; command {
	case "init":
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

	case "cat-file":
		if len(os.Args) < 4 {
			fmt.Fprintf(os.Stderr, "specify the sha of blob object: cat-file -p <blob_sha>\n")
			os.Exit(1)
		}

		fullSha := os.Args[3]
		content, err := os.ReadFile(fmt.Sprintf(".git/objects/%s/%s", fullSha[:2], fullSha[2:]))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading blob object (binary data): %s\n", err)
		}

		buffer := bytes.NewBuffer(content)
		r, err := zlib.NewReader(buffer)
		defer r.Close()

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error decompressing blob object (binary data): %s\n", err)
		}
		io.Copy(os.Stdout, r)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command %s\n", command)
		os.Exit(1)
	}
}
