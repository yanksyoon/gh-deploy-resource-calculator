package main

import (
	"fmt"
	"strconv"
	"strings"

	hcl "github.com/joselitofilho/hcl-parser-go/pkg/parser/hcl"
)

type CloudName string
type LocalCloudVarName string

type Resource struct {
	CPU  int
	MEM  int
	DISK int
}

func main() {
	directories := []string{}
	files := []string{"main.tf"}

	// Parse Terraform configurations
	config, err := hcl.Parse(directories, files)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	clouds := parseClouds(config)
	local_var_to_cloud_map := parseLocalVarToClouds(config, &clouds)

	fmt.Println("All detected clouds: ", clouds)
	fmt.Println("All dtected local var mapping to clouds: ", local_var_to_cloud_map)

	// Print resources
	fmt.Println("Resources:")
	for _, resource := range config.Resources {
		if resource.Type != "juju_application" {
			continue
		}
		// parse constraints
		if app_constraints, ok := resource.Attributes["constraints"]; !ok {
			fmt.Println("Application constraint not defined.")
		} else {
			constraint_str := app_constraints.(string)
			resrc := parseConstraints(constraint_str)
			clouds["DEFAULT"].CPU += resrc.CPU
			clouds["DEFAULT"].MEM += resrc.MEM
			clouds["DEFAULT"].DISK += resrc.DISK
		}
		// parse config
		if app_config, ok := resource.Attributes["config"]; !ok {
			fmt.Println("No config for juju application found.")
			continue
		} else {
			if config_map, ok := app_config.(map[string]any); !ok {
				fmt.Println("Invalid config map type.")
				continue
			} else {
				config_map
			}
		}
	}

	// Print locals
	fmt.Println("\nLocals:")
	for _, local := range config.Locals {
		for key, value := range local.Attributes {
			fmt.Printf("    %s: %v\n", key, value)
		}
	}
}

func parseClouds(config *hcl.Config) map[CloudName]*Resource {
	clouds := map[CloudName]*Resource{
		"DEFAULT": &Resource{},
	}
	for _, variable := range config.Locals {
		for key := range variable.Attributes {
			if strings.Contains(key, "project_name") {
				clouds[CloudName(key)] = &Resource{}
			}
		}
	}
	return clouds
}

func parseLocalVarToClouds(config *hcl.Config, clouds *map[CloudName]*Resource) map[LocalCloudVarName]CloudName {
	local_var_to_cloud_map := map[LocalCloudVarName]CloudName{}
	for _, variable := range config.Locals {
		for key, val := range variable.Attributes {
			if !strings.Contains(key, "clouds_yaml") {
				continue
			}
			clouds_yaml_contents := val.(string)
			for cloud_name := range *clouds {
				if strings.Contains(clouds_yaml_contents, string(cloud_name)) {
					local_var_to_cloud_map[LocalCloudVarName(key)] = CloudName(cloud_name)
				}
			}
		}
	}
	return local_var_to_cloud_map
}

// Return resource for charm deployment and VM deployment
func parseRunner(resource *hcl.Resource, varNameToCloudName *map[LocalCloudVarName]CloudName) (CloudName, Resource, CloudName, Resource) {
	if resource.Type != "juju_application" {
		return "", Resource{}, "", Resource{}
	}
	if strings.Contains(resource.Name, "image-builder") {
		return "", Resource{}, "", Resource{}
	}
	if !strings.Contains(resource.Name, "github-runner") {
		fmt.Println("Invalid resource name detected, assuming runner.")
	}
	constraints := resource.Attributes["constraints"].(string)
	deploy_resource := parseConstraints(constraints)
	config := resource.Attributes["config"].(map[string]string)
	if _, ok := config["openstack-clouds-yaml"]; !ok {
		fmt.Println("Runner application openstack-clouds-yaml not defined.")
		return "", Resource{}, "", Resource{}
	}
	clouds_yaml_var := config["openstack-clouds-yaml"]
	cloud_name := (*varNameToCloudName)[LocalCloudVarName(clouds_yaml_var)]
	return CloudName("DEFAULT"), deploy_resource, cloud_name, Resource{}
}

// Return resource for charm deployment and VM deployment
func parseImageBuilder(resource *hcl.Resource) (CloudName, Resource, CloudName, Resource) {
	if resource.Type != "juju_application" {
		return "", Resource{}, "", Resource{}
	}
	return "", Resource{}, "", Resource{}
}

func parseConstraints(constraint_str string) Resource {
	resource := Resource{}
	constraints := strings.Split(constraint_str, " ")
	for _, constraint := range constraints {
		keyval := strings.Split(constraint, "=")
		if keyval[0] == "cores" {
			if v, err := strconv.Atoi(keyval[1]); err != nil {
				fmt.Println("Invalid CPU value")
				continue
			} else {
				resource.CPU += v
			}
		} else if keyval[0] == "mem" {
			if v, err := strconv.Atoi(strings.Split(keyval[1], "M")[0]); err != nil {
				fmt.Println("Invalid MEM value")
				continue
			} else {
				resource.MEM += v
			}
		} else if keyval[0] == "root-disk" {
			if v, err := strconv.Atoi(strings.Split(keyval[1], "M")[0]); err != nil {
				fmt.Println("Invalid DISK value")
				continue
			} else {
				resource.DISK += v
			}
		}
	}
	return resource
}
