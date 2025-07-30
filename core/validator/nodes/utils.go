package nodes

import (
	"fmt"
	"strings"
)


func GetDependencyIDs(nodes []*BaseNode[any]) []string {
	ids := make([]string, len(nodes))
	for i, node := range nodes {
		ids[i] = node.id
	}
	return ids
}


func BuildNodeID(path, nodeType, suffix string) string {
	id := fmt.Sprintf("%s:%s", path, nodeType)
	if path == "" {
		id = nodeType
	}
	if suffix != "" {
		id = fmt.Sprintf("%s:%s", id, suffix)
	}
	return id
}

func getScopedPath(path string) string {
	if !strings.Contains(path, ".") {
		return ""
	}
	parts := strings.Split(path, ".")
	return strings.Join(parts[:len(parts)-1], ".")
}
