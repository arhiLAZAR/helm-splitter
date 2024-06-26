package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

const tmpDir = "helm_splitter_tmp"

var overwrite, debug bool

// TODO: use config instead of pre-defined map
var shortcutMap = map[string]string{
	"ServiceAccount":     "sa",
	"ClusterRole":        "crol",
	"ClusterRoleBinding": "crb",
	"Role":               "rol",
	"RoleBinding":        "rb",
	"Service":            "svc",
	"Deployment":         "dep",
	"APIService":         "asvc",
	"Secret":             "sec",
	"ConfigMap":          "cm",
	"StatefulSet":        "ss",
}

type ManifestStruct struct {
	Kind     string         `yaml:"kind"`
	Metadata MetadataStruct `yaml:"metadata"`
}

type MetadataStruct struct {
	Name string `yaml:"name"`
}

func main() {

	// Read input params
	var namespace, helmRepo, helmChart, helmChartVersion, customValues string

	flag.StringVar(&namespace, "namespace", "", "target k8s namespace")
	flag.StringVar(&helmRepo, "repository", "", "helm repository")
	flag.StringVar(&helmChart, "chart", "", "helm chart name")
	flag.StringVar(&helmChartVersion, "version", "", "helm chart version, default: <latest>")
	flag.StringVar(&customValues, "custom-values-file", "", "file with custom values")
	flag.BoolVar(&overwrite, "overwrite", false, "overwrite existing output files, default: false")
	flag.BoolVar(&debug, "debug", false, "debug")

	flag.Parse()

	printDebug("Input values:\nNamespace: %v\nRepository: %v\nChart: %v\nVersion: %v\nCustom Values: %v\nDebug: %t\n", namespace, helmRepo, helmChart, helmChartVersion, customValues, debug)

	if namespace == "" || helmChart == "" || helmRepo == "" {
		fmt.Println("ERROR! Missing parameters.")
		fmt.Println("\"--namespace\", \"--repository\" and \"--chart\" MUST be specified!")
		os.Exit(1)
	}

	if helmChartVersion != "" {
		helmChartVersion = " --version " + helmChartVersion
	}

	if customValues != "" {
		customValues = " --values " + customValues
	}

	// Run helm commands
	printDebug("Adding helm repository\n")
	execCommand("helm repo add", helmChart, helmRepo)

	printDebug("Updating helm repository\n")
	execCommand("helm repo update")

	printDebug("Pulling helm chart\n")
	execCommand("helm pull --untar --untardir "+tmpDir+helmChartVersion, helmChart+"/"+helmChart)

	printDebug("Templating helm chart\n")
	execCommand("helm template"+customValues, "--namespace", namespace, helmChart, tmpDir+"/"+helmChart, "--output-dir", tmpDir+"/rendered")

	// Rename all rendered yamls
	renderedDir := tmpDir + "/rendered/" + helmChart + "/templates"
	dir, err := os.Open(renderedDir)
	checkErr(err)
	dirInfo, err := dir.ReadDir(-1)
	dir.Close()
	checkErr(err)

	splitAndRename(renderedDir, ".", dirInfo)

	if !debug {
		os.RemoveAll(tmpDir)
	}
}

func splitAndRename(renderedDir, subchartDir string, dirInfo []fs.DirEntry) {
	var obj ManifestStruct

	for _, file := range dirInfo {
		inputFile := renderedDir + "/" + file.Name()

		printDebug("Checking file %v\n", inputFile)

		if file.IsDir() {
			printDebug("It is a directory\n")
			dir, err := os.Open(inputFile)
			checkErr(err)
			subDirInfo, err := dir.ReadDir(-1)
			dir.Close()
			checkErr(err)

			splitAndRename(inputFile, subchartDir+"/"+file.Name(), subDirInfo)
			continue
		}

		yamlFile, err := os.ReadFile(inputFile)
		checkErr(err)

		yamlSlice := strings.Split(string(yamlFile), "---")

		for _, manifest := range yamlSlice[1:] {
			manifestByte := []byte("---" + manifest)

			err = yaml.Unmarshal(manifestByte, &obj)
			checkErr(err)

			shortcut := shortcutMap[obj.Kind]
			if shortcut == "" {
				panic("Unknown kind " + obj.Kind)
			}

			manifestName := obj.Metadata.Name

			if fileIsAbsent(subchartDir) {
				printDebug("Creating directory " + subchartDir + "\n")
				os.MkdirAll(subchartDir, 0755)
			}

			outputFilename := fmt.Sprintf("%v/%v-%v.yaml", subchartDir, shortcut, manifestName)
			fmt.Println("Generating", outputFilename)

			if !fileIsAbsent(outputFilename) {
				if overwrite {
					printDebug("WARNING! File %v is present. Continue anyway, because --overwrite was provided\n", outputFilename)
				} else {
					fmt.Printf("ERROR! File %v is present. Use --overwrite if you want to skip this error. Exiting...\n", outputFilename)
					os.Exit(1)
				}
			}

			err = os.WriteFile(outputFilename, manifestByte, 0644)
			checkErr(err)
		}
	}
}

func fileIsAbsent(filename string) bool {
	_, err := os.Stat(filename)
	return os.IsNotExist(err)
}

func execCommand(command ...string) {
	joinedCommand := strings.Join(command, " ")
	args := strings.Split(joinedCommand, " ")

	printDebug("Running command: %v\n", args)

	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.CombinedOutput()

	printDebug(string(output))
	if err != nil {
		if !debug {
			fmt.Println(string(output))
		}
		fmt.Println(err)
		os.Exit(1)
	}
}

func printDebug(message ...interface{}) {
	if debug {
		fmt.Printf(message[0].(string), message[1:]...)
	}
}

func checkErr(err error) {
	if err != nil {
		fmt.Println(err)
	}
}
