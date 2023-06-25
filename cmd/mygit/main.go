package main

import (
	"fmt"
	"os"
)

var ExitCodeOK int = 0
var ExitCodeError int = 1

type Status struct {
	exitCode int
	err      error
}

// Usage: your_git.sh <command> <arg1> <arg2> ...
func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: mygit <command> [<args>...]\n")
		os.Exit(1)
	}
	if status := run(os.Args[1:]); status.err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", status.err)
		os.Exit(1)
	}
}

func run(args []string) *Status {
	var result *Status

	switch command := os.Args[1]; command {
	case "init":
		result = initCmd()

	case "cat-file":
		result = catFileCmd()

	case "hash-object":
		result = hashObjectCmd()

	case "ls-tree":
		result = lsTreeCmd()

	case "write-tree":
		result = writeTreeCmd()

	case "commit-tree":
		result = createCommitCmd()

	default:
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("unknown command %q", command),
		}
	}

	return &Status{
		exitCode: result.exitCode,
		err:      result.err,
	}
}
