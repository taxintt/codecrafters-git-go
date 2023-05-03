package main

import (
	"fmt"
	"os"
)

// Usage: your_git.sh <command> <arg1> <arg2> ...
func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: mygit <command> [<args>...]\n")
		os.Exit(1)
	}

	switch command := os.Args[1]; command {
	case "init":
		initCmd()

	case "cat-file":
		catFileCmd()

	case "hash-object":
		hashObjectCmd()

	case "ls-tree":
		lsTreeCmd()

	case "write-tree":
		writeTreeCmd()

	default:
		fmt.Fprintf(os.Stderr, "Unknown command %s\n", command)
		os.Exit(1)
	}
}
