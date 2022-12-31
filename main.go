package main

import (
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	rootDir, err := parseArgs()
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	extFilter := getExtFilter()

	isAscending := getSortingOption()

	files, err := GetFiles(rootDir, extFilter)
	if err != nil {
		fmt.Println("error walking directory")
		return
	}

	groups := groupByFileSize(files)

	sort.Slice(groups, func(i, j int) bool {
		if isAscending {
			return groups[i].Size < groups[j].Size
		}
		return groups[i].Size > groups[j].Size
	})

	for _, group := range groups {
		group.print()
	}

	shouldCheckForDuplicates := askShouldCheckDups()

	if shouldCheckForDuplicates {
		showDuplicates(groups)
	}
}

func showDuplicates(groups []UserFileGroupBySize) {
	duplicateCount := 0
	duplicateGroups := make([]DuplicateGroup, 0)
	var potentialDupGp *DuplicateGroup
	var err error
	for _, group := range groups {
		potentialDupGp, duplicateCount, err = DupGroupFromSizeGroup(group, duplicateCount)
		if err == nil {
			duplicateGroups = append(duplicateGroups, *potentialDupGp)
		}
	}

	for _, dupGroup := range duplicateGroups {
		fmt.Println(dupGroup.String())
	}
}

type DuplicateGroup struct {
	Size  int64
	Hash  string
	Files []Duplicate
}

func (dp *DuplicateGroup) String() string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("%d bytes\n", dp.Size))
	sb.WriteString(fmt.Sprintf("Hash: %s\n", dp.Hash))
	for _, dup := range dp.Files {
		sb.WriteString(fmt.Sprintf("%d. %s\n", dup.Number, dup.File.Path))
	}
	return sb.String()
}

func DupGroupFromSizeGroup(group UserFileGroupBySize, lastCount int) (*DuplicateGroup, int, error) {
	if len(group.files) < 2 {
		return nil, lastCount, errors.New("need at least two files of the same size to check for duplicates")
	}
	size := group.files[0].Size
	hashes := []string{group.files[0].Hash}
	dupGroup := &DuplicateGroup{
		Size:  size,
		Files: make([]Duplicate, 0),
	}
	for index, file := range group.files {
		if index == 0 {
			continue
		}
		hashes = append(hashes, file.Hash)
		if containsString(hashes, file.Hash) {
			lastCount += 1
			dupGroup.Files = append(dupGroup.Files, *NewDuplicate(file, lastCount))
			if dupGroup.Hash == "" {
				dupGroup.Hash = file.Hash
			}
		}
	}

	// Check the first file's hash to see if it's a duplicate
	skipFirstHashes := hashes[1:]
	firstFile := group.files[0]
	if containsString(skipFirstHashes, firstFile.Hash) {
		lastCount += 1
		dupGroup.Files = append(dupGroup.Files, *NewDuplicate(firstFile, lastCount))
	}

	return dupGroup, lastCount, nil
}

func containsString(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

type Duplicate struct {
	File   UserFile
	Number int
}

func NewDuplicate(file UserFile, number int) *Duplicate {
	return &Duplicate{
		File:   file,
		Number: number,
	}
}

func groupByFileSize(files []UserFile) []UserFileGroupBySize {
	// init groups
	sizes := make([]int64, 1)
	groups := make([]UserFileGroupBySize, 0)

	contains := func(s []int64, e int64) bool {
		for _, a := range s {
			if a == e {
				return true
			}
		}
		return false
	}

	for _, file := range files {
		if file.Size != 0 {
			if !contains(sizes, file.Size) {
				sizes = append(sizes, file.Size)
				groups = append(groups, *NewUserFileGroupBySize(file.Size))
			}
		}
	}

	// add files to groups
	for index, group := range groups {
		for _, file := range files {
			if file.Size == 0 {
				continue
			}
			if group.Size == file.Size {
				group.AddFile(file)
				groups[index] = group
			}
		}
	}
	return groups
}

func parseArgs() (string, error) {
	if len(os.Args) != 2 {
		return "", errors.New("Directory is not specified")
	}
	arg := os.Args[1]
	return arg, nil
}

func getExtFilter() *ExtFilter {
	fmt.Println("Enter file format:")
	var ext string
	_, _ = fmt.Scanln(&ext)
	return NewExtFilter(ext)
}

func getSortingOption() bool {
	fmt.Println("Size sorting options:")
	fmt.Println("1. Descending")
	fmt.Println("2. Ascending")
	var answer int

	for true {
		fmt.Println("Enter a sorting option:")
		_, _ = fmt.Scanf("%d", &answer)
		if answer == 1 {
			return false
		}
		if answer == 2 {
			return true
		}
		fmt.Println()
		fmt.Println("Wrong option")
		fmt.Println()
	}
	panic("Illegal state")
}

func askShouldCheckDups() bool {
	fmt.Println()
	var answer string

	for true {
		fmt.Println("Check for duplicates")
		_, _ = fmt.Scanf("%s", &answer)
		if answer == "no" {
			return false
		}
		if answer == "yes" {
			return true
		}
		fmt.Println()
		fmt.Println("Wrong option")
		fmt.Println()
	}
	panic("Illegal state")
}

func GetFiles(directoryName string, filter *ExtFilter) ([]UserFile, error) {
	files := make([]UserFile, 0, 10)

	err := filepath.Walk(directoryName, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		if filter.ShouldFilter() {
			if filepath.Ext(path) != filter.Filter() {
				return nil
			}
		}
		files = append(files, *NewUserFile(path, info))
		return nil
	})
	return files, err
}

type ExtFilter struct {
	ext string
}

func NewExtFilter(ext string) *ExtFilter {
	return &ExtFilter{ext: "." + ext}
}

func (f *ExtFilter) ShouldFilter() bool {
	return f.ext != "."
}

func (f *ExtFilter) Filter() string {
	return f.ext
}

type UserFile struct {
	Name          string
	Path          string
	Size          int64
	FileExtension string
	Hash          string
}

func NewUserFile(path string, info os.FileInfo) *UserFile {
	file, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	md5Hash := md5.New()
	if _, err := io.Copy(md5Hash, file); err != nil {
		log.Fatal(err)
	}

	hash := fmt.Sprintf("%x", md5Hash.Sum(nil))
	return &UserFile{
		Name:          info.Name(),
		Path:          path,
		Size:          info.Size(),
		FileExtension: filepath.Ext(path),
		Hash:          hash,
	}
}

type UserFileGroupBySize struct {
	Size  int64
	files []UserFile
}

func NewUserFileGroupBySize(size int64) *UserFileGroupBySize {
	return &UserFileGroupBySize{
		Size:  size,
		files: make([]UserFile, 0),
	}
}

func (g *UserFileGroupBySize) AddFile(file UserFile) {
	if g.Size == file.Size {
		g.files = append(g.files, file)
	}
}

func (g *UserFileGroupBySize) print() {
	fmt.Println()
	fmt.Printf("%d bytes\n", g.Size)
	for _, file := range g.files {
		fmt.Println(file.Path)
	}
}
