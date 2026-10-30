package routes_test

import (
	"encoding/json"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func voteRanked(t *testing.T, body []byte) mwanachamataskmanager.RankedCase {
	t.Helper()
	var r mwanachamataskmanager.RankedCase
	if err := json.Unmarshal(body, &r); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return r
}

func TestAMemberVotesTakesTheVoteBackAndTheListFollowsTheVotes(t *testing.T) {
	m := w73Mux(wk11Manager(t))
	quiet := w73Open(t, m, "amina")
	loud := w73Open(t, m, "amina")

	rec := w73Do(t, m, "juma", "POST", "/cases/"+loud.ID+"/votes", map[string]any{})
	if rec.Code != 200 || voteRanked(t, rec.Body.Bytes()).Votes != 2 {
		t.Fatalf("vote %d %s", rec.Code, rec.Body)
	}
	rec = w73Do(t, m, "hub-steward", "POST", "/cases/"+loud.ID+"/votes", map[string]any{"member_id": "neema"})
	if rec.Code != 200 || voteRanked(t, rec.Body.Bytes()).Votes != 3 {
		t.Fatalf("vote for neema %d %s", rec.Code, rec.Body)
	}

	rec = w73Do(t, m, "juma", "GET", "/cases/ranked", nil)
	var ranked []mwanachamataskmanager.RankedCase
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &ranked) != nil || len(ranked) != 2 {
		t.Fatalf("ranked %d %s", rec.Code, rec.Body)
	}
	if ranked[0].ID != loud.ID || ranked[0].Votes != 3 || ranked[1].ID != quiet.ID || ranked[1].Votes != 1 {
		t.Fatalf("ranked %+v", ranked)
	}

	rec = w73Do(t, m, "juma", "GET", "/cases/"+loud.ID+"/votes", nil)
	var votes []mwanachamataskmanager.CaseVote
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &votes) != nil || len(votes) != 2 {
		t.Fatalf("votes %d %s", rec.Code, rec.Body)
	}
	castFor := map[string]string{}
	for _, v := range votes {
		castFor[v.MemberID] = v.CastBy
	}
	if castFor["juma"] != "juma" || castFor["neema"] != "hub-steward" {
		t.Fatalf("who cast what: %+v", castFor)
	}

	rec = w73Do(t, m, "juma", "DELETE", "/cases/"+loud.ID+"/votes/juma", nil)
	if rec.Code != 200 || voteRanked(t, rec.Body.Bytes()).Votes != 2 {
		t.Fatalf("take back %d %s", rec.Code, rec.Body)
	}
}

func TestOwnersAndAgentsAreRefusedAsVotersOverRest(t *testing.T) {
	m := w73Mux(wk11Manager(t))
	c := w73Open(t, m, "amina")
	for _, kind := range []string{"owner", "agent"} {
		rec := w73Do(t, m, "boss", "POST", "/cases/"+c.ID+"/votes", map[string]any{"member_kind": kind})
		if rec.Code != 403 {
			t.Fatalf("%s %d %s", kind, rec.Code, rec.Body)
		}
	}
}

func TestVoteRefusalsKeepTheirStatusCodes(t *testing.T) {
	m := w73Mux(wk11Manager(t))
	c := w73Open(t, m, "amina")
	if rec := w73Do(t, m, "amina", "DELETE", "/cases/"+c.ID+"/votes/amina", nil); rec.Code != 409 {
		t.Fatalf("requester take back %d %s", rec.Code, rec.Body)
	}
	if rec := w73Do(t, m, "juma", "DELETE", "/cases/"+c.ID+"/votes/juma", nil); rec.Code != 404 {
		t.Fatalf("no vote to take back %d %s", rec.Code, rec.Body)
	}
	if rec := w73Do(t, m, "juma", "POST", "/cases/nope/votes", map[string]any{}); rec.Code != 404 {
		t.Fatalf("unknown case %d %s", rec.Code, rec.Body)
	}
	if rec := w73Do(t, m, "", "POST", "/cases/"+c.ID+"/votes", map[string]any{}); rec.Code != 400 {
		t.Fatalf("no caller %d %s", rec.Code, rec.Body)
	}
}
