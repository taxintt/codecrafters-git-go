package main

import (
	"compress/zlib"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// ./your_git.sh init
func initCmd(path string) *Status {
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

// ./your_git.sh cat-file -p <blob_sha>
func catFileCmd() *Status {
	if len(os.Args) < 3 {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("specify the sha of blob object: cat-file -p <blob_sha>\n"),
		}
	}

	fullSha := os.Args[3]
	objectContent, err := catObject(fullSha)
	if err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("Error reading blob object (binary data): %s\n", err),
		}
	}

	// blob(object type) 4(size)\000test(content)
	fmt.Print(strings.Split(objectContent.String(), "\x00")[1])

	return &Status{
		exitCode: ExitCodeOK,
		err:      nil,
	}
}

// ./your_git.sh hash-object -w <file>
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
	hash, err := createHash(content)
	if err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("error creating hash"),
		}
	}
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
	object, err := os.OpenFile(objectPath(hash), os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("error opening file"),
		}
	}

	writer := zlib.NewWriter(object)
	header := []byte(fmt.Sprintf("blob %d\x00", len(content)))
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

// ./your_git.sh ls-tree --name-only <tree_sha>
func lsTreeCmd() *Status {
	if len(os.Args) < 3 {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("pass the tree object hash: ls-tree --name-only <tree_sha>\n"),
		}
	}

	fullSha := os.Args[3]
	objectContent, err := catObject(fullSha)
	if err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("Error reading tree object: %s\n", err),
		}
	}

	var result []string

	fileContentlist := strings.Split(objectContent.String(), "\x00")[1:]
	for i := 0; i < len(fileContentlist)-1; i++ {
		temp := strings.Split(fileContentlist[i], " ")
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
func createCommitCmd() *Status {
	if len(os.Args) < 3 {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("pass the tree object hash: <tree_sha>\n"),
		}
	}

	tree_sha := os.Args[2]
	commit_sha := os.Args[4]
	message := os.Args[6]

	sha, err := WriteCommitObject(tree_sha, commit_sha, message)
	if err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("error writing to commit object: %s\n", err),
		}
	}
	fmt.Printf("%x\n", sha)

	return &Status{
		exitCode: ExitCodeOK,
		err:      nil,
	}
}

// ./your_git.sh write-tree
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

// ./your_git.sh clone https://github.com/blah/blah <some_dir>
func cloneCmd() *Status {
	gitRepositoryURL := os.Args[2]
	directory := os.Args[3]

	repoPath := path.Join(".", directory)
	if err := os.MkdirAll(repoPath, 0750); err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("error creating directory: %s\n", err),
		}
	}

	status := initCmd(repoPath)
	if status.err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("error initializing git repository: %s\n", status.err),
		}
	}

	commitSha, err := fetchLatestCommitHash(gitRepositoryURL)
	log.Printf("[Debug] the sha of latest commit: %s\n", commitSha)
	if err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("error fetching latest commit hash: %s\n", err),
		}
	}

	if err := writeBranchRefFile(repoPath, "master", commitSha); err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("error writing branch ref file: %s\n", err),
		}
	}

	// Fetch objects.
	if err := fetchObjects(gitRepositoryURL, commitSha); err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("error fetching objects: %s\n", err),
		}
	}

	if err := writeFetchedObjects(repoPath); err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("error writing fetched objects: %s\n", err),
		}
	}
	// Restore files committed at the commit sha.
	if err := restoreRepository(repoPath, commitSha); err != nil {
		return &Status{
			exitCode: ExitCodeError,
			err:      fmt.Errorf("error restoring repository: %s\n", err),
		}
	}

	return &Status{
		exitCode: ExitCodeOK,
		err:      nil,
	}
}
