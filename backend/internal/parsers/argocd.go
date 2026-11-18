package parsers

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/pipescope/backend/internal/graph"
	"gopkg.in/yaml.v3"
)

// ArgoCDParser statically parses Argo CD Application YAML manifests.
type ArgoCDParser struct{}

// NewArgoCDParser returns a new ArgoCDParser instance.
func NewArgoCDParser() *ArgoCDParser {
	return &ArgoCDParser{}
}

type argoApplicationManifest struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
	Spec struct {
		Project string `yaml:"project"`
		Source  struct {
			RepoURL        string `yaml:"repoURL"`
			Path           string `yaml:"path"`
			TargetRevision string `yaml:"targetRevision"`
		} `yaml:"source"`
		Destination struct {
			Server    string `yaml:"server"`
			Namespace string `yaml:"namespace"`
		} `yaml:"destination"`
	} `yaml:"spec"`
}

// Parse implements the Parser interface for Argo CD Application YAML manifests.
func (p *ArgoCDParser) Parse(filePath string, content []byte) (*graph.PipelineGraph, error) {
	g := graph.NewPipelineGraph("argocd-gitops", "Argo CD GitOps Graph")

	decoder := yaml.NewDecoder(bytes.NewReader(content))
	for {
		var app argoApplicationManifest
		err := decoder.Decode(&app)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to parse Argo CD manifest %s: %w", filePath, err)
		}

		if !strings.EqualFold(strings.TrimSpace(app.Kind), "Application") ||
			!strings.HasPrefix(strings.ToLower(strings.TrimSpace(app.APIVersion)), "argoproj.io/") {
			continue
		}

		appName := strings.TrimSpace(app.Metadata.Name)
		if appName == "" {
			appName = "application"
		}

		nodeID := "argocd-app-" + slugify(appName)
		syncPath := strings.TrimSpace(app.Spec.Source.Path)
		targetRev := strings.TrimSpace(app.Spec.Source.TargetRevision)
		destNS := strings.TrimSpace(app.Spec.Destination.Namespace)
		project := strings.TrimSpace(app.Spec.Project)

		var argoTags []string
		if syncPath != "" {
			argoTags = appendUnique(argoTags, "path:"+syncPath)
		}
		if targetRev != "" {
			argoTags = appendUnique(argoTags, "rev:"+targetRev)
		}
		if destNS != "" {
			argoTags = appendUnique(argoTags, "ns:"+destNS)
		}
		if project != "" {
			argoTags = appendUnique(argoTags, "proj:"+project)
		}

		meta := map[string]interface{}{
			"file_path":             filePath,
			"project":               project,
			"repo_url":              strings.TrimSpace(app.Spec.Source.RepoURL),
			"sync_path":             syncPath,
			"target_revision":       targetRev,
			"destination_server":    strings.TrimSpace(app.Spec.Destination.Server),
			"destination_namespace": destNS,
			"tags":                  argoTags,
		}

		if err := g.AddNode(graph.Node{
			ID:        nodeID,
			Label:     fmt.Sprintf("Argo CD App: %s", appName),
			Stage:     graph.StageGitOps,
			Tool:      graph.ToolArgoCD,
			Status:    graph.StatusIdle,
			ZoomLevel: graph.ZoomJobOrService,
			Metadata:  meta,
		}); err != nil {
			return nil, err
		}
	}

	return g, nil
}
