package gormstore

import (
	"encoding/json"

	"gorm.io/datatypes"
)

// stringsToJSON marshals a []string into a datatypes.JSON column value.
// Returns nil for an empty slice so the column stores SQL NULL rather than
// the literal "[]" — mirrors the rest of this package's "empty means unset"
// convention.
func stringsToJSON(ss []string) datatypes.JSON {
	if len(ss) == 0 {
		return nil
	}
	b, _ := json.Marshal(ss)
	return datatypes.JSON(b)
}

// jsonToStrings unmarshals a datatypes.JSON column value back into a
// []string. Returns nil for an empty/absent column.
func jsonToStrings(j datatypes.JSON) []string {
	if len(j) == 0 {
		return nil
	}
	var out []string
	if err := json.Unmarshal(j, &out); err != nil {
		return nil
	}
	return out
}

// intsToJSON marshals a []int into a datatypes.JSON column value. Returns
// nil for an empty slice.
func intsToJSON(ii []int) datatypes.JSON {
	if len(ii) == 0 {
		return nil
	}
	b, _ := json.Marshal(ii)
	return datatypes.JSON(b)
}

// jsonToInts unmarshals a datatypes.JSON column value back into a []int.
// Returns nil for an empty/absent column.
func jsonToInts(j datatypes.JSON) []int {
	if len(j) == 0 {
		return nil
	}
	var out []int
	if err := json.Unmarshal(j, &out); err != nil {
		return nil
	}
	return out
}
