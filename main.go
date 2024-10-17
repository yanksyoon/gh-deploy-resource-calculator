package main

import (
	"fmt"
	"strconv"
	"strings"

	hcl "github.com/joselitofilho/hcl-parser-go/pkg/parser/hcl"
)

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

	// Local variables - use to parse clouds
	clouds := map[string]*Resource{
		"DEFAULT": &Resource{},
	}
	for _, variable := range config.Locals {
		fmt.Println("Var: ", variable)
		for key := range variable.Attributes {
			if strings.Contains(key, "project_name") {
				clouds[key] = &Resource{}
			}
		}
	}
	local_var_to_cloud_map := map[string]string{}
	for _, variable := range config.Locals {
		for key, val := range variable.Attributes {
			if !strings.Contains(key, "clouds_yaml") {
				continue
			}
			if clouds_yaml_contents, ok := val.(string); !ok {
				continue
			} else {
				for cloud_name := range clouds {
					if strings.Contains(clouds_yaml_contents, cloud_name) {
						local_var_to_cloud_map[key] = cloud_name
					}
				}
			}
		}
	}

	fmt.Println(clouds, local_var_to_cloud_map)

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
			resrc := parse_constraints(constraint_str)
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

func parse_constraints(constraint_str string) Resource {
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
