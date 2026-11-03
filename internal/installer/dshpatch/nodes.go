package dshpatch

import "gopkg.in/yaml.v3"

func findEntry(root *yaml.Node, id string) (int, int, bool) {
	for outerIndex, outer := range root.Content {
		insert := mappingValue(outer, "insert")
		if insert == nil || insert.Kind != yaml.SequenceNode {
			continue
		}
		for innerIndex, entry := range insert.Content {
			entryID := mappingValue(entry, "id")
			if entryID != nil && entryID.Value == id {
				return outerIndex, innerIndex, true
			}
		}
	}
	return 0, 0, false
}

func upsertEntry(root *yaml.Node, id, plugin, configPath string) {
	entry := entryNode(id, plugin, configPath)
	outerIndex, innerIndex, found := findEntry(root, id)
	if found {
		insert := mappingValue(root.Content[outerIndex], "insert")
		insert.Content[innerIndex] = entry
		return
	}
	root.Content = append(root.Content, &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			scalar("insert"),
			{Kind: yaml.SequenceNode, Content: []*yaml.Node{entry}},
		},
	})
}

func removeEntry(root *yaml.Node, id string) {
	outerIndex, innerIndex, found := findEntry(root, id)
	if !found {
		return
	}
	insert := mappingValue(root.Content[outerIndex], "insert")
	insert.Content = append(insert.Content[:innerIndex], insert.Content[innerIndex+1:]...)
	if len(insert.Content) == 0 {
		root.Content = append(root.Content[:outerIndex], root.Content[outerIndex+1:]...)
	}
}

func entryNode(id, plugin, configPath string) *yaml.Node {
	return &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			scalar("id"), scalar(id),
			scalar("name"), {
				Kind:  yaml.ScalarNode,
				Tag:   "!!str",
				Value: plugin,
				Style: yaml.SingleQuotedStyle,
			},
			scalar("config"), {
				Kind: yaml.MappingNode,
				Content: []*yaml.Node{
					scalar("configPath"), scalar(configPath),
				},
			},
		},
	}
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func scalar(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}
