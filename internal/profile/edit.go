package profile

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chilogen/jit-rules/internal/fileutil"
	"gopkg.in/yaml.v3"
)

func addPack(dir string, pack PackRef) error {
	return editProfile(dir, func(mapping *yaml.Node) error {
		packs := sequenceValue(mapping, "packs", true)
		for _, item := range packs.Content {
			if mappingScalar(item, "name") == pack.Name {
				return fmt.Errorf("pack %s already exists", pack.Name)
			}
		}
		var encoded yaml.Node
		if err := encoded.Encode(pack); err != nil {
			return err
		}
		packs.Content = append(packs.Content, &encoded)
		return nil
	})
}

func removePack(dir, name string) error {
	return editProfile(dir, func(mapping *yaml.Node) error {
		packs := sequenceValue(mapping, "packs", false)
		if packs == nil {
			return fmt.Errorf("pack %s not found", name)
		}
		for index, item := range packs.Content {
			if mappingScalar(item, "name") == name {
				packs.Content = append(packs.Content[:index], packs.Content[index+1:]...)
				return nil
			}
		}
		return fmt.Errorf("pack %s not found", name)
	})
}

func setPackRef(dir, name, ref string) error {
	return editProfile(dir, func(mapping *yaml.Node) error {
		packs := sequenceValue(mapping, "packs", false)
		if packs == nil {
			return fmt.Errorf("pack %s not found", name)
		}
		for _, item := range packs.Content {
			if mappingScalar(item, "name") != name {
				continue
			}
			for index := 0; index+1 < len(item.Content); index += 2 {
				if item.Content[index].Value != "ref" {
					continue
				}
				if ref == "" {
					item.Content = append(item.Content[:index], item.Content[index+2:]...)
				} else {
					item.Content[index+1].Value = ref
				}
				return nil
			}
			if ref != "" {
				item.Content = append(item.Content,
					&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "ref"},
					&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: ref},
				)
			}
			return nil
		}
		return fmt.Errorf("pack %s not found", name)
	})
}

func addDisable(dir, id string) error {
	return editProfile(dir, func(mapping *yaml.Node) error {
		disabled := sequenceValue(mapping, "disable", true)
		for _, item := range disabled.Content {
			if item.Value == id {
				return nil
			}
		}
		disabled.Content = append(disabled.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: id},
		)
		return nil
	})
}

func removeDisable(dir, id string) error {
	return editProfile(dir, func(mapping *yaml.Node) error {
		disabled := sequenceValue(mapping, "disable", false)
		if disabled == nil {
			return nil
		}
		for index, item := range disabled.Content {
			if item.Value == id {
				disabled.Content = append(disabled.Content[:index], disabled.Content[index+1:]...)
				return nil
			}
		}
		return nil
	})
}

func editProfile(dir string, edit func(*yaml.Node) error) error {
	path := filepath.Join(dir, "profile.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return err
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return errors.New("profile.yaml must contain a mapping")
	}
	if err := edit(document.Content[0]); err != nil {
		return err
	}
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	return fileutil.WriteAtomic(path, output.Bytes(), 0o600)
}

func sequenceValue(mapping *yaml.Node, key string, create bool) *yaml.Node {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1]
		}
	}
	if !create {
		return nil
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valueNode := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	mapping.Content = append(mapping.Content, keyNode, valueNode)
	return valueNode
}

func mappingScalar(mapping *yaml.Node, key string) string {
	if mapping.Kind != yaml.MappingNode {
		return ""
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1].Value
		}
	}
	return ""
}
