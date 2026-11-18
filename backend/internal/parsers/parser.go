package parsers

import (
	"regexp"
	"strings"

	"github.com/pipescope/backend/internal/graph"
)

// Parser is the common interface implemented by all static configuration parsers.
// Every parser accepts a file path and raw file content and returns a partial PipelineGraph.
type Parser interface {
	Parse(filePath string, content []byte) (*graph.PipelineGraph, error)
}

var nonAlnumRegex = regexp.MustCompile(`[^a-z0-9]+`)

// slugify converts an arbitrary string into a deterministic lowercase hyphen-separated slug.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonAlnumRegex.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "unknown"
	}
	return s
}

// splitImageRepoAndTag separates an image string (e.g., "ghcr.io/pipescope/api:latest")
// into its repository path ("ghcr.io/pipescope/api") and normalized full image string.
func splitImageRepoAndTag(raw string) (repo string, full string) {
	cleaned := strings.TrimSpace(raw)
	// Take first line or comma-separated entry if multiple tags were passed
	for _, sep := range []string{"\n", ","} {
		if idx := strings.Index(cleaned, sep); idx != -1 {
			cleaned = strings.TrimSpace(cleaned[:idx])
		}
	}
	cleaned = strings.Trim(cleaned, `"'`)
	if cleaned == "" {
		return "", ""
	}
	full = cleaned

	// Split tag after the last '/' so registry ports (localhost:5000/app:v1) are preserved.
	lastSlash := strings.LastIndex(cleaned, "/")
	lastColon := strings.LastIndex(cleaned, ":")
	if lastColon > lastSlash {
		repo = cleaned[:lastColon]
	} else {
		repo = cleaned
	}
	return repo, full
}

// extractImageTag returns the tag portion of a container image reference (e.g. "latest" from "ghcr.io/org/api:latest").
func extractImageTag(full string) string {
	cleaned := strings.TrimSpace(full)
	if cleaned == "" {
		return ""
	}
	lastSlash := strings.LastIndex(cleaned, "/")
	lastColon := strings.LastIndex(cleaned, ":")
	if lastColon > lastSlash && lastColon+1 < len(cleaned) {
		return cleaned[lastColon+1:]
	}
	return "latest"
}

func appendUnique(slice []string, items ...string) []string {
	seen := make(map[string]struct{}, len(slice))
	for _, existing := range slice {
		seen[existing] = struct{}{}
	}
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; !ok {
			seen[trimmed] = struct{}{}
			slice = append(slice, trimmed)
		}
	}
	return slice
}
