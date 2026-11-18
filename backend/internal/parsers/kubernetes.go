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

// KubernetesParser statically parses multi-document Kubernetes YAML manifests
// for Deployment, StatefulSet, Service, and Ingress resources.
type KubernetesParser struct{}

// NewKubernetesParser returns a new KubernetesParser instance.
func NewKubernetesParser() *KubernetesParser {
	return &KubernetesParser{}
}

type k8sManifest struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name      string            `yaml:"name"`
		Namespace string            `yaml:"namespace"`
		Labels    map[string]string `yaml:"labels"`
	} `yaml:"metadata"`
	Spec struct {
		// Workload (Deployment / StatefulSet) fields
		Replicas *int `yaml:"replicas"`
		Selector struct {
			MatchLabels map[string]string `yaml:"matchLabels"`
		} `yaml:"selector"`
		Template struct {
			Metadata struct {
				Labels map[string]string `yaml:"labels"`
			} `yaml:"metadata"`
			Spec struct {
				Containers []struct {
					Name  string `yaml:"name"`
					Image string `yaml:"image"`
				} `yaml:"containers"`
			} `yaml:"spec"`
		} `yaml:"template"`

		// Service fields (note: Service selector is a flat map[string]string in raw YAML)
		Ports []struct {
			Port       int         `yaml:"port"`
			TargetPort interface{} `yaml:"targetPort"`
			Protocol   string      `yaml:"protocol"`
		} `yaml:"ports"`

		// Ingress fields
		DefaultBackend *k8sIngressBackend `yaml:"defaultBackend"`
		Rules          []struct {
			Host string `yaml:"host"`
			HTTP struct {
				Paths []struct {
					Path     string            `yaml:"path"`
					PathType string            `yaml:"pathType"`
					Backend  k8sIngressBackend `yaml:"backend"`
				} `yaml:"paths"`
			} `yaml:"http"`
		} `yaml:"rules"`
	} `yaml:"spec"`
}

type k8sIngressBackend struct {
	ServiceName string `yaml:"serviceName"`
	Service     struct {
		Name string `yaml:"name"`
	} `yaml:"service"`
}

type k8sServiceSpecRaw struct {
	Spec struct {
		Selector map[string]string `yaml:"selector"`
	} `yaml:"spec"`
}

type parsedWorkload struct {
	NodeID    string
	Name      string
	Namespace string
	Kind      string
	Labels    map[string]string
}

type parsedService struct {
	NodeID    string
	Name      string
	Namespace string
	Selector  map[string]string
}

type parsedIngress struct {
	NodeID          string
	Name            string
	Namespace       string
	BackendServices []string
}

// Parse implements the Parser interface for multi-document Kubernetes YAML files.
func (p *KubernetesParser) Parse(filePath string, content []byte) (*graph.PipelineGraph, error) {
	g := graph.NewPipelineGraph("k8s-manifests", "Kubernetes Runtime Graph")

	var (
		workloads []parsedWorkload
		services  []parsedService
		ingresses []parsedIngress
	)

	docs := bytes.Split(content, []byte("\n---"))
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	docIdx := 0

	for {
		var m k8sManifest
		err := decoder.Decode(&m)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to parse Kubernetes manifest %s: %w", filePath, err)
		}
		rawDoc := []byte{}
		if docIdx < len(docs) {
			rawDoc = docs[docIdx]
		}
		docIdx++

		kind := strings.TrimSpace(m.Kind)
		name := strings.TrimSpace(m.Metadata.Name)
		if kind == "" || name == "" {
			continue
		}
		ns := strings.TrimSpace(m.Metadata.Namespace)
		if ns == "" {
			ns = "default"
		}

		switch kind {
		case "Deployment", "StatefulSet":
			prefix := "k8s-deploy-"
			if kind == "StatefulSet" {
				prefix = "k8s-sts-"
			}
			nodeID := prefix + slugify(name)

			labels := make(map[string]string)
			for k, v := range m.Metadata.Labels {
				labels[k] = v
			}
			for k, v := range m.Spec.Selector.MatchLabels {
				labels[k] = v
			}
			for k, v := range m.Spec.Template.Metadata.Labels {
				labels[k] = v
			}

			var images []string
			var primaryRepo, primaryImage string
			var containerPorts []string
			for _, c := range m.Spec.Template.Spec.Containers {
				repo, full := splitImageRepoAndTag(c.Image)
				if full != "" {
					images = appendUnique(images, full)
					if primaryImage == "" {
						primaryRepo = repo
						primaryImage = full
					}
				}
			}

			var wlTags []string
			wlTags = appendUnique(wlTags, "ns:"+ns)
			if m.Spec.Replicas != nil {
				wlTags = appendUnique(wlTags, fmt.Sprintf("replicas:%d", *m.Spec.Replicas))
			}
			if primaryImage != "" {
				wlTags = appendUnique(wlTags, "tag:"+extractImageTag(primaryImage))
			}
			for k, v := range labels {
				wlTags = appendUnique(wlTags, fmt.Sprintf("%s=%s", k, v))
			}
			for _, cp := range containerPorts {
				wlTags = appendUnique(wlTags, "port:"+cp)
			}

			meta := map[string]interface{}{
				"file_path": filePath,
				"kind":      kind,
				"namespace": ns,
				"labels":    labels,
				"tags":      wlTags,
			}
			if m.Spec.Replicas != nil {
				meta["replicas"] = *m.Spec.Replicas
			}
			if primaryImage != "" {
				meta["container_image"] = primaryImage
				meta["image_repo"] = primaryRepo
				meta["image_tag"] = extractImageTag(primaryImage)
				meta["images"] = images
			}

			if err := g.AddNode(graph.Node{
				ID:        nodeID,
				Label:     fmt.Sprintf("%s: %s", kind, name),
				Stage:     graph.StageRuntime,
				Tool:      graph.ToolKubernetes,
				Status:    graph.StatusIdle,
				ZoomLevel: graph.ZoomJobOrService,
				Metadata:  meta,
			}); err != nil {
				return nil, err
			}

			workloads = append(workloads, parsedWorkload{
				NodeID:    nodeID,
				Name:      name,
				Namespace: ns,
				Kind:      kind,
				Labels:    labels,
			})

		case "Service":
			nodeID := "k8s-svc-" + slugify(name)
			var svcRaw k8sServiceSpecRaw
			_ = yaml.Unmarshal(rawDoc, &svcRaw)

			selector := svcRaw.Spec.Selector
			if selector == nil {
				selector = make(map[string]string)
			}

			var svcTags []string
			svcTags = appendUnique(svcTags, "ns:"+ns)
			for _, p := range m.Spec.Ports {
				proto := strings.ToUpper(strings.TrimSpace(p.Protocol))
				if proto == "" {
					proto = "TCP"
				}
				if p.TargetPort != nil {
					svcTags = appendUnique(svcTags, fmt.Sprintf("port:%d->%v/%s", p.Port, p.TargetPort, proto))
				} else if p.Port > 0 {
					svcTags = appendUnique(svcTags, fmt.Sprintf("port:%d/%s", p.Port, proto))
				}
			}
			for k, v := range selector {
				svcTags = appendUnique(svcTags, fmt.Sprintf("sel:%s=%s", k, v))
			}

			meta := map[string]interface{}{
				"file_path": filePath,
				"kind":      "Service",
				"namespace": ns,
				"selector":  selector,
				"tags":      svcTags,
			}

			if err := g.AddNode(graph.Node{
				ID:        nodeID,
				Label:     fmt.Sprintf("Service: %s", name),
				Stage:     graph.StageRuntime,
				Tool:      graph.ToolKubernetes,
				Status:    graph.StatusIdle,
				ZoomLevel: graph.ZoomGranularResource,
				Metadata:  meta,
			}); err != nil {
				return nil, err
			}

			services = append(services, parsedService{
				NodeID:    nodeID,
				Name:      name,
				Namespace: ns,
				Selector:  selector,
			})

		case "Ingress":
			nodeID := "k8s-ing-" + slugify(name)
			var backends []string
			var hosts []string

			if m.Spec.DefaultBackend != nil {
				if svcName := extractBackendServiceName(*m.Spec.DefaultBackend); svcName != "" {
					backends = appendUnique(backends, svcName)
				}
			}
			for _, rule := range m.Spec.Rules {
				if strings.TrimSpace(rule.Host) != "" {
					hosts = appendUnique(hosts, strings.TrimSpace(rule.Host))
				}
				for _, pth := range rule.HTTP.Paths {
					if svcName := extractBackendServiceName(pth.Backend); svcName != "" {
						backends = appendUnique(backends, svcName)
					}
				}
			}

			var ingTags []string
			ingTags = appendUnique(ingTags, "ns:"+ns)
			for _, h := range hosts {
				ingTags = appendUnique(ingTags, "host:"+h)
			}
			for _, b := range backends {
				ingTags = appendUnique(ingTags, "svc:"+b)
			}

			meta := map[string]interface{}{
				"file_path":        filePath,
				"kind":             "Ingress",
				"namespace":        ns,
				"backend_services": backends,
				"hosts":            hosts,
				"tags":             ingTags,
			}

			if err := g.AddNode(graph.Node{
				ID:        nodeID,
				Label:     fmt.Sprintf("Ingress: %s", name),
				Stage:     graph.StageRuntime,
				Tool:      graph.ToolKubernetes,
				Status:    graph.StatusIdle,
				ZoomLevel: graph.ZoomGranularResource,
				Metadata:  meta,
			}); err != nil {
				return nil, err
			}

			ingresses = append(ingresses, parsedIngress{
				NodeID:          nodeID,
				Name:            name,
				Namespace:       ns,
				BackendServices: backends,
			})
		}
	}

	// Link Ingress -> Service via deploys_to edges
	svcByName := make(map[string]parsedService, len(services))
	for _, svc := range services {
		svcByName[svc.Name] = svc
	}
	for _, ing := range ingresses {
		for _, backendSvcName := range ing.BackendServices {
			if targetSvc, ok := svcByName[backendSvcName]; ok {
				if err := g.AddEdge(graph.Edge{
					Source:       ing.NodeID,
					Target:       targetSvc.NodeID,
					RelationType: graph.RelationDeploysTo,
					Label:        "routes to",
				}); err != nil {
					return nil, err
				}
			}
		}
	}

	// Link Service -> Deployment / StatefulSet via deploys_to edges when label selectors match
	for _, svc := range services {
		if len(svc.Selector) == 0 {
			continue
		}
		for _, wl := range workloads {
			if selectorMatchesLabels(svc.Selector, wl.Labels) {
				if err := g.AddEdge(graph.Edge{
					Source:       svc.NodeID,
					Target:       wl.NodeID,
					RelationType: graph.RelationDeploysTo,
					Label:        "selects",
				}); err != nil {
					return nil, err
				}
			}
		}
	}

	return g, nil
}

func extractBackendServiceName(b k8sIngressBackend) string {
	if strings.TrimSpace(b.Service.Name) != "" {
		return strings.TrimSpace(b.Service.Name)
	}
	return strings.TrimSpace(b.ServiceName)
}

func selectorMatchesLabels(selector, labels map[string]string) bool {
	if len(selector) == 0 || len(labels) == 0 {
		return false
	}
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}
