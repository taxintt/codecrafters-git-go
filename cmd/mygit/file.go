package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

func getDirectoryInfo(dir string) map[string]string {
	fileInfos, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}

	temp := map[string]string{}
	for _, fileInfo := range fileInfos {
		fileName := fileInfo.Name()
		if strings.HasPrefix(fileName, ".") {
			continue
		}
		fileInfo, statErr := os.Stat(fileName)
		if statErr != nil {
			panic(err)
		}
		temp[fileInfo.Name()] = fmt.Sprint(int(fileInfo.Mode().Perm()))
	}
	return temp
}
