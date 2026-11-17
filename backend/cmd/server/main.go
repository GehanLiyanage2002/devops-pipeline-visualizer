package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/pipescope/backend/internal/api"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	sampleDir := resolveSamplePipelineDir()
	overridesPath := filepath.Join("data", "overrides.json")

	srv := api.NewServer(sampleDir, overridesPath)
	addr := ":" + port

	log.Printf("PipeScope backend server listening on http://localhost%s (sample-pipeline: %s)", addr, sampleDir)
	if err := http.ListenAndServe(addr, srv.Router()); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}

func resolveSamplePipelineDir() string {
	candidates := []string{
		filepath.Join("..", "sample-pipeline"),
		"sample-pipeline",
		filepath.Join("..", "..", "sample-pipeline"),
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			abs, err := filepath.Abs(c)
			if err == nil {
				return abs
			}
			return c
		}
	}
	return filepath.Join("..", "sample-pipeline")
}
