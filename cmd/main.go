package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"strings"

	"gopkg.in/yaml.v3"
)

const tmpDir = "helm_splitter_tmp"
const etcConfigPath = "/etc/helm-splitter/config.yaml"
const homeConfigName = ".helm-splitter.yaml"

var overwrite, debug bool

type configStruct struct {
	FilePath  string            `yaml:"filepath,omitempty"`
	Shortcuts map[string]string `yaml:"shortcuts"`
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
	var namespace, helmRepo, helmChart, helmChartVersion, customValues, outputDir, includeCRDsFlag, customConfigFile string
	var skipCRDs bool

	flag.StringVar(&namespace, "namespace", "", "target k8s namespace")
	flag.StringVar(&helmRepo, "repository", "", "helm repository")
	flag.StringVar(&helmChart, "chart", "", "helm chart name")
	flag.StringVar(&helmChartVersion, "version", "", "helm chart version, default: <latest>")
	flag.StringVar(&customValues, "custom-values-file", "", "file with custom values")
	flag.StringVar(&outputDir, "output-dir", "", "output directory")
	flag.BoolVar(&skipCRDs, "skip-crds", false, "do not generate CRDs, default: false")
	flag.BoolVar(&overwrite, "overwrite", false, "overwrite existing output files, default: false")
	flag.StringVar(&customConfigFile, "config", "", "path to config file")
	flag.BoolVar(&debug, "debug", false, "debug")

	flag.Parse()

	printDebug("Input values:\nNamespace: %v\nRepository: %v\nChart: %v\nVersion: %v\nCustom Values: %v\nSkip CRDs: %t\nOverwrte: %t\nConfig: %v\nDebug: %t\n", namespace, helmRepo, helmChart, helmChartVersion, customValues, skipCRDs, overwrite, customConfigFile, debug)

	config := parseConfig(customConfigFile)

	if namespace == "" || helmChart == "" || helmRepo == "" {
		fmt.Println("ERROR! Missing parameters. \"--namespace\", \"--repository\" and \"--chart\" MUST be specified!")
		exit(1)
	}

	if helmChartVersion != "" {
		helmChartVersion = " --version " + helmChartVersion
	}

	if customValues != "" {
		customValues = " --values " + customValues
	}

	if outputDir == "" {
		outputDir = helmChart
	}

	if skipCRDs {
		includeCRDsFlag = ""
	} else {
		includeCRDsFlag = " --include-crds"
	}

	// Run helm commands
	printDebug("Adding helm repository\n")
	execCommand("helm repo add", helmChart, helmRepo)

	printDebug("Updating helm repository\n")
	execCommand("helm repo update")

	printDebug("Pulling helm chart\n")
	execCommand("helm pull --untar --untardir "+tmpDir+helmChartVersion, helmChart+"/"+helmChart)

	printDebug("Templating helm chart\n")
	execCommand("helm template"+customValues+includeCRDsFlag, "--namespace", namespace, helmChart, tmpDir+"/"+helmChart, "--output-dir", tmpDir+"/rendered")

	// Rename all rendered yamls
	processRenderedDir(tmpDir+"/rendered/"+helmChart+"/templates", &config, outputDir)
	processRenderedDir(tmpDir+"/rendered/"+helmChart+"/crds", &config, outputDir)

	exit(0)
}

func parseConfig(customConfigFilePath string) configStruct {
	var config configStruct
	var configFilePath string

	// Default shortcuts are used only if there is no config file
	defaultShortcuts := map[string]string{
		"Alertmanager":                   "am",
		"APIService":                     "asvc",
		"ClusterRole":                    "crol",
		"ClusterRoleBinding":             "crb",
		"ConfigMap":                      "cm",
		"CronJob":                        "cj",
		"CustomResourceDefinition":       "crd",
		"DaemonSet":                      "ds",
		"Deployment":                     "dep",
		"HorizontalPodAutoscaler":        "hpa",
		"Ingress":                        "ing",
		"Job":                            "job",
		"MutatingWebhookConfiguration":   "mwc",
		"Namespace":                      "ns",
		"NetworkPolicy":                  "np",
		"PersistentVolumeClaim":          "pvc",
		"PodDisruptionBudget":            "pdb",
		"PriorityClass":                  "pc",
		"Prometheus":                     "prom",
		"PrometheusRule":                 "prul",
		"Role":                           "rol",
		"RoleBinding":                    "rb",
		"Secret":                         "sec",
		"Service":                        "svc",
		"ServiceAccount":                 "sa",
		"ServiceMonitor":                 "sm",
		"StatefulSet":                    "ss",
		"StorageClass":                   "sc",
		"ValidatingWebhookConfiguration": "vwc",
	}

	usr, err := user.Current()
	checkErr(err)
	homeConfigPath := usr.HomeDir + "/" + homeConfigName

	// Find config location
	printDebug("Checking configs...\n")
	if customConfigFilePath != "" {
		printDebug("--config was specified, using %v\n", customConfigFilePath)
		configFilePath = customConfigFilePath
	} else if !fileIsAbsent(homeConfigPath) {
		printDebug("Found %v, using it\n", homeConfigPath)
		configFilePath = homeConfigPath
	} else if !fileIsAbsent(etcConfigPath) {
		printDebug("Found %v, using it\n", etcConfigPath)
		configFilePath = etcConfigPath
	} else {
		printDebug("No config was found, creating a default one in %v\n", homeConfigPath)
		config.Shortcuts = defaultShortcuts

		configYamlData, err := yaml.Marshal(&config)
		checkErr(err)
		err = os.WriteFile(homeConfigPath, configYamlData, 0644)
		checkErr(err)

		config.FilePath = homeConfigPath
		return config
	}

	configByte, err := os.ReadFile(configFilePath)
	checkErr(err)

	err = yaml.Unmarshal(configByte, &config)
	checkErr(err)

	config.FilePath = configFilePath

	return config
}

func processRenderedDir(renderedDir string, config *configStruct, outputDir string) {
	printDebug("Processing directory %v\n", renderedDir)
	if fileIsAbsent(renderedDir) {
		printDebug("Directory not found\n")
		return
	}

	dir, err := os.Open(renderedDir)
	checkErr(err)
	dirInfo, err := dir.ReadDir(-1)
	dir.Close()
	checkErr(err)

	splitAndRename(renderedDir, outputDir, dirInfo, config)
}

func splitAndRename(renderedDir, subchartDir string, dirInfo []fs.DirEntry, config *configStruct) {
	var obj ManifestStruct

	// Iterate over all rendered files
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

			splitAndRename(inputFile, subchartDir+"/"+file.Name(), subDirInfo, config)
			continue
		}

		yamlFile, err := os.ReadFile(inputFile)
		checkErr(err)

		// Split yamls containing multiple manifests
		yamlSlice := strings.Split(string(yamlFile), "---")

		for _, manifest := range yamlSlice[1:] {
			manifestByte := []byte("---" + manifest)

			err = yaml.Unmarshal(manifestByte, &obj)
			checkErr(err)

			if obj.Kind == "" {
				printDebug("WARNING! Empty Kind, skipping manifest:\n%v", string(manifestByte))
				continue
			}

			shortcut := config.Shortcuts[obj.Kind]
			if shortcut == "" {
				fmt.Printf("ERROR! Unknown kind \"%v\"! Add a shortcut for this kind to %v and rerun!\n", obj.Kind, config.FilePath)
				printDebug("Caused by this manifest:\n%v", string(manifestByte))
				exit(1)
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
					exit(1)
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
		exit(1)
	}
}

func exit(exitCode int) {
	if !debug {
		os.RemoveAll(tmpDir)
	}
	os.Exit(exitCode)
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
