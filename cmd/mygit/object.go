package main

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	objCommit   = 1
	objTree     = 2
	objBlob     = 3
	objTag      = 4
	objOfsDelta = 6
	objRefDelta = 7

	msbMask      = uint8(0b10000000)
	remMask      = uint8(0b01111111)
	objMask      = uint8(0b01110000)
	firstRemMask = uint8(0b00001111)
)

var (
	// Fetched object. Map from sha1 to the object.
	shaToObj map[string]Object = make(map[string]Object)
)

type GitObjectReader struct {
	objectFileReader *bufio.Reader
	ContentSize      int64
	Type             string // "tree", "commit", "blob"
	Sha              string
}

type Object struct {
	Type byte // object type.
	Buf  []byte
}

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

func fetchLatestCommitHash(repositoryURL string) (string, error) {
	// $ curl 'https://github.com/taxintt/codecrafters-git-go/info/refs?service=git-upload-pack' --output -
	// 2023/06/27 23:40:54 SHA: 4b825dc642cb6eb9a060e54bf8d69288fbee4904
	// 001e# service=git-upload-pack
	// 0000
	// 0155 39065120688df73291eb9ec890bd5fd72e2bc9f1 HEADmulti_ack thin-pack side-band side-band-64k ofs-delta shallow deepen-since deepen-not deepen-relative no-progress include-tag multi_ack_detailed allow-tip-sha1-in-want allow-reachable-sha1-in-want no-done symref=HEAD:refs/heads/master filter object-format=sha1 agent=git/github-3b381533b78b
	// 003f 39065120688df73291eb9ec890bd5fd72e2bc9f1 refs/heads/master
	// 0000%
	resp, err := http.Get(fmt.Sprintf("%s/info/refs?service=git-upload-pack", repositoryURL))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	buf := bytes.NewBuffer([]byte{})
	if _, err := io.Copy(buf, resp.Body); err != nil {
		return "", err
	}

	reader := bufio.NewReader(buf)
	// read "001e# service=git-upload-pack\n"
	if _, err := readPacketLine(reader); err != nil {
		return "", err
	}
	// read "0000"
	if _, err := readPacketLine(reader); err != nil {
		return "", err
	}
	// read "0155 <commit sha> HEADmulti_ack..."
	// 0155 39065120688df73291eb9ec890bd5fd72e2bc9f1 HEADmulti_ack
	head, err := readPacketLine(reader)
	if err != nil {
		return "", err
	}

	// extract commit sha from head
	commitHash := strings.Split(string(head), " ")[0]
	return commitHash, nil
}

// read packet line sequentially from reader
func readPacketLine(reader io.Reader) ([]byte, error) {
	// e.g.) string(hex)=001e → size=30
	hex := make([]byte, 4)
	if _, err := reader.Read(hex); err != nil {
		return []byte{}, err
	}
	size, err := strconv.ParseInt(string(hex), 16, 64)
	if err != nil {
		return []byte{}, err
	}

	// Return immediately for "0000".
	if size == 0 {
		return []byte{}, nil
	}

	// read content and write to buf
	buf := make([]byte, size-4)
	if _, err := reader.Read(buf); err != nil {
		return []byte{}, err
	}
	return buf, nil
}

// write $repo/.git/refs/heads/<branch>
func writeBranchRefFile(repoPath string, branch string, commitSha string) error {
	refFilePath := path.Join(repoPath, ".git", "refs", "heads", branch)
	if err := os.MkdirAll(path.Dir(refFilePath), 0750); err != nil && !os.IsExist(err) {
		return err
	}
	refFileContent := []byte(commitSha)
	if err := ioutil.WriteFile(refFilePath, refFileContent, 0644); err != nil {
		return err
	}
	return nil
}

func fetchObjects(gitRepositoryURL, commitSha string) error {
	// Reference discovery
	packfileBuf := fetchPackfile(gitRepositoryURL, commitSha)

	sign := packfileBuf[:4]
	version := binary.BigEndian.Uint32(packfileBuf[4:8])
	numObjects := binary.BigEndian.Uint32(packfileBuf[8:12])
	log.Printf("[Debug] packfile sign: %s\n", string(sign))
	log.Printf("[Debug] version: %d\n", version)
	log.Printf("[Debug] num objects: %d\n", numObjects)

	checksumLen := 20
	calculatedChecksum := packfileBuf[len(packfileBuf)-checksumLen:]
	storedChecksum := sha1.Sum(packfileBuf[:len(packfileBuf)-checksumLen])
	if !bytes.Equal(storedChecksum[:], calculatedChecksum) {
		log.Printf("[Error] expected checksum: %v, but got: %v", storedChecksum, calculatedChecksum)
	}

	headerLen := 12
	bufReader := bytes.NewReader(packfileBuf[headerLen:])
	for {
		err := readObject(bufReader)
		if err != nil {
			return err
		}
		if bufReader.Len() <= checksumLen {
			log.Printf("[Debug] remaining buf len: %d\n", bufReader.Len())
			break
		}
	}

	return nil
}

func fetchPackfile(gitUrl, commitSha string) []byte {
	buf := bytes.NewBuffer([]byte{})

	// Without progress.
	buf.WriteString(packetLine(fmt.Sprintf("want %s no-progress\n", commitSha)))
	buf.WriteString("0000")
	buf.WriteString(packetLine("done\n"))
	uploadPackUrl := fmt.Sprintf("%s/git-upload-pack", gitUrl)
	log.Printf("[Debug] url: %s\n", uploadPackUrl)

	// contentType := "application/x-git-upload-pack-request"
	// resp, err := http.Post(url, contentType, buf)
	resp, err := http.Post(uploadPackUrl, "", buf)
	if err != nil {
		log.Fatalf("[Error] Error in git-upload-pack request: %v\n", err)
	}
	// log.Printf("[Debug] resp: %+v\n", resp)
	result := bytes.NewBuffer([]byte{})
	if _, err := io.Copy(result, resp.Body); err != nil {
		log.Fatal(err)
	}
	// log.Printf("[Debug] resp body: %v\n", result)
	packfileBuf := result.Bytes()[8:] // skip "0008NAK\n"
	return packfileBuf
}

func packetLine(rawLine string) string {
	size := len(rawLine) + 4
	return fmt.Sprintf("%04x%s", size, rawLine)
}

func readSha(reader io.Reader) (string, error) {
	sha := make([]byte, 20)
	if _, err := reader.Read(sha); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha), nil
}

// Read objects.
func readObject(reader *bytes.Reader) error {
	objType, objLen, err := readObjectTypeAndLen(reader)
	if err != nil {
		return err
	}
	// log.Printf("[Debug] obj type: %d\n", objType)
	// log.Printf("[Debug] obj len: %d\n", objLen)
	// log.Printf("[Debug] read data: %x\n", data[:n])
	if objType == objRefDelta {
		baseObjSha, err := readSha(reader)
		if err != nil {
			return err
		}
		baseObj, ok := shaToObj[baseObjSha]
		if !ok {
			return errors.New(fmt.Sprintf("Unknown obj sha: %s", baseObjSha))
		}
		decompressed, err := decompressObject(reader)
		if err != nil {
			return err
		}
		// log.Printf("[Debug] decompressed len: %d\n", decompressed.Len())
		// log.Printf("[Debug] decompressed: %x\n", decompressed.Bytes()[:20])
		deltified, err := readDeltified(decompressed, &baseObj)
		if err != nil {
			return err
		}
		// log.Printf("[Debug] deltified: %x\n", deltified.Bytes())
		// log.Printf("[Debug] deltified len: %d\n", deltified.Len())
		// log.Printf("[Debug] deltified: %s\n", string(deltified.Bytes()))
		obj := Object{
			Type: baseObj.Type,
			Buf:  deltified.Bytes(),
		}
		if err := saveObj(&obj); err != nil {
			return err
		}
	} else if objType == objOfsDelta {
		// TODO : Implement.
		return errors.New("Unsupported")
	} else {
		decompressed, err := decompressObject(reader)
		if err != nil {
			return err
		}
		if objLen != decompressed.Len() {
			return errors.New(fmt.Sprintf("Expected obj len: %d, but got: %d", objLen, decompressed.Len()))
		}
		obj := Object{
			Type: objType,
			Buf:  decompressed.Bytes(),
		}
		if err := saveObj(&obj); err != nil {
			return err
		}
	}
	return nil
}

// Read objects. Update data.
func readObjectTypeAndLen(reader *bytes.Reader) (byte, int, error) {
	num := 0
	b, err := reader.ReadByte()
	if err != nil {
		return 0, 0, err
	}
	objType := (b & objMask) >> 4
	num += int(b & firstRemMask)
	if (b & msbMask) == 0 {
		return objType, num, nil
	}
	i := 0
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return 0, 0, err
		}
		num += int(b) << (4 + 7*i)
		if (b & msbMask) == 0 {
			break
		}
		i++
	}
	// log.Printf("[Debug] varint num: %d\n", num)
	// log.Printf("[Debug] read data: %b\n", data[:i+1])
	return objType, num, nil
}

func decompressObject(reader *bytes.Reader) (*bytes.Buffer, error) {
	decompressedReader, err := zlib.NewReader(reader)
	if err != nil {
		return nil, err
	}
	decompressed := bytes.NewBuffer([]byte{})
	if _, err := io.Copy(decompressed, decompressedReader); err != nil {
		return nil, err
	}
	return decompressed, nil
}

func readDeltified(reader *bytes.Buffer, baseObj *Object) (*bytes.Buffer, error) {
	// srcObjLen, err := binary.ReadUvarint(reader)
	_, err := binary.ReadUvarint(reader)
	if err != nil {
		return nil, err
	}
	// log.Printf("[Debug] base len: %d\n", srcObjLen)
	dstObjLen, err := binary.ReadUvarint(reader)
	if err != nil {
		return nil, err
	}
	// log.Printf("[Debug] deltified len: %d\n", dstObjLen)
	result := bytes.NewBuffer([]byte{})
	for reader.Len() > 0 {
		firstByte, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		// log.Printf("[Debug] first byte: %b\n", firstByte)
		if (firstByte & msbMask) == 0 {
			// Add new data.
			n := int64(firstByte & remMask)
			if _, err := io.CopyN(result, reader, n); err != nil {
				return nil, err
			}
		} else { // msb == 1
			// Copy data.
			offset := 0
			size := 0
			// Check offset byte.
			for i := 0; i < 4; i++ {
				if (firstByte>>i)&1 > 0 { // i-bit is present.
					b, err := reader.ReadByte()
					if err != nil {
						return nil, err
					}
					offset += int(b) << (i * 8)
				}
			}
			// Check size byte.
			for i := 4; i < 7; i++ {
				if (firstByte>>i)&1 > 0 { // i-bit is present.
					b, err := reader.ReadByte()
					if err != nil {
						return nil, err
					}
					size += int(b) << ((i - 4) * 8)
				}
			}
			// log.Printf("[Debug] offset: %d\n", offset)
			// log.Printf("[Debug] size: %d\n", size)
			// log.Printf("[Debug] size: %b\n", size)
			if _, err := result.Write(baseObj.Buf[offset : offset+size]); err != nil {
				return nil, err
			}
		}
	}
	if result.Len() != int(dstObjLen) {
		return nil, errors.New(fmt.Sprintf("Invalid deltified buf: expected: %d, but got: %d", dstObjLen, result.Len()))
	}
	return result, nil
}

func saveObj(o *Object) error {
	objSha, err := o.sha()
	if err != nil {
		return err
	}
	shaToObj[objSha] = *o
	// log.Printf("[Debug] obj sha: %s\n", objSha)
	// log.Printf("[Debug] actual obj len: %d\n", len(o.Buf))
	return nil
}

func (o *Object) sha() (string, error) {
	b, err := o.wrappedBuf()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha1.Sum(b)), nil
}

// Write objects in shaToObj to .git/objects.
func writeFetchedObjects(repoPath string) error {
	for _, object := range shaToObj {
		b, err := object.wrappedBuf()
		if err != nil {
			return err
		}
		if _, err := writeGitObject(repoPath, b); err != nil {
			return err
		}
	}
	return nil
}

func (o *Object) wrappedBuf() ([]byte, error) {
	t, err := o.typeString()
	if err != nil {
		return []byte{}, err
	}
	wrappedBuf, err := wrapContent(o.Buf, t)
	if err != nil {
		return []byte{}, err
	}
	return wrappedBuf.Bytes(), nil
}

// Write the git object and return the sha1.
func writeGitObject(repoPath string, object []byte) (string, error) {
	blobSha := fmt.Sprintf("%x", sha1.Sum(object))
	// log.Printf("[Debug] object sha: %s\n", blobSha)

	objectFilePath := path.Join(repoPath, ".git", "objects", blobSha[:2], blobSha[2:])
	// log.Printf("[Debug] object file path: %s\n", objectFilePath)
	if err := os.MkdirAll(path.Dir(objectFilePath), 0755); err != nil {
		return "", err
	}
	objectFile, err := os.Create(objectFilePath)
	if err != nil {
		return "", err
	}
	compresssedFileWriter := zlib.NewWriter(objectFile)
	if _, err = compresssedFileWriter.Write(object); err != nil {
		return "", err
	}
	if err := compresssedFileWriter.Close(); err != nil {
		return "", err
	}
	return blobSha, nil
}

func (o *Object) typeString() (string, error) {
	switch o.Type {
	case objCommit:
		return "commit", nil
	case objTree:
		return "tree", nil
	case objBlob:
		return "blob", nil
	default:
		return "", errors.New(fmt.Sprintf("Invalid type: %d", o.Type))
	}
}

// Wrap content and returns a git object.
func wrapContent(contents []byte, objectType string) (*bytes.Buffer, error) {
	outerContents := bytes.NewBuffer([]byte{})
	outerContents.WriteString(fmt.Sprintf("%s %d\x00", objectType, len(contents)))
	if _, err := io.Copy(outerContents, bytes.NewReader(contents)); err != nil {
		return nil, err
	}
	return outerContents, nil
}

func restoreRepository(repoPath, commitSha string) error {
	// Parse commit and get tree sha.
	commitBuf, err := readObjectContent(repoPath, commitSha)
	if err != nil {
		return err
	}
	log.Printf("[Debug] latest commit sha: %s\n", commitSha)
	log.Printf("[Debug] latest commit buf: %s\n", string(commitBuf))
	commitReader := bufio.NewReader(bytes.NewReader(commitBuf))
	treePrefix, err := commitReader.ReadString(' ')
	if err != nil {
		return err
	}
	if treePrefix != "tree " {
		return errors.New(fmt.Sprintf("Invalid commit blob: %s", string(commitBuf)))
	}
	treeSha, err := commitReader.ReadString('\n')
	if err != nil {
		return err
	}
	treeSha = treeSha[:len(treeSha)-1] // Strip newline.
	// Traverse tree objects.
	if err := traverseTree(repoPath, "", treeSha); err != nil {
		return err
	}
	return nil
}

func readObjectContent(repoPath, objSha string) ([]byte, error) {
	objReader, err := NewGitObjectReader(repoPath, objSha)
	if err != nil {
		return []byte{}, err
	}
	contents, err := objReader.ReadContents()
	if err != nil {
		return []byte{}, err
	}
	return contents, nil
}

func NewGitObjectReader(repoPath, objectSha string) (GitObjectReader, error) {
	objectFilePath := path.Join(repoPath, ".git", "objects", objectSha[:2], objectSha[2:])
	objectFile, err := os.Open(objectFilePath)
	if err != nil {
		return GitObjectReader{}, err
	}
	objectFileDecompressed, err := zlib.NewReader(objectFile)
	if err != nil {
		return GitObjectReader{}, err
	}
	objectFileReader := bufio.NewReader(objectFileDecompressed)
	// Read the object type (includes the space character after).
	// e.g. tree for tree object.
	objectType, err := objectFileReader.ReadString(' ')
	if err != nil {
		return GitObjectReader{}, err
	}
	objectType = objectType[:len(objectType)-1] // Remove the trailing space character
	// Read the object size (includes the null byte after)
	// e.g. 100 as the ascii string.
	objectSizeStr, err := objectFileReader.ReadString(0)
	if err != nil {
		return GitObjectReader{}, err
	}
	objectSizeStr = objectSizeStr[:len(objectSizeStr)-1] // Remove the trailing null byte
	size, err := strconv.ParseInt(objectSizeStr, 10, 64)
	if err != nil {
		return GitObjectReader{}, err
	}
	return GitObjectReader{
		objectFileReader: objectFileReader,
		Type:             objectType,
		Sha:              objectSha,
		ContentSize:      size,
	}, nil
}

func (g *GitObjectReader) ReadContents() ([]byte, error) {
	contents := make([]byte, g.ContentSize)
	if _, err := io.ReadFull(g.objectFileReader, contents); err != nil {
		return []byte{}, err
	}
	return contents, nil
}
