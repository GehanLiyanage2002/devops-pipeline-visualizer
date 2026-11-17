package parsers

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/pipescope/backend/internal/graph"
	"gopkg.in/yaml.v3"
)

// GitHubActionsParser statically parses GitHub Actions workflow files (.github/workflows/*.yml).
type GitHubActionsParser struct{}

// NewGitHubActionsParser returns a new GitHubActionsParser instance.
func NewGitHubActionsParser() *GitHubActionsParser {
	return &GitHubActionsParser{}
}

type ghaWorkflow struct {
	Name string    `yaml:"name"`
	On   yaml.Node `yaml:"on"`
	Jobs yaml.Node `yaml:"jobs"`
}

type ghaJob struct {
	Name   string    `yaml:"name"`
	RunsOn yaml.Node `yaml:"runs-on"`
	Needs  yaml.Node `yaml:"needs"`
	Steps  []ghaStep `yaml:"steps"`
}

type ghaStep struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	With map[string]string `yaml:"with"`
}

var (
	dockerBuildTagRegex  = regexp.MustCompile(`docker\s+build\b[^\n]*?(?:-t|--tag)\s+([^\s\\]+)`)
	dockerPushImageRegex = regexp.MustCompile(`docker\s+push\s+([^\s\\]+)`)
	ansiblePlaybookRegex = regexp.MustCompile(`ansible-playbook\b[^\n]*?\s+([^\s\\]+\.ya?ml)`)
)

// Parse implements the Parser interface for GitHub Actions YAML files.
func (p *GitHubActionsParser) Parse(filePath string, content []byte) (*graph.PipelineGraph, error) {
	var wf ghaWorkflow
	if err := yaml.Unmarshal(content, &wf); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub Actions workflow %s: %w", filePath, err)
	}

	wfName := strings.TrimSpace(wf.Name)
	if wfName == "" {
		wfName = "GitHub Actions Workflow"
	}

	g := graph.NewPipelineGraph("gha-"+slugify(wfName), wfName)

	triggers, branches := extractGHATriggers(&wf.On)
	sourceNodeID := "gh-source-repo"
	sourceLabel := "GitHub Repository"
	if len(branches) > 0 {
		sourceLabel = fmt.Sprintf("GitHub Repository (%s)", strings.Join(branches, ", "))
	}

	sourceMeta := map[string]interface{}{
		"file_path":     filePath,
		"workflow_name": wfName,
		"triggers":      triggers,
	}
	if len(branches) > 0 {
		sourceMeta["branches"] = branches
	}

	if err := g.AddNode(graph.Node{
		ID:        sourceNodeID,
		Label:     sourceLabel,
		Stage:     graph.StageSource,
		Tool:      graph.ToolGitHub,
		Status:    graph.StatusIdle,
		ZoomLevel: graph.ZoomHighLevelStage,
		Metadata:  sourceMeta,
	}); err != nil {
		return nil, err
	}

	// Parse jobs in declaration order from the YAML mapping node
	if wf.Jobs.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(wf.Jobs.Content); i += 2 {
			jobKeyNode := wf.Jobs.Content[i]
			jobValNode := wf.Jobs.Content[i+1]

			jobID := strings.TrimSpace(jobKeyNode.Value)
			var job ghaJob
			if err := jobValNode.Decode(&job); err != nil {
				return nil, fmt.Errorf("failed to decode job %q in %s: %w", jobID, filePath, err)
			}

			nodeID := "gha-job-" + slugify(jobID)
			jobLabel := strings.TrimSpace(job.Name)
			if jobLabel == "" {
				jobLabel = jobID
			}

			runsOn := extractScalarOrListString(&job.RunsOn)
			needs := extractStringList(&job.Needs)

			jobMeta := map[string]interface{}{
				"file_path": filePath,
				"job_id":    jobID,
				"runs_on":   runsOn,
			}

			scanGHASteps(job.Steps, jobMeta)

			if err := g.AddNode(graph.Node{
				ID:        nodeID,
				Label:     jobLabel,
				Stage:     graph.StageCI,
				Tool:      graph.ToolGitHubActions,
				Status:    graph.StatusIdle,
				ZoomLevel: graph.ZoomJobOrService,
				Metadata:  jobMeta,
			}); err != nil {
				return nil, err
			}

			if len(needs) == 0 {
				if err := g.AddEdge(graph.Edge{
					Source:       sourceNodeID,
					Target:       nodeID,
					RelationType: graph.RelationTriggers,
					Label:        "triggers",
				}); err != nil {
					return nil, err
				}
			} else {
				for _, depJobID := range needs {
					depNodeID := "gha-job-" + slugify(depJobID)
					if err := g.AddEdge(graph.Edge{
						Source:       depNodeID,
						Target:       nodeID,
						RelationType: graph.RelationTriggers,
						Label:        "needs",
					}); err != nil {
						return nil, err
					}
				}
			}
		}
	}

	return g, nil
}

func extractGHATriggers(onNode *yaml.Node) (triggers []string, branches []string) {
	if onNode == nil || onNode.Kind == 0 {
		return nil, nil
	}

	switch onNode.Kind {
	case yaml.ScalarNode:
		triggers = appendUnique(triggers, onNode.Value)
	case yaml.SequenceNode:
		for _, item := range onNode.Content {
			if item.Kind == yaml.ScalarNode {
				triggers = appendUnique(triggers, item.Value)
			}
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(onNode.Content); i += 2 {
			eventKey := onNode.Content[i].Value
			eventVal := onNode.Content[i+1]
			triggers = appendUnique(triggers, eventKey)

			if eventVal.Kind == yaml.MappingNode {
				for j := 0; j+1 < len(eventVal.Content); j += 2 {
					subKey := eventVal.Content[j].Value
					subVal := eventVal.Content[j+1]
					if subKey == "branches" || subKey == "branches-ignore" {
						branches = appendUnique(branches, extractStringList(subVal)...)
					}
				}
			}
		}
	}
	return triggers, branches
}

func extractStringList(n *yaml.Node) []string {
	if n == nil || n.Kind == 0 {
		return nil
	}
	var out []string
	switch n.Kind {
	case yaml.ScalarNode:
		out = appendUnique(out, n.Value)
	case yaml.SequenceNode:
		for _, item := range n.Content {
			if item.Kind == yaml.ScalarNode {
				out = appendUnique(out, item.Value)
			}
		}
	}
	return out
}

func extractScalarOrListString(n *yaml.Node) string {
	list := extractStringList(n)
	return strings.Join(list, ", ")
}

func scanGHASteps(steps []ghaStep, meta map[string]interface{}) {
	var cloudProviders []string

	for _, step := range steps {
		usesLower := strings.ToLower(strings.TrimSpace(step.Uses))
		runCmd := strings.TrimSpace(step.Run)
		runLower := strings.ToLower(runCmd)

		// 1. Docker build/push action or CLI detection
		if strings.Contains(usesLower, "docker/build-push-action") {
			rawTag := step.With["tags"]
			if rawTag == "" {
				rawTag = step.With["images"]
			}
			if repo, full := splitImageRepoAndTag(rawTag); repo != "" {
				meta["image_repo"] = repo
				meta["container_image"] = full
			}
		}
		if matches := dockerBuildTagRegex.FindStringSubmatch(runCmd); len(matches) > 1 {
			if repo, full := splitImageRepoAndTag(matches[1]); repo != "" {
				meta["image_repo"] = repo
				meta["container_image"] = full
			}
		} else if matches := dockerPushImageRegex.FindStringSubmatch(runCmd); len(matches) > 1 {
			if repo, full := splitImageRepoAndTag(matches[1]); repo != "" {
				meta["image_repo"] = repo
				meta["container_image"] = full
			}
		}

		// 2. Cloud auth detection (AWS / Azure)
		if strings.Contains(usesLower, "aws-actions/configure-aws-credentials") {
			cloudProviders = appendUnique(cloudProviders, "aws")
			meta["cloud_provider"] = "aws"
			if region, ok := step.With["aws-region"]; ok && strings.TrimSpace(region) != "" {
				meta["aws_region"] = strings.TrimSpace(region)
			}
		}
		if strings.Contains(usesLower, "azure/login") {
			cloudProviders = appendUnique(cloudProviders, "azure")
			meta["cloud_provider"] = "azure"
		}

		// 3. Terraform execution steps
		if strings.Contains(usesLower, "hashicorp/setup-terraform") ||
			strings.Contains(runLower, "terraform init") ||
			strings.Contains(runLower, "terraform plan") ||
			strings.Contains(runLower, "terraform apply") {
			meta["executes_terraform"] = true
		}

		// 4. Ansible execution steps
		if strings.Contains(usesLower, "ansible-playbook") ||
			strings.Contains(runLower, "ansible-playbook") {
			meta["executes_ansible"] = true
			if matches := ansiblePlaybookRegex.FindStringSubmatch(runCmd); len(matches) > 1 {
				meta["ansible_playbook"] = strings.TrimSpace(matches[1])
			} else if pb, ok := step.With["playbook"]; ok && strings.TrimSpace(pb) != "" {
				meta["ansible_playbook"] = strings.TrimSpace(pb)
			}
		}
	}

	if len(cloudProviders) > 0 {
		meta["cloud_providers"] = cloudProviders
	}
}
