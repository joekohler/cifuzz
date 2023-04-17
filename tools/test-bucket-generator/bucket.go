package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

type TestLogLine struct {
	Action  string  `json:"Action"`
	Elapsed float64 `json:"Elapsed"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
}

const numberOfBuckets = 3
const numberOfBucketsWin = 6

/*
1. Read gotest.log file with JSON output
2. Read JSONs line-by-line
3. Look for "Action":"pass" and collect "Elapsed" time for each package
4. Collect timing for each test and package
5. Create N buckets with tested packages
6. Save into .txt files
*/
func main() {
	passingTestLogLines := getSortedPassingTestLogLines()

	writeBucketFiles(generateNBuckets(numberOfBuckets, passingTestLogLines), "linux-mac")
	writeBucketFiles(generateNBuckets(numberOfBucketsWin, passingTestLogLines), "win")
}

func getSortedPassingTestLogLines() []TestLogLine {
	testLog, err := os.ReadFile("gotest.log")
	if err != nil {
		panic(err)
	}
	testLogLines := strings.Split(string(testLog), "\n")
	passingTestLogLines := make([]TestLogLine, 0)
	for _, line := range testLogLines {
		var testLogLine TestLogLine
		_ = json.Unmarshal([]byte(line), &testLogLine)
		if testLogLine.Action != "pass" {
			continue
		}

		if testLogLine.Test != "" {
			continue
		}

		passingTestLogLines = append(passingTestLogLines, testLogLine)
	}

	sort.Slice(passingTestLogLines, func(i, j int) bool {
		return passingTestLogLines[i].Elapsed > passingTestLogLines[j].Elapsed
	})

	return passingTestLogLines
}

func sumBucket(bucket []TestLogLine) float64 {
	var bucketSum = 0.0
	for _, line := range bucket {
		bucketSum += line.Elapsed
	}
	return bucketSum
}

func generateNBuckets(numberOfBucketsWanted int, sortedPassingTestLogLines []TestLogLine) [][]TestLogLine {
	buckets := make([][]TestLogLine, numberOfBucketsWanted)

	for _, line := range sortedPassingTestLogLines {
		sort.Slice(buckets, func(i, j int) bool {
			return sumBucket(buckets[i]) < sumBucket(buckets[j])
		})
		buckets[0] = append(buckets[0], line)
	}
	return buckets
}

func writeBucketFiles(buckets [][]TestLogLine, pathPrefix string) {
	for index, bucket := range buckets {
		filePath := fmt.Sprintf("tools/test-bucket-generator/%s/bucket-%d.txt", pathPrefix, index)
		fmt.Println("Writing", filePath, "Test run time:", math.Floor(sumBucket(bucket)), "seconds with", len(bucket), "tested packages")
		var packageList string
		for _, line := range bucket {
			packageList += line.Package + "\n"
		}
		_ = os.WriteFile(filePath, []byte(packageList), 0644)
	}
}
