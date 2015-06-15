package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

type DfileConfig struct {
	Distribution string   `json:"distribution"`
	Dependencies []string `json:"dependencies"`
	Buildtags    string   `json:"buildtags"`
	Markers      []string `json:"markers:`
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func genDockerfileName() (string, error) {
	file, err := ioutil.ReadFile("/etc/redhat-release")
	if err != nil {
		return "Dockerfile", nil
	}
	line := strings.Split(string(file), " ")
	os_str := line[0]
	switch os_str {
	case "Fedora", "Centos":
		break
	case "red":
		os_str = "RHEL"
		break
	default:
		return "Dockerfile", nil
	}
	return "patches/Dockerfile" + os_str, nil
}

func patchLines(patched string, original string, patchText string) error {
	patchedFile, err := os.Create(patched)
	check(err)
	w := bufio.NewWriter(patchedFile)
	defer w.Flush()
	dfConfig := new(DfileConfig)
	patchJsonFile, err := os.Open(patchText)
	check(err)
	defer patchJsonFile.Close()
	jsonParser := json.NewDecoder(patchJsonFile)
	check(err)
	err = jsonParser.Decode(&dfConfig)
	check(err)
	origDf, err := os.Open(original)
	check(err)
	defer origDf.Close()
	scanner := bufio.NewScanner(origDf)
	scanner.Split(bufio.ScanLines)
	i := 0 // will increment to avoid multiple writes of same lines
	for scanner.Scan() {
		if i == 0 {
			if !strings.Contains(scanner.Text(), dfConfig.Markers[0]) {
				fmt.Fprintln(w, scanner.Text())
			} else {
				fmt.Fprintln(w, scanner.Text())
				fmt.Fprintln(w, dfConfig.Distribution)
				for _, dep := range dfConfig.Dependencies {
					fmt.Fprintln(w, dep)
				}
				i++
			}
		} else if i == 1 {
			if strings.Contains(scanner.Text(), dfConfig.Markers[1]) {
				fmt.Fprintln(w, scanner.Text())
				i++
			}
		} else if i == 2 {
			if !strings.Contains(scanner.Text(), dfConfig.Markers[2]) {
				fmt.Fprintln(w, scanner.Text())
			} else {
				fmt.Fprintln(w, scanner.Text())
				fmt.Fprintln(w, dfConfig.Buildtags)
				i++
			}
		} else if i == 3 {
			if strings.Contains(scanner.Text(), dfConfig.Markers[3]) {
				fmt.Fprintln(w, scanner.Text())
				i++
			}
		} else {
			fmt.Fprintln(w, scanner.Text())
		}
	}
	return nil
}

func main() {
	patchedDockerfile, err := genDockerfileName()
	if err != nil {
		log.Fatal(err)
	}
	if patchedDockerfile != "Dockerfile" {
		err = patchLines(patchedDockerfile, "Dockerfile", "patches/FedoraPatch.json")
		if err != nil {
			log.Fatal(err)
		}
	}
	fmt.Println(patchedDockerfile)
}
