package terraform

import (
	"fmt"

	"github.com/hashicorp/terraform/config"
	"github.com/hashicorp/terraform/dag"
)

// GraphNodeReferenceable must be implemented by any node that represents
// a Terraform thing that can be referenced (resource, module, etc.).
type GraphNodeReferenceable interface {
	// ReferenceableName is the name by which this can be referenced.
	// This can be either just the type, or include the field. Example:
	// "aws_instance.bar" or "aws_instance.bar.id".
	ReferenceableName() []string
}

// GraphNodeReferencer must be implemented by nodes that reference other
// Terraform items and therefore depend on them.
type GraphNodeReferencer interface {
	// References are the list of things that this node references. This
	// can include fields or just the type, just like GraphNodeReferenceable
	// above.
	References() []string
}

// ReferenceTransformer is a GraphTransformer that connects all the
// nodes that reference each other in order to form the proper ordering.
type ReferenceTransformer struct{}

func (t *ReferenceTransformer) Transform(g *Graph) error {
	// Build the mapping of reference => vertex for efficient lookups.
	refMap := make(map[string][]dag.Vertex)
	for _, v := range g.Vertices() {
		// We're only looking for referenceable nodes
		rn, ok := v.(GraphNodeReferenceable)
		if !ok {
			continue
		}

		// If this node represents a sub path then we prefix
		var prefix string
		if pn, ok := v.(GraphNodeSubPath); ok {
			if path := normalizeModulePath(pn.Path()); len(path) > 1 {
				prefix = modulePrefixStr(path[1:]) + "."
			}
		}

		// Go through and cache them
		for _, n := range rn.ReferenceableName() {
			n = prefix + n
			refMap[n] = append(refMap[n], v)
		}
	}

	// Find the things that reference things and connect them
	for _, v := range g.Vertices() {
		rn, ok := v.(GraphNodeReferencer)
		if !ok {
			continue
		}

		// If this node represents a sub path then we prefix
		var prefix string
		if pn, ok := v.(GraphNodeSubPath); ok {
			if path := normalizeModulePath(pn.Path()); len(path) > 1 {
				prefix = modulePrefixStr(path[1:]) + "."
			}
		}

		for _, n := range rn.References() {
			n = prefix + n
			if parents, ok := refMap[n]; ok {
				for _, parent := range parents {
					g.Connect(dag.BasicEdge(v, parent))
				}
			}
		}
	}

	return nil
}

// ReferencesFromConfig returns the references that a configuration has
// based on the interpolated variables in a configuration.
func ReferencesFromConfig(c *config.RawConfig) []string {
	var result []string
	for _, v := range c.Variables {
		if r := ReferenceFromInterpolatedVar(v); r != "" {
			result = append(result, r)
		}

	}

	return result
}

// ReferenceFromInterpolatedVar returns the reference from this variable,
// or an empty string if there is no reference.
func ReferenceFromInterpolatedVar(v config.InterpolatedVariable) string {
	switch v := v.(type) {
	case *config.ModuleVariable:
		return fmt.Sprintf("module.%s.output.%s", v.Name, v.Field)
	case *config.ResourceVariable:
		return v.ResourceId()
	case *config.UserVariable:
		return fmt.Sprintf("var.%s", v.Name)
	default:
		return ""
	}
}
