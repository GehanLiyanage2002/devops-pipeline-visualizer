package parsers

import (
	"bufio"
	"bytes"
	"strings"

	"github.com/pipescope/backend/internal/graph"
)

// DockerfileParser statically parses Dockerfile configurations.
type DockerfileParser struct{}

// NewDockerfileParser returns a new DockerfileParser instance.
func NewDockerfileParser() *DockerfileParser {
	return &DockerfileParser{}
}

// Parse implements the Parser interface for Dockerfiles.
func (p *DockerfileParser) Parse(filePath string, content []byte) (*graph.PipelineGraph, error) {
	g := graph.NewPipelineGraph("dockerfile-graph", "Dockerfile Artifact Graph")

	var (
		baseImages   []string
		buildStages  []string
		exposedPorts []string
	)
	stageSet := make(map[string]struct{})

	logicalLines := joinDockerfileContinuationLines(content)
	for _, line := range logicalLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}

		instruction := strings.ToUpper(fields[0])
		switch instruction {
		case "FROM":
			imageRef, stageName := parseFromInstruction(fields[1:])
			if imageRef != "" {
				// Only record as external base image if it does not reference an earlier stage alias
				if _, isInternalStage := stageSet[strings.ToLower(imageRef)]; !isInternalStage {
					baseImages = appendUnique(baseImages, imageRef)
				}
			}
			if stageName != "" {
				stageSet[strings.ToLower(stageName)] = struct{}{}
				buildStages = appendUnique(buildStages, stageName)
			}

		case "EXPOSE":
			for _, portToken := range fields[1:] {
				cleanedPort := strings.TrimSpace(portToken)
				if cleanedPort != "" && !strings.HasPrefix(cleanedPort, "#") {
					exposedPorts = appendUnique(exposedPorts, cleanedPort)
				}
			}
		}
	}

	meta := map[string]interface{}{
		"file_path":     filePath,
		"base_images":   baseImages,
		"build_stages":  buildStages,
		"exposed_ports": exposedPorts,
	}

	label := "Docker Image"
	if len(baseImages) > 0 {
		label = "Docker Image (" + baseImages[len(baseImages)-1] + ")"
	}

	if err := g.AddNode(graph.Node{
		ID:        "docker-image",
		Label:     label,
		Stage:     graph.StageArtifact,
		Tool:      graph.ToolDocker,
		Status:    graph.StatusIdle,
		ZoomLevel: graph.ZoomJobOrService,
		Metadata:  meta,
	}); err != nil {
		return nil, err
	}

	return g, nil
}

func joinDockerfileContinuationLines(content []byte) []string {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	var lines []string
	var current strings.Builder

	for scanner.Scan() {
		raw := strings.TrimRight(scanner.Text(), "\r ")
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "#") {
			if current.Len() == 0 {
				lines = append(lines, trimmed)
			}
			continue
		}
		if strings.HasSuffix(raw, `\`) {
			current.WriteString(strings.TrimSuffix(raw, `\`))
			current.WriteString(" ")
			continue
		}
		current.WriteString(raw)
		lines = append(lines, current.String())
		current.Reset()
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return lines
}

func parseFromInstruction(args []string) (imageRef string, stageName string) {
	idx := 0
	for idx < len(args) && strings.HasPrefix(args[idx], "--") {
		idx++
	}
	if idx >= len(args) {
		return "", ""
	}
	imageRef = args[idx]
	idx++

	if idx+1 < len(args) && strings.EqualFold(args[idx], "AS") {
		stageName = args[idx+1]
	}
	return imageRef, stageName
}
