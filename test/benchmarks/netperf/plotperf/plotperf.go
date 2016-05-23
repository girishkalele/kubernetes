/*
Copyright 2016 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

/*
 plotperf.go

 Parses the input CSV file and generates line and bar charts.
*/

package main

// Imports only base Golang packages
import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

var csvfile string
var suffix string

type csvDataType map[string][]float64

func init() {
	flag.StringVar(&csvfile, "c", "netperf-latest.csv", "Input CSV file")
	flag.StringVar(&suffix, "s", "", "Suffix to add to generated files")
}

func main() {
	flag.Parse()
	data, err := getData(csvfile)
	if err != nil {
		fmt.Println("Error reading input csv file", csvfile, err)
		os.Exit(1)
	}
	for k, vl := range *data {
		fmt.Printf("Key: %s ", k)
		for _, v := range vl {
			fmt.Printf("%.2f ", v)
		}
		fmt.Println(" ")
	}
	linePlot(data, suffix)
	barPlot(data, suffix)
}

// getData : Parse the CSV file into formatted data
func getData(filename string) (*csvDataType, error) {

	csvmap := make(csvDataType)
	fd, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	r := bufio.NewReader(fd)
	for true {
		line, err := r.ReadString('\n')
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		label, _, items, err := parseCsvLine(line)
		if err != nil {
			return nil, err
		}
		floatItems := []float64{}
		for _, sn := range items {
			f, err := strconv.ParseFloat(strings.Trim(sn, " "), 64)
			if err != nil {
				fmt.Println("Failed to parse numeric value in CSV file", err)
				return nil, err
			}
			floatItems = append(floatItems, f)
		}
		csvmap[label] = floatItems
	}

	return &csvmap, nil
}

func parseCsvLine(line string) (string, string, []string, error) {

	cleanedLine := strings.Replace(line, " ", "", -1)
	cleanedLine = strings.Trim(strings.Replace(line, "\n", "", -1), " ,")
	items := strings.Split(cleanedLine, ",")
	if len(items) < 2 {
		return "", "", items, fmt.Errorf("Invalid line in CSV file %s", line)
	}
	max := items[1]
	label := items[0]
	return label, max, items[2:], nil
}

func linePlot(data *csvDataType, suffix string) {

}

func barPlot(data *csvDataType, suffix string) {

}
