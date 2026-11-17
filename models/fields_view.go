package models

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

func KnownFields(item any) map[string]bool {
	known := map[string]bool{}
	t := reflect.TypeOf(item)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		switch name {
		case "-":
		case "":
			known[field.Name] = true
		default:
			known[name] = true
		}
	}
	return known
}

func ChooseFields(asked []string, item any) ([]string, error) {
	known := KnownFields(item)
	var chosen []string
	seen := map[string]bool{}
	for _, entry := range asked {
		for _, name := range strings.Split(entry, ",") {
			name = strings.TrimSpace(name)
			if name == "" || seen[name] {
				continue
			}
			if !known[name] {
				return nil, fmt.Errorf("fields: %q is not a field of the item", name)
			}
			seen[name] = true
			chosen = append(chosen, name)
		}
	}
	return chosen, nil
}

func KeepFields(item json.RawMessage, fields []string) (json.RawMessage, error) {
	var whole map[string]json.RawMessage
	if err := json.Unmarshal(item, &whole); err != nil {
		return nil, err
	}
	kept := make(map[string]json.RawMessage, len(fields))
	for _, name := range fields {
		if value, ok := whole[name]; ok {
			kept[name] = value
		}
	}
	return json.Marshal(kept)
}

func KeepFieldsOfEach(items any, fields []string) (json.RawMessage, error) {
	encoded, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	var each []json.RawMessage
	if err := json.Unmarshal(encoded, &each); err != nil {
		return nil, err
	}
	for i, item := range each {
		if each[i], err = KeepFields(item, fields); err != nil {
			return nil, err
		}
	}
	return json.Marshal(each)
}

func MarshalWithItems(whole any, fields []string) ([]byte, error) {
	if len(fields) == 0 {
		return json.Marshal(whole)
	}
	encoded, err := json.Marshal(whole)
	if err != nil {
		return nil, err
	}
	var parts map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &parts); err != nil {
		return nil, err
	}
	if parts["items"], err = KeepFieldsOfEach(parts["items"], fields); err != nil {
		return nil, err
	}
	return json.Marshal(parts)
}

type pendingTasksJSON PendingTasks

func (p PendingTasks) MarshalJSON() ([]byte, error) {
	return MarshalWithItems(pendingTasksJSON(p), p.Fields)
}
