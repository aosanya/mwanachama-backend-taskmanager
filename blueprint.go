package mwanachamataskmanager

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/spec"
)

//go:embed taskmanager.blueprint.json
var blueprintJSON []byte

//go:embed spec/examples/agency.taskmanager.json
var domainJSON []byte

var loadBlueprint = sync.OnceValues(func() (*spec.Blueprint, error) {
	return spec.ParseBlueprint(blueprintJSON)
})

func Blueprint() (*spec.Blueprint, error) { return loadBlueprint() }

func LoadSpec(path string) (*spec.Spec, error) {
	b, err := Blueprint()
	if err != nil {
		return nil, err
	}
	return b.Load(path)
}

func ParseSpec(raw []byte) (*spec.Spec, error) {
	b, err := Blueprint()
	if err != nil {
		return nil, err
	}
	return b.Parse(raw)
}

func SpecFor(instance string) (*spec.Spec, error) {
	return SpecForMount(instance, "")
}

func SpecForMount(instance, mount string) (*spec.Spec, error) {
	var doc map[string]any
	if err := json.Unmarshal(domainJSON, &doc); err != nil {
		return nil, fmt.Errorf("taskmanager spec: %w", err)
	}
	doc["instance"] = instance
	if mount != "" {
		doc["mount"] = mount
	}

	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("taskmanager spec: %w", err)
	}
	return ParseSpec(raw)
}

func Provision(db *gorm.DB, s *spec.Spec) error {
	return spec.Migrate(db, s)
}
