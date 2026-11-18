package parsers

import (
	"fmt"
	"strings"

	"github.com/pipescope/backend/internal/graph"
	"gopkg.in/yaml.v3"
)

// AnsibleParser statically parses Ansible Playbook YAML files (playbook.yml, site.yml).
type AnsibleParser struct{}

// NewAnsibleParser returns a new AnsibleParser instance.
func NewAnsibleParser() *AnsibleParser {
	return &AnsibleParser{}
}

type ansiblePlay struct {
	Name      string                   `yaml:"name"`
	Hosts     yaml.Node                `yaml:"hosts"`
	Roles     []yaml.Node              `yaml:"roles"`
	PreTasks  []map[string]interface{} `yaml:"pre_tasks"`
	Tasks     []map[string]interface{} `yaml:"tasks"`
	PostTasks []map[string]interface{} `yaml:"post_tasks"`
}

var nonModuleTaskKeys = map[string]struct{}{
	"name":          {},
	"become":        {},
	"become_user":   {},
	"when":          {},
	"register":      {},
	"tags":          {},
	"notify":        {},
	"vars":          {},
	"environment":   {},
	"loop":          {},
	"with_items":    {},
	"ignore_errors": {},
	"changed_when":  {},
	"failed_when":   {},
	"delegate_to":   {},
	"run_once":      {},
	"block":         {},
	"rescue":        {},
	"always":        {},
}

// Parse implements the Parser interface for Ansible playbook YAML files.
func (p *AnsibleParser) Parse(filePath string, content []byte) (*graph.PipelineGraph, error) {
	var plays []ansiblePlay
	if err := yaml.Unmarshal(content, &plays); err != nil {
		return nil, fmt.Errorf("failed to parse Ansible playbook %s: %w", filePath, err)
	}

	g := graph.NewPipelineGraph("ansible-playbook", "Ansible Playbook Graph")

	for idx, play := range plays {
		targetHosts := extractScalarOrListString(&play.Hosts)
		playName := strings.TrimSpace(play.Name)
		if playName == "" {
			if targetHosts != "" {
				playName = fmt.Sprintf("Play %d (%s)", idx+1, targetHosts)
			} else {
				playName = fmt.Sprintf("Play %d", idx+1)
			}
		}

		nodeID := "ansible-play-" + slugify(playName)
		roles := extractAnsibleRoles(play.Roles)

		allTasks := make([]map[string]interface{}, 0, len(play.PreTasks)+len(play.Tasks)+len(play.PostTasks))
		allTasks = append(allTasks, play.PreTasks...)
		allTasks = append(allTasks, play.Tasks...)
		allTasks = append(allTasks, play.PostTasks...)

		taskModules, taskNames, imageRepo, containerImage := extractAnsibleTasks(allTasks)

		var ansTags []string
		if targetHosts != "" {
			ansTags = appendUnique(ansTags, "hosts:"+targetHosts)
		}
		if len(roles) > 0 {
			ansTags = appendUnique(ansTags, "roles:"+strings.Join(roles, ","))
		}
		if len(allTasks) > 0 {
			ansTags = appendUnique(ansTags, fmt.Sprintf("tasks:%d", len(allTasks)))
		}
		if containerImage != "" {
			ansTags = appendUnique(ansTags, "tag:"+extractImageTag(containerImage))
		}

		meta := map[string]interface{}{
			"file_path":    filePath,
			"target_hosts": targetHosts,
			"roles":        roles,
			"tasks":        taskModules,
			"task_names":   taskNames,
			"task_count":   len(allTasks),
			"tags":         ansTags,
		}
		if imageRepo != "" {
			meta["image_repo"] = imageRepo
			meta["container_image"] = containerImage
			meta["image_tag"] = extractImageTag(containerImage)
		}

		if err := g.AddNode(graph.Node{
			ID:        nodeID,
			Label:     playName,
			Stage:     graph.StageConfigMgmt,
			Tool:      graph.ToolAnsible,
			Status:    graph.StatusIdle,
			ZoomLevel: graph.ZoomJobOrService,
			Metadata:  meta,
		}); err != nil {
			return nil, err
		}
	}

	return g, nil
}

func extractAnsibleRoles(roleNodes []yaml.Node) []string {
	var roles []string
	for _, rn := range roleNodes {
		switch rn.Kind {
		case yaml.ScalarNode:
			roles = appendUnique(roles, rn.Value)
		case yaml.MappingNode:
			for i := 0; i+1 < len(rn.Content); i += 2 {
				k := rn.Content[i].Value
				v := rn.Content[i+1].Value
				if (k == "role" || k == "name") && v != "" {
					roles = appendUnique(roles, v)
				}
			}
		}
	}
	return roles
}

func extractAnsibleTasks(tasks []map[string]interface{}) (modules []string, names []string, imageRepo string, containerImage string) {
	for _, task := range tasks {
		if rawName, ok := task["name"].(string); ok && strings.TrimSpace(rawName) != "" {
			names = appendUnique(names, strings.TrimSpace(rawName))
		}

		for key, val := range task {
			if _, ignored := nonModuleTaskKeys[key]; ignored {
				continue
			}

			shortMod := normalizeAnsibleModuleName(key)
			modules = appendUnique(modules, shortMod)

			if shortMod == "docker_container" {
				if modMap, ok := val.(map[string]interface{}); ok {
					if rawImg, ok := modMap["image"].(string); ok {
						if repo, full := splitImageRepoAndTag(rawImg); repo != "" {
							imageRepo = repo
							containerImage = full
						}
					}
				}
			}
		}
	}
	return modules, names, imageRepo, containerImage
}

func normalizeAnsibleModuleName(moduleKey string) string {
	trimmed := strings.TrimSpace(moduleKey)
	if lastDot := strings.LastIndex(trimmed, "."); lastDot != -1 && lastDot+1 < len(trimmed) {
		return trimmed[lastDot+1:]
	}
	return trimmed
}
