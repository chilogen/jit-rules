package hooksjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

type member struct {
	key   string
	value any
}

type object struct {
	members []member
}

func decodeObject(data []byte) (*object, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	value, err := decodeValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values")
		}
		return nil, err
	}
	result, ok := value.(*object)
	if !ok {
		return nil, fmt.Errorf("target is not a JSON object")
	}
	return result, nil
}

func decodeValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	switch delim {
	case '{':
		return decodeMembers(decoder)
	case '[':
		return decodeArray(decoder)
	default:
		return nil, fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
}

func decodeMembers(decoder *json.Decoder) (*object, error) {
	result := &object{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		value, err := decodeValue(decoder)
		if err != nil {
			return nil, err
		}
		result.members = append(result.members, member{key: key.(string), value: value})
	}
	_, err := decoder.Token()
	return result, err
}

func decodeArray(decoder *json.Decoder) ([]any, error) {
	var result []any
	for decoder.More() {
		value, err := decodeValue(decoder)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	_, err := decoder.Token()
	return result, err
}

func encodeObject(value *object) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func (o *object) MarshalJSON() ([]byte, error) {
	var output bytes.Buffer
	output.WriteByte('{')
	for i, item := range o.members {
		if i > 0 {
			output.WriteByte(',')
		}
		key, _ := compactJSON(item.key)
		output.Write(key)
		output.WriteByte(':')
		value, err := compactJSON(item.value)
		if err != nil {
			return nil, err
		}
		output.Write(value)
	}
	output.WriteByte('}')
	return output.Bytes(), nil
}

func compactJSON(value any) ([]byte, error) {
	if nested, ok := value.(*object); ok {
		return nested.MarshalJSON()
	}
	if values, ok := value.([]any); ok {
		var output bytes.Buffer
		output.WriteByte('[')
		for i, item := range values {
			if i > 0 {
				output.WriteByte(',')
			}
			data, err := compactJSON(item)
			if err != nil {
				return nil, err
			}
			output.Write(data)
		}
		output.WriteByte(']')
		return output.Bytes(), nil
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(output.Bytes(), []byte{'\n'}), nil
}

func (o *object) get(key string) (any, bool) {
	for _, item := range o.members {
		if item.key == key {
			return item.value, true
		}
	}
	return nil, false
}

func (o *object) set(key string, value any) {
	for i := range o.members {
		if o.members[i].key == key {
			o.members[i].value = value
			return
		}
	}
	o.members = append(o.members, member{key: key, value: value})
}

func (o *object) remove(key string) {
	for i := range o.members {
		if o.members[i].key == key {
			o.members = append(o.members[:i], o.members[i+1:]...)
			return
		}
	}
}

func normalize(value any) any {
	mapping, ok := value.(map[string]any)
	if !ok {
		if values, ok := value.([]any); ok {
			result := make([]any, len(values))
			for i := range values {
				result[i] = normalize(values[i])
			}
			return result
		}
		return value
	}
	keys := make([]string, 0, len(mapping))
	for key := range mapping {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := &object{}
	for _, key := range keys {
		result.set(key, normalize(mapping[key]))
	}
	return result
}
