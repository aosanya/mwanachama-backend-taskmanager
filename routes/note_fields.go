package routes

import "github.com/aosanya/mwanachama-backend-shared/dispatch"

var noteFields = map[string]dispatch.FieldDoc{
	"RuleNoteDraft.kind":                  {Description: "note, contradiction, compatible or question. note when left out.", Values: []string{"note", "contradiction", "compatible", "question"}},
	"RuleNoteDraft.source":                {Description: "person, worker or judge. person when left out.", Values: []string{"person", "worker", "judge"}},
	"RuleNoteDraft.text":                  {Description: "The note, a line or a longer write-up.", Required: true},
	"RuleNoteDraft.related_codes":         {Description: "Codes of the other records the note is about, as a list of strings such as R-9 or D-3."},
	"RuleNoteDraft.links":                 {Description: "Links to the work, as a list of strings."},
	"RuleNoteResolution.resolution":       {Description: "How it was settled, in words."},
	"RuleNoteResolution.resolved_by_code": {Description: "The code of the rule or decision that settled it."},
}
