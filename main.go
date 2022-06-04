package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/exp/slices"
)

type labelledXML struct {
	XMLName  xml.Name `xml:"annotation"`
	Text     string   `xml:",chardata"`
	Folder   string   `xml:"folder"`
	Filename string   `xml:"filename"`
	Path     string   `xml:"path"`
	Source   struct {
		Text     string `xml:",chardata"`
		Database string `xml:"database"`
	} `xml:"source"`
	Size struct {
		Text   string `xml:",chardata"`
		Width  string `xml:"width"`
		Height string `xml:"height"`
		Depth  string `xml:"depth"`
	} `xml:"size"`
	Segmented string `xml:"segmented"`
	Object    struct {
		Text      string `xml:",chardata"`
		Name      string `xml:"name"`
		Pose      string `xml:"pose"`
		Truncated string `xml:"truncated"`
		Difficult string `xml:"difficult"`
		Bndbox    struct {
			Text string `xml:",chardata"`
			Xmin string `xml:"xmin"`
			Ymin string `xml:"ymin"`
			Xmax string `xml:"xmax"`
			Ymax string `xml:"ymax"`
		} `xml:"bndbox"`
	} `xml:"object"`
}

// Class is pseudo type for classes in dataset
type Class = string

// ClassCount is pseudo type for count of classes
type ClassCount = int

type ClassIndex = int

// Classes in dataset
const (
	GUN         Class = "gun"
	KNIFE       Class = "knife"
	WRENCH      Class = "wrench"
	FORK        Class = "fork"
	SCREWDRIVER Class = "screwdriver"
	UNKNOWN     Class = "unknown"
)

const (
	knifeIndex       ClassIndex = 0
	forkIndex        ClassIndex = 1
	gunIndex         ClassIndex = 2
	wrenchIndex      ClassIndex = 3
	screwdriverIndex ClassIndex = 4
)

// File count of classes
var (
	GunCount         ClassCount = 0
	KnifeCount       ClassCount = 0
	WrenchCount      ClassCount = 0
	ForkCount        ClassCount = 0
	ScrewdriverCount ClassCount = 0
	UnknownCount     ClassCount = 0
	TotalCount       ClassCount = 0
)

var copiedImages = make([]string, 0)

func main() {
	scanAllLabelled(true)
	scanImages()
}

func getIndexFromPath(path string) ClassIndex {
	path = strings.ToLower(path)

	if strings.Contains(path, GUN) {
		return gunIndex
	}

	if strings.Contains(path, KNIFE) {
		return knifeIndex
	}
	if strings.Contains(path, WRENCH) {
		return wrenchIndex
	}
	if strings.Contains(path, FORK) {
		return forkIndex
	}
	if strings.Contains(path, SCREWDRIVER) {
		return screwdriverIndex
	}

	return -1
}

func getClassFromPath(path string) (Class, *ClassCount) {
	path = strings.ToLower(path)

	if strings.Contains(path, GUN) {
		return GUN, &GunCount
	}

	if strings.Contains(path, KNIFE) {
		return KNIFE, &KnifeCount
	}
	if strings.Contains(path, WRENCH) {
		return WRENCH, &WrenchCount
	}
	if strings.Contains(path, FORK) {
		return FORK, &ForkCount
	}
	if strings.Contains(path, SCREWDRIVER) {
		return SCREWDRIVER, &ScrewdriverCount
	}

	return UNKNOWN, &UnknownCount
}

func scanImages() {
	err := filepath.WalkDir("dataset/images", func(path string, d fs.DirEntry, err error) error {
		if !slices.Contains(copiedImages, path) {
			class, classCount := getClassFromPath(path)
			outputFile := getOutputFileName(class, *classCount, filepath.Ext(path))
			copy(path, outputFile)
			*classCount++
		}
		return nil
	})

	if err != nil {
		log.Fatal(err)
	}
}

func copy(src, dst string) (int64, error) {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)

	copiedImages = append(copiedImages, src)
	return nBytes, err
}

func createOutputDir(dir string) string {
	outputDir := filepath.Join("output", dir)
	os.MkdirAll(outputDir, 0700)
	return outputDir
}

func getOutputFileName(class Class, classCount ClassCount, path string) string {
	return filepath.Join(createOutputDir(class), strings.Join([]string{class, strconv.Itoa(classCount)}, "_")+filepath.Ext(path))
}

func scanAllLabelled(yolo bool) {
	err := filepath.WalkDir("dataset", func(path string, d fs.DirEntry, err error) error {
		if filepath.Ext(d.Name()) == ".xml" {

			data, err := readLabelled(path)
			if err != nil {
				return err
			}

			if data == nil {
				return fmt.Errorf("Cannot read XML: " + path)
			}

			class, classCount := getClassFromPath(path)
			if class == UNKNOWN {
				class, classCount = getClassFromPath(data.Path)
			}

			classIndex := getIndexFromPath(path)
			if classIndex == -1 {
				classIndex = getIndexFromPath(data.Path)
			}

			foundImage := findCorrespondingFile(data.Path)

			if len(foundImage) != 0 {

				if !yolo {
					outputFile := getOutputFileName(class, *classCount, filepath.Ext(foundImage))
					xmlOutFile := getOutputFileName(class, *classCount, filepath.Ext(path))

					relativeImagePath, err := filepath.Rel(filepath.Dir(xmlOutFile), filepath.Dir(outputFile))
					if err != nil {
						return err
					}

					relativeImagePath = filepath.Join(relativeImagePath, filepath.Base(outputFile))

					data.Path = relativeImagePath
					data.Filename = filepath.Base(outputFile)
					data.Object.Name = class

					b, err := xml.MarshalIndent(data, "", "  ")
					b = bytes.Replace(b, []byte("&#xA;"), []byte(""), -1)
					b = bytes.Replace(b, []byte("&#x9;"), []byte(""), -1)

					if err != nil {
						return err
					}

					err = ioutil.WriteFile(xmlOutFile, b, 0644)
					if err != nil {
						return err
					}

					copy(foundImage, outputFile)
					*classCount++
				} else {
					outputFile := filepath.Join(createOutputDir("yolo"), strconv.Itoa(TotalCount)+".jpg")
					outputYoloFile := filepath.Join(createOutputDir("yolo"), strconv.Itoa(TotalCount)+".txt")

					xMax, err := strconv.Atoi(data.Object.Bndbox.Xmax)
					xMin, err := strconv.Atoi(data.Object.Bndbox.Xmin)
					yMax, err := strconv.Atoi(data.Object.Bndbox.Ymax)
					yMin, err := strconv.Atoi(data.Object.Bndbox.Ymin)

					width, err := strconv.Atoi(data.Size.Width)
					height, err := strconv.Atoi(data.Size.Height)

					if err != nil {
						return err
					}

					dw := 1.0 / float64(width)
					dh := 1.0 / float64(height)
					x := (float64(xMin+xMax))/2.0 - 1
					y := (float64(yMin+yMax))/2.0 - 1
					w := float64(xMax - xMin)
					h := float64(yMax - yMin)

					xStr := fmt.Sprintf("%f", (x * dw))
					wStr := fmt.Sprintf("%f", (w * dw))
					yStr := fmt.Sprintf("%f", (y * dh))
					hStr := fmt.Sprintf("%f", (h * dh))

					err = ioutil.WriteFile(outputYoloFile, []byte(strconv.Itoa(classIndex)+" "+xStr+" "+yStr+" "+wStr+" "+hStr), 0644)
					if err != nil {
						return err
					}

					copy(foundImage, outputFile)
					TotalCount++
				}

			}

		}
		return nil
	})

	if err != nil {
		log.Fatal(err)
	}
}

func readLabelled(filename string) (*labelledXML, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	labelled := &labelledXML{}
	xml.NewDecoder(file).Decode(&labelled)

	return labelled, nil
}

func findCorrespondingFile(p string) string {
	baseName := filepath.Base(strings.ReplaceAll(p, "\\", "/"))

	var found string

	walker := func(path string, d fs.DirEntry, err error) error {
		if !d.IsDir() && d.Name() == baseName {
			found = path
		}
		return nil
	}

	filepath.WalkDir("dataset/images", walker)

	if len(found) == 0 {
		filepath.WalkDir("dataset/allimages", walker)
	}

	return found
}
