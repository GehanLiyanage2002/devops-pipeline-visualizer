package parsers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/pipescope/backend/internal/graph"
	"github.com/zclconf/go-cty/cty"
)

// TerraformParser statically parses Terraform .tf HCL files and optional .tfstate JSON files.
type TerraformParser struct{}

// NewTerraformParser returns a new TerraformParser instance.
func NewTerraformParser() *TerraformParser {
	return &TerraformParser{}
}

type parsedTFResource struct {
	ResourceType string
	ResourceName string
	NodeID       string
	GroupID      string
	Attributes   map[string]string
	References   []string // e.g., "aws_vpc.main"
}

// Parse implements the Parser interface for Terraform HCL (.tf) and state (.tfstate) files.
func (p *TerraformParser) Parse(filePath string, content []byte) (*graph.PipelineGraph, error) {
	if strings.HasSuffix(strings.ToLower(filePath), ".tfstate") {
		return p.parseTFState(filePath, content)
	}
	return p.parseHCL(filePath, content)
}

func (p *TerraformParser) parseHCL(filePath string, content []byte) (*graph.PipelineGraph, error) {
	file, diags := hclsyntax.ParseConfig(content, filePath, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse Terraform HCL %s: %s", filePath, diags.Error())
	}

	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, fmt.Errorf("unexpected HCL body type in %s", filePath)
	}

	g := graph.NewPipelineGraph("terraform-infra", "Terraform Infrastructure Graph")

	var providers []string
	providerRegions := make(map[string]string)
	var resources []*parsedTFResource
	resByAddress := make(map[string]*parsedTFResource)

	for _, block := range body.Blocks {
		switch block.Type {
		case "provider":
			if len(block.Labels) > 0 {
				provName := normalizeCloudProvider(block.Labels[0])
				providers = appendUnique(providers, provName)
				attrs := extractHCLScalarAttributes(block.Body)
				if region, ok := attrs["region"]; ok {
					providerRegions[provName] = region
				}
			}

		case "resource":
			if len(block.Labels) < 2 {
				continue
			}
			resType := strings.TrimSpace(block.Labels[0])
			resName := strings.TrimSpace(block.Labels[1])
			address := resType + "." + resName
			nodeID := fmt.Sprintf("tf-%s-%s", slugify(resType), slugify(resName))

			attrs := extractHCLScalarAttributes(block.Body)
			refs := extractHCLResourceReferences(block.Body)

			res := &parsedTFResource{
				ResourceType: resType,
				ResourceName: resName,
				NodeID:       nodeID,
				Attributes:   attrs,
				References:   refs,
			}

			// Determine if this resource also defines a logical NodeGroup boundary
			if grp, ok := buildTFNodeGroup(resType, resName, attrs); ok {
				res.GroupID = grp.ID
				if err := g.AddGroup(grp); err != nil {
					return nil, err
				}
			}

			resources = append(resources, res)
			resByAddress[address] = res
		}
	}

	// Second pass: resolve parent groups, add IaC Terraform nodes, and create provisions edges.
	for _, res := range resources {
		var parentGroupID string
		for _, refAddr := range res.References {
			if targetRes, ok := resByAddress[refAddr]; ok {
				if targetRes.GroupID != "" && targetRes.GroupID != res.GroupID && parentGroupID == "" {
					parentGroupID = targetRes.GroupID
				}
			}
		}

		// If this resource created a group and is nested inside another group (e.g. EKS inside VPC),
		// update the group's ParentGroupID.
		if res.GroupID != "" && parentGroupID != "" {
			_ = g.AddGroup(graph.NodeGroup{
				ID:            res.GroupID,
				ParentGroupID: parentGroupID,
			})
		}

		nodeParentGroup := res.GroupID
		if nodeParentGroup == "" {
			nodeParentGroup = parentGroupID
		}

		meta := map[string]interface{}{
			"file_path":     filePath,
			"resource_type": res.ResourceType,
			"resource_name": res.ResourceName,
			"tf_address":    res.ResourceType + "." + res.ResourceName,
		}
		if len(providers) > 0 {
			meta["providers"] = providers
		}
		prov := inferProviderFromResourceType(res.ResourceType)
		if prov != "" {
			meta["cloud_provider"] = prov
			if reg, ok := providerRegions[prov]; ok {
				meta["region"] = reg
			}
		}

		// Container registries (aws_ecr_repository, azurerm_container_registry)
		if res.ResourceType == "aws_ecr_repository" || res.ResourceType == "azurerm_container_registry" {
			regName := res.Attributes["name"]
			if regName == "" {
				regName = res.ResourceName
			}
			meta["registry_name"] = regName
			meta["image_repo"] = regName
		}

		// Compute instances (aws_instance) -> record target_hosts for Ansible correlation
		if res.ResourceType == "aws_instance" {
			hostTag := res.Attributes["tags.Role"]
			if hostTag == "" {
				hostTag = res.Attributes["tags.Name"]
			}
			if hostTag == "" {
				hostTag = res.ResourceName
			}
			meta["target_hosts"] = hostTag
		}

		label := fmt.Sprintf("%s.%s", res.ResourceType, res.ResourceName)
		if nameAttr := res.Attributes["name"]; nameAttr != "" {
			label = fmt.Sprintf("%s (%s)", label, nameAttr)
		}

		if err := g.AddNode(graph.Node{
			ID:            res.NodeID,
			Label:         label,
			Stage:         graph.StageIaC,
			Tool:          graph.ToolTerraform,
			Status:        graph.StatusIdle,
			ZoomLevel:     graph.ZoomJobOrService,
			ParentGroupID: nodeParentGroup,
			Metadata:      meta,
		}); err != nil {
			return nil, err
		}

		// Create provisions edges for implicit resource references (e.g., vpc_id = aws_vpc.main.id)
		for _, refAddr := range res.References {
			if upstreamRes, ok := resByAddress[refAddr]; ok && upstreamRes.NodeID != res.NodeID {
				if err := g.AddEdge(graph.Edge{
					Source:       upstreamRes.NodeID,
					Target:       res.NodeID,
					RelationType: graph.RelationProvisions,
					Label:        "provisions",
				}); err != nil {
					return nil, err
				}
			}
		}
	}

	return g, nil
}

type tfStateFile struct {
	Resources []struct {
		Mode      string `json:"mode"`
		Type      string `json:"type"`
		Name      string `json:"name"`
		Provider  string `json:"provider"`
		Instances []struct {
			Attributes   map[string]interface{} `json:"attributes"`
			Dependencies []string               `json:"dependencies"`
		} `json:"instances"`
	} `json:"resources"`
}

func (p *TerraformParser) parseTFState(filePath string, content []byte) (*graph.PipelineGraph, error) {
	var state tfStateFile
	if err := json.Unmarshal(content, &state); err != nil {
		return nil, fmt.Errorf("failed to parse terraform.tfstate %s: %w", filePath, err)
	}

	g := graph.NewPipelineGraph("terraform-state", "Terraform State Graph")
	nodeIDByAddr := make(map[string]string)

	for _, r := range state.Resources {
		if r.Mode != "" && r.Mode != "managed" {
			continue
		}
		addr := r.Type + "." + r.Name
		nodeID := fmt.Sprintf("tf-%s-%s", slugify(r.Type), slugify(r.Name))
		nodeIDByAddr[addr] = nodeID

		attrs := make(map[string]string)
		if len(r.Instances) > 0 {
			for k, v := range r.Instances[0].Attributes {
				if s, ok := v.(string); ok {
					attrs[k] = s
				}
			}
		}

		var groupID string
		if grp, ok := buildTFNodeGroup(r.Type, r.Name, attrs); ok {
			groupID = grp.ID
			_ = g.AddGroup(grp)
		}

		meta := map[string]interface{}{
			"file_path":     filePath,
			"resource_type": r.Type,
			"resource_name": r.Name,
			"tf_address":    addr,
		}
		if r.Type == "aws_ecr_repository" || r.Type == "azurerm_container_registry" {
			regName := attrs["name"]
			if regName == "" {
				regName = r.Name
			}
			meta["registry_name"] = regName
		}

		_ = g.AddNode(graph.Node{
			ID:            nodeID,
			Label:         addr,
			Stage:         graph.StageIaC,
			Tool:          graph.ToolTerraform,
			Status:        graph.StatusSuccess,
			ZoomLevel:     graph.ZoomJobOrService,
			ParentGroupID: groupID,
			Metadata:      meta,
		})
	}

	for _, r := range state.Resources {
		targetID := nodeIDByAddr[r.Type+"."+r.Name]
		if targetID == "" || len(r.Instances) == 0 {
			continue
		}
		for _, dep := range r.Instances[0].Dependencies {
			if srcID, ok := nodeIDByAddr[dep]; ok && srcID != targetID {
				_ = g.AddEdge(graph.Edge{
					Source:       srcID,
					Target:       targetID,
					RelationType: graph.RelationProvisions,
					Label:        "provisions",
				})
			}
		}
	}

	return g, nil
}

func buildTFNodeGroup(resType, resName string, attrs map[string]string) (graph.NodeGroup, bool) {
	displayName := attrs["name"]
	if displayName == "" {
		displayName = attrs["tags.Name"]
	}
	if displayName == "" {
		displayName = resName
	}

	switch resType {
	case "aws_vpc":
		return graph.NodeGroup{
			ID:           "tf-group-aws-vpc-" + slugify(resName),
			Label:        fmt.Sprintf("AWS VPC: %s", displayName),
			Provider:     graph.ProviderAWS,
			ResourceType: graph.GroupResourceVPC,
		}, true
	case "aws_eks_cluster":
		return graph.NodeGroup{
			ID:           "tf-group-aws-eks-" + slugify(resName),
			Label:        fmt.Sprintf("AWS EKS Cluster: %s", displayName),
			Provider:     graph.ProviderAWS,
			ResourceType: graph.GroupResourceEKSCluster,
		}, true
	case "azurerm_virtual_network":
		return graph.NodeGroup{
			ID:           "tf-group-azure-vnet-" + slugify(resName),
			Label:        fmt.Sprintf("Azure VNet: %s", displayName),
			Provider:     graph.ProviderAzure,
			ResourceType: graph.GroupResourceVNet,
		}, true
	case "azurerm_kubernetes_cluster":
		return graph.NodeGroup{
			ID:           "tf-group-azure-aks-" + slugify(resName),
			Label:        fmt.Sprintf("Azure AKS Cluster: %s", displayName),
			Provider:     graph.ProviderAzure,
			ResourceType: graph.GroupResourceAKSCluster,
		}, true
	case "aws_instance":
		return graph.NodeGroup{
			ID:           "tf-group-aws-instance-" + slugify(resName),
			Label:        fmt.Sprintf("AWS EC2 Instance Group: %s", displayName),
			Provider:     graph.ProviderAWS,
			ResourceType: graph.GroupResourceVPC,
		}, true
	default:
		return graph.NodeGroup{}, false
	}
}

func extractHCLScalarAttributes(body *hclsyntax.Body) map[string]string {
	out := make(map[string]string)
	if body == nil {
		return out
	}

	for name, attr := range body.Attributes {
		if val, diags := attr.Expr.Value(nil); !diags.HasErrors() {
			switch val.Type() {
			case cty.String:
				out[name] = val.AsString()
			default:
				if val.Type().IsObjectType() || val.Type().IsMapType() {
					for k, v := range val.AsValueMap() {
						if v.Type() == cty.String {
							out[name+"."+k] = v.AsString()
						}
					}
				}
			}
		}
	}

	for _, blk := range body.Blocks {
		nested := extractHCLScalarAttributes(blk.Body)
		for k, v := range nested {
			out[blk.Type+"."+k] = v
		}
	}
	return out
}

func extractHCLResourceReferences(body *hclsyntax.Body) []string {
	var refs []string
	if body == nil {
		return refs
	}

	for _, attr := range body.Attributes {
		for _, traversal := range attr.Expr.Variables() {
			if ref := traversalToResourceAddress(traversal); ref != "" {
				refs = appendUnique(refs, ref)
			}
		}
	}
	for _, blk := range body.Blocks {
		refs = appendUnique(refs, extractHCLResourceReferences(blk.Body)...)
	}
	return refs
}

func traversalToResourceAddress(t hcl.Traversal) string {
	if len(t) < 2 {
		return ""
	}
	root, ok := t[0].(hcl.TraverseRoot)
	if !ok {
		return ""
	}
	// Ignore built-in HCL roots that are not direct managed resource types
	switch root.Name {
	case "var", "local", "module", "data", "path", "terraform", "each", "count", "self":
		return ""
	}
	attr, ok := t[1].(hcl.TraverseAttr)
	if !ok {
		return ""
	}
	return root.Name + "." + attr.Name
}

func normalizeCloudProvider(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "aws":
		return "aws"
	case "azurerm", "azure", "azuread":
		return "azure"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func inferProviderFromResourceType(resType string) string {
	if strings.HasPrefix(resType, "aws_") {
		return "aws"
	}
	if strings.HasPrefix(resType, "azurerm_") || strings.HasPrefix(resType, "azure") {
		return "azure"
	}
	return ""
}
