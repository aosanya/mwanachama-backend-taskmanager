package mwanachamataskmanager_test

import (
	"fmt"
	"testing"
)

func TestRuleSubjects_TagsCameFromTaggingRowsWhenTheIssueOwnTagsAreEmpty(t *testing.T) {
	for _, count := range []int{499, 500, 501, 1000, 1001} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			mgr, db, table := newTestManagerWithDB(t)
			for i := 0; i < count; i++ {
				issueTagged(t, mgr, fmt.Sprintf("issue %d", i), "css", "app")
			}
			if err := db.Table(table).Where("1 = 1").Update("tags", nil).Error; err != nil {
				t.Fatal(err)
			}
			ruleInForceWhen(t, mgr, "Use rem", tagWhen("css"))
			ruleInForceWhen(t, mgr, "Keep it small", tagWhen("app"))

			got := overlaps(t, mgr)

			if len(got) != 1 || len(got[0].SharedIssues) != count {
				t.Fatalf("overlaps %+v, want one pair sharing %d issues", got, count)
			}
		})
	}
}
