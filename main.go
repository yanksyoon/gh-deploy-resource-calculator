package main

import (
	"fmt"
	"strconv"
	"strings"

	hcl "github.com/joselitofilho/hcl-parser-go/pkg/parser/hcl"
	"gopkg.in/yaml.v3"
)

type Resource struct {
	// CPU in cores
	CPU int
	// MEM in M
	MEM int
	// DISK in M
	DISK int
}

const JujuApplicationResourceType string = "juju_application"
const GitHubRunnerCharmName string = "github-runner"
const ImageBuilderCharmName string = "github-runner-image-builder"
const UndefinedModelName string = "UNDEFINED"

var DefaultGitHubRunnerResource Resource = Resource{
	CPU:  2,
	MEM:  8192,
	DISK: 29000,
}

var DefaultImageBuilderResource Resource = Resource{
	CPU:  2,
	MEM:  16384,
	DISK: 51200,
}
var DefaultImageBuilderFlavor Resource = Resource{
	CPU:  2,
	MEM:  16384,
	DISK: 20480,
}

const DefaultNumVirtualMachines int = 1

func main() {
	files := []string{"prod-main.tf"}
	directories := []string{}

	// Parse Terraform configurations
	config, err := hcl.Parse(directories, files)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	localsMap := map[string]string{}
	for _, local := range config.Locals {
		for key, val := range local.Attributes {
			localsMap[key] = val.(string)
		}
	}

	cloudToResourceMap := map[string]*Resource{
		UndefinedModelName: {},
	}

	for _, resource := range config.Resources {
		if resource.Type != JujuApplicationResourceType {
			continue
		}
		charmAttributes := resource.Attributes["charm"].(map[string]any)
		charm := charmAttributes["name"].(string)
		charm = replaceLocalVar(&localsMap, charm)
		if charm == GitHubRunnerCharmName {
			parseGitHubRunnerCharm(resource, &localsMap, &cloudToResourceMap)
		} else if charm == ImageBuilderCharmName {
			parseImageBuilderCharm(resource, &localsMap, &cloudToResourceMap)
		} else {
			fmt.Println("[WARNING] Skipping charm type: ", charm)
		}
	}

	for cloudName, resource := range cloudToResourceMap {
		fmt.Printf("%s: cpu: %d mem: %dM, disk: %dM\n", cloudName, resource.CPU, resource.MEM, resource.DISK)
	}
}

func replaceLocalVar(localsMap *map[string]string, varOrValue string) string {
	if strings.Contains(varOrValue, "local") {
		varOrValue = strings.ReplaceAll(varOrValue, "local.", "")
	} else {
		return varOrValue
	}
	if localVar, ok := (*localsMap)[varOrValue]; ok {
		return localVar
	}
	fmt.Println("[WARNING] Local variable not fouund: ", varOrValue)
	return varOrValue
}

func parseGitHubRunnerCharm(resource *hcl.Resource, localsMap *map[string]string, cloudResourceMap *map[string]*Resource) {
	// Manager model deployment resource calculation
	var localModelResource *Resource
	if constraint, ok := resource.Attributes["constraints"].(string); ok {
		constraint = replaceLocalVar(localsMap, constraint)
		resource := parseConstraints(constraint)
		localModelResource = &resource
	} else {
		fmt.Printf("[WARNING] Constraint for GitHub runner %s not defined.\n", resource.Name)
		localModelResource = &Resource{
			CPU:  DefaultGitHubRunnerResource.CPU,
			MEM:  DefaultGitHubRunnerResource.MEM,
			DISK: DefaultGitHubRunnerResource.DISK,
		}
	}
	if modelName, ok := resource.Attributes["model"].(string); ok {
		modelName = replaceLocalVar(localsMap, modelName)
		if resource, ok := (*cloudResourceMap)[modelName]; ok {
			resource.CPU += localModelResource.CPU
			resource.MEM += localModelResource.MEM
			resource.DISK += localModelResource.DISK
		} else {
			(*cloudResourceMap)[modelName] = localModelResource
		}
	} else {
		fmt.Printf("[WARNING] Default model name for GitHub runner %s not defined.\n", resource.Name)
		(*cloudResourceMap)[UndefinedModelName].CPU += localModelResource.CPU
		(*cloudResourceMap)[UndefinedModelName].MEM += localModelResource.MEM
		(*cloudResourceMap)[UndefinedModelName].DISK += localModelResource.DISK
	}

	// VM model deployment resource calculation
	configs := resource.Attributes["config"].(map[string]any)
	var numVirtualMachines = 1
	vms, ok := configs["virtual-machines"]
	if !ok {
		fmt.Printf("[WARNING] Virtual machines not set for %s, using 1 (default).\n", resource.Name)
	} else {
		vmsStr := vms.(string)
		numVms, err := strconv.Atoi(vmsStr)
		if err != nil {
			fmt.Printf("[WARNING] Invalid config virtual-machines for %s, using 1 (default).\n", resource.Name)
			numVms = 1
		}
		numVirtualMachines = numVms
	}

	cloudsYaml := configs["openstack-clouds-yaml"].(string)
	cloudsYaml = replaceLocalVar(localsMap, cloudsYaml)
	cloudName := parseOpenStackCloudsYaml(cloudsYaml)
	cloudName = replaceLocalVar(localsMap, cloudName)
	flavor := configs["openstack-flavor"].(string)
	flavor = replaceLocalVar(localsMap, flavor)
	cloudResource := parseOpenStackFlavor(flavor)
	cloudResource.CPU *= numVirtualMachines
	cloudResource.MEM *= numVirtualMachines
	cloudResource.DISK *= numVirtualMachines
	if resource, ok := (*cloudResourceMap)[cloudName]; ok {
		resource.CPU += cloudResource.CPU
		resource.MEM += cloudResource.MEM
		resource.DISK += cloudResource.DISK
	} else {
		(*cloudResourceMap)[cloudName] = &cloudResource
	}
}

func parseImageBuilderCharm(resource *hcl.Resource, localsMap *map[string]string, cloudResourceMap *map[string]*Resource) {
	var localModelResource *Resource
	if constraint, ok := resource.Attributes["constraints"].(string); ok {
		constraint = replaceLocalVar(localsMap, constraint)
		resource := parseConstraints(constraint)
		localModelResource = &resource
	} else {
		fmt.Printf("[WARNING] Constraint for Image builder %s not defined.\n", resource.Name)
		localModelResource = &Resource{
			CPU:  DefaultImageBuilderResource.CPU,
			MEM:  DefaultImageBuilderResource.MEM,
			DISK: DefaultImageBuilderResource.DISK,
		}
	}
	if modelName, ok := resource.Attributes["model"].(string); ok {
		modelName = replaceLocalVar(localsMap, modelName)
		if resource, ok := (*cloudResourceMap)[modelName]; ok {
			resource.CPU += localModelResource.CPU
			resource.MEM += localModelResource.MEM
			resource.DISK += localModelResource.DISK
		} else {
			(*cloudResourceMap)[modelName] = localModelResource
		}
	} else {
		fmt.Printf("[WARNING] Default model name for Image builder %s not defined.\n", resource.Name)
		(*cloudResourceMap)[UndefinedModelName].CPU += localModelResource.CPU
		(*cloudResourceMap)[UndefinedModelName].MEM += localModelResource.MEM
		(*cloudResourceMap)[UndefinedModelName].DISK += localModelResource.DISK
	}

	configs := resource.Attributes["config"].(map[string]any)
	modelName, ok := configs["openstack-user-name"]
	if !ok {
		fmt.Printf("[WARNING] openstack-user-name not defined for Image builder %s, likely using local builder.\n", resource.Name)
		return
	}
	modelNameVal := modelName.(string)
	modelNameVal = replaceLocalVar(localsMap, modelNameVal)
	flavorStr, ok := configs["experimental-external-build-flavor"]
	var flavorResource *Resource
	if !ok {
		fmt.Printf("[Warning] Image builder flavor not defined for %s, using default.\n", resource.Name)
		flavorResource = &Resource{
			CPU:  DefaultImageBuilderFlavor.CPU,
			MEM:  DefaultImageBuilderFlavor.MEM,
			DISK: DefaultImageBuilderFlavor.DISK,
		}
	} else {
		flavor := replaceLocalVar(localsMap, flavorStr.(string))
		openstackFlavor := parseOpenStackFlavor(flavor)
		flavorResource = &openstackFlavor
	}

	if resource, ok := (*cloudResourceMap)[modelNameVal]; ok {
		resource.CPU += flavorResource.CPU
		resource.MEM += flavorResource.MEM
		resource.DISK += flavorResource.DISK
	} else {
		(*cloudResourceMap)[modelNameVal] = flavorResource
	}
}

const JujuConstraintCores string = "cores"
const JujuConstraintMem string = "mem"
const JujuConstraintDisk string = "root-disk"

func parseConstraints(constraintStr string) Resource {
	cpu := 0
	mem := 0
	disk := 0
	for _, constraint := range strings.Split(constraintStr, " ") {
		resourceDef := strings.Split(constraint, "=")
		resourceType := strings.TrimSpace((resourceDef[0]))
		multiplier := 1
		resourceVal := strings.TrimSpace(resourceDef[1])
		if strings.Contains(resourceVal, "M") {
			resourceVal = strings.ReplaceAll(resourceVal, "M", "")
			multiplier = 1
		} else if strings.Contains(resourceVal, "G") {
			resourceVal = strings.ReplaceAll(resourceVal, "G", "")
			multiplier = 1000
		}
		val, err := strconv.Atoi(resourceVal)
		if err != nil && resourceType != "arch" {
			fmt.Printf("[Warning] Invalid constraint resource value %s\n", constraintStr)
			val = 0
		}

		if resourceType == JujuConstraintCores {
			cpu = val
		} else if resourceType == JujuConstraintMem {
			mem = val * multiplier
		} else if resourceType == JujuConstraintDisk {
			disk = val * multiplier
		}
	}
	return Resource{
		CPU:  cpu,
		MEM:  mem,
		DISK: disk,
	}
}

const OpenstackFlavorCores string = "cpu"
const OpenstackFlavorMem string = "ram"
const OpenstackFlavorDisk string = "disk"

func parseOpenStackFlavor(flavorStr string) Resource {
	cpu := 0
	mem := 0
	disk := 0
	multiplier := 1000
	for _, constraint := range strings.Split(flavorStr, "-") {
		if strings.Contains(constraint, OpenstackFlavorCores) {
			cpuStr := strings.TrimLeft(constraint, OpenstackFlavorCores)
			val, err := strconv.Atoi(cpuStr)
			if err != nil {
				fmt.Printf("[Warning] Invalid constraint CPU resource value %s\n", cpuStr)
			}
			cpu = val
		} else if strings.Contains(constraint, OpenstackFlavorMem) {
			memStr := strings.TrimLeft(constraint, OpenstackFlavorMem)
			val, err := strconv.Atoi(memStr)
			if err != nil {
				fmt.Printf("[Warning] Invalid constraint MEM resource value %s\n", memStr)
			}
			mem = val * multiplier
		} else if strings.Contains(constraint, OpenstackFlavorDisk) {
			diskStr := strings.TrimLeft(constraint, OpenstackFlavorDisk)
			val, err := strconv.Atoi(diskStr)
			if err != nil {
				fmt.Printf("[Warning] Invalid constraint DISK resource value %s\n", diskStr)
			}
			disk = val * multiplier
		}
	}
	if cpu == 0 {
		fmt.Printf("[Warning] CPU 0 detectd in %s\n", flavorStr)
	}
	if mem == 0 {
		fmt.Printf("[Warning] mem 0 detectd in %s\n", flavorStr)
	}
	if disk == 0 {
		fmt.Printf("[Warning] disk 0 detectd in %s\n", flavorStr)
	}

	return Resource{
		CPU:  cpu,
		MEM:  mem,
		DISK: disk,
	}
}

// Get Cloud name
func parseOpenStackCloudsYaml(yamlStr string) string {
	yamlMap := map[string]any{}
	if err := yaml.Unmarshal([]byte(yamlStr), yamlMap); err != nil {
		fmt.Printf("[WARNING] Invalid Openstack clouds YAML %s \n", yamlStr)
		return UndefinedModelName
	}
	clouds := yamlMap["clouds"].(map[string]any)
	for _, cloudMapAny := range clouds {
		cloudMap := cloudMapAny.(map[string]any)
		authMap := cloudMap["auth"].(map[string]any)
		return authMap["username"].(string)
	}
	fmt.Printf("Cloud name not found in %s\n", yamlStr)
	return UndefinedModelName
}
