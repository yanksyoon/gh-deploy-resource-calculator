package main

import (
	"fmt"
	"strconv"
	"strings"

	hcl "github.com/joselitofilho/hcl-parser-go/pkg/parser/hcl"
	"gopkg.in/yaml.v3"
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

	cloudToResource := parseClouds(config)
	local_var_to_cloud_map := parseLocalVarToClouds(config, &cloudToResource)

	// Print resources
	for _, resource := range config.Resources {
		if resource.Type != "juju_application" {
			fmt.Println("Skipping resource type: ", resource.Type, resource.Name)
			continue
		}
		if strings.Contains(resource.Name, "image-builder") {
			deploy_cloud, deploy_resource, vm_cloud, vm_resource := parseImageBuilder(resource, &local_var_to_cloud_map)
			fmt.Println(deploy_cloud, deploy_resource, vm_cloud, vm_resource)
			addResourceToClouds(&cloudToResource, deploy_cloud, &deploy_resource)
			addResourceToClouds(&cloudToResource, vm_cloud, &vm_resource)
		} else if strings.Contains(resource.Name, "github-runner") || strings.Contains(resource.Name, "runner") {
			deploy_cloud, deploy_resource, vm_cloud, vm_resource := parseRunner(resource, &local_var_to_cloud_map)
			fmt.Println(deploy_cloud, deploy_resource, vm_cloud, vm_resource)
			addResourceToClouds(&cloudToResource, deploy_cloud, &deploy_resource)
			addResourceToClouds(&cloudToResource, vm_cloud, &vm_resource)
		} else {
			fmt.Println("Unable to detect resource type from resource name, ", resource.Name)
		}
	}

	for cloud_name, resource := range cloudToResource {
		fmt.Println("Cloud: ", cloud_name)
		fmt.Println("CPU: ", resource.CPU)
		fmt.Println("RAM: ", resource.MEM)
		fmt.Println("DISK: ", resource.DISK)
		fmt.Println()
	}
}

func parseClouds(config *hcl.Config) map[CloudName]*Resource {
	clouds := map[CloudName]*Resource{
		"DEFAULT": {},
	}
	for _, variable := range config.Locals {
		for key, val := range variable.Attributes {
			if strings.Contains(key, "user_name") {
				clouds[CloudName(val.(string))] = &Resource{}
			}
		}
	}
	return clouds
}

func parseLocalVarToClouds(config *hcl.Config, clouds *map[CloudName]*Resource) map[LocalCloudVarName]CloudName {
	local_var_to_cloud_map := map[LocalCloudVarName]CloudName{}
	localUserNameVarToCloudNameMap := getLocalUserNameVarToCloudNameMap(config, clouds)
	for _, variable := range config.Locals {
		for localVarName, localVarValue := range variable.Attributes {
			if !strings.Contains(localVarName, "clouds_yaml") && !strings.Contains(localVarName, "user_name") {
				continue
			}
			if strings.Contains(localVarName, "clouds_yaml") {
				clouds_yaml_contents := localVarValue.(string)
				username := getCloudsYamlUsername(clouds_yaml_contents)
				if cloudname, ok := localUserNameVarToCloudNameMap[username]; ok {
					local_var_to_cloud_map[LocalCloudVarName(localVarName)] = cloudname
				} else {
					local_var_to_cloud_map[LocalCloudVarName(localVarName)] = CloudName(username)
				}
			}
		}
	}
	for usernameVarName, cloudname := range localUserNameVarToCloudNameMap {
		local_var_to_cloud_map[LocalCloudVarName(usernameVarName)] = cloudname
	}
	return local_var_to_cloud_map
}

// either local.variable_name or the username itself.
func getCloudsYamlUsername(cloudsYamlContents string) string {
	contentsMap := map[string]any{}
	if err := yaml.Unmarshal([]byte(cloudsYamlContents), &contentsMap); err != nil {
		panic("INVALID YAML FOUND")
	}
	clouds := contentsMap["clouds"].(map[string]any)
	for _, authContents := range clouds {
		authContentsMap := authContents.(map[string]any)["auth"].(map[string]any)
		return strings.ReplaceAll(authContentsMap["username"].(string), "local.", "")
	}
	fmt.Println("Username in clouds yaml not detected")
	return ""
}

func getLocalUserNameVarToCloudNameMap(config *hcl.Config, clouds *map[CloudName]*Resource) map[string]CloudName {
	localUserNameVarToCloudNameMap := map[string]CloudName{}
	for _, variable := range config.Locals {
		for key, val := range variable.Attributes {
			if !strings.Contains(key, "user_name") {
				continue
			}
			username_contents := val.(string)
			for cloud_name := range *clouds {
				if strings.Contains(username_contents, string(cloud_name)) {
					localUserNameVarToCloudNameMap[key] = CloudName(cloud_name)
				}
			}
		}
	}
	return localUserNameVarToCloudNameMap
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
	fmt.Println("PARSING RUNNER")
	constraints := resource.Attributes["constraints"].(string)
	deploy_resource := parseConstraints(constraints)
	config := resource.Attributes["config"].(map[string]any)
	if _, ok := config["openstack-clouds-yaml"]; !ok {
		fmt.Println("Runner application openstack-clouds-yaml not defined.")
		return "", Resource{}, "", Resource{}
	}
	clouds_yaml_var := strings.ReplaceAll(config["openstack-clouds-yaml"].(string), "local.", "")
	cloud_name := (*varNameToCloudName)[LocalCloudVarName(clouds_yaml_var)]
	vmResource := parseFlavor(config["openstack-flavor"].(string))
	numVms, err := strconv.Atoi(config["virtual-machines"].(string))
	if err != nil {
		numVms = 0
		fmt.Println("Invalid virtual-machines config detected")
	}
	vmResource.CPU *= numVms
	vmResource.MEM *= numVms
	vmResource.DISK *= numVms
	return CloudName("DEFAULT"), deploy_resource, cloud_name, vmResource
}

// Return resource for charm deployment and VM deployment
func parseImageBuilder(resource *hcl.Resource, varNameToCloudName *map[LocalCloudVarName]CloudName) (CloudName, Resource, CloudName, Resource) {
	if resource.Type != "juju_application" {
		return "", Resource{}, "", Resource{}
	}
	if !strings.Contains(resource.Name, "image-builder") {
		return "", Resource{}, "", Resource{}
	}
	constraints := resource.Attributes["constraints"].(string)
	deploy_resource := parseConstraints(constraints)
	config := resource.Attributes["config"].(map[string]any)
	if _, ok := config["experimental-external-build-flavor"]; !ok {
		fmt.Println("experimental-external-build-flavor not defined.")
		return "", Resource{}, "", Resource{}
	}
	if _, ok := config["openstack-user-name"]; !ok {
		fmt.Println("openstack-user-name not defined.")
		return "", Resource{}, "", Resource{}
	}
	varname := strings.ReplaceAll(config["openstack-user-name"].(string), "local.", "")
	cloud_name := (*varNameToCloudName)[LocalCloudVarName(varname)]
	vmResource := parseFlavor(config["experimental-external-build-flavor"].(string))
	return CloudName("DEFAULT"), deploy_resource, cloud_name, vmResource
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

func parseFlavor(flavor_str string) Resource {
	resource := Resource{}
	defs := strings.Split(flavor_str, "-")
	for _, def := range defs {
		if strings.Contains(def, "cpu") {
			coresStr := strings.ReplaceAll(def, "cpu", "")
			cores, err := strconv.Atoi(coresStr)
			if err != nil {
				fmt.Println("Invalid flavor detected, setting cores to 0.")
				cores = 0
			}
			resource.CPU = cores
		}
		if strings.Contains(def, "ram") {
			ramStr := strings.ReplaceAll(def, "ram", "")
			ram, err := strconv.Atoi(ramStr)
			if err != nil {
				fmt.Println("Invalid flavor detected, setting ram to 0.")
				ram = 0
			}
			resource.MEM = ram
		}
		if strings.Contains(def, "disk") {
			diskStr := strings.ReplaceAll(def, "disk", "")
			disk, err := strconv.Atoi(diskStr)
			if err != nil {
				fmt.Println("Invalid flavor detected, setting disk to 0.")
				disk = 0
			}
			resource.DISK = disk
		}
	}
	return resource
}

func addResourceToClouds(cloudToResource *map[CloudName]*Resource, cloudName CloudName, resource *Resource) {
	if _, ok := (*cloudToResource)[cloudName]; !ok {
		(*cloudToResource)[cloudName] = &Resource{}
	}
	targetResource := (*cloudToResource)[cloudName]
	targetResource.CPU += resource.CPU
	targetResource.MEM += resource.MEM
	targetResource.DISK += resource.DISK
}
