package session

import (
	"math"
	"testing"
)

// twoTeams builds two labelled teams (A, B) from member ratings, with synthetic
// user ids so deltas can be looked up.
func twoTeams(a, b []int) []Team {
	mk := func(label string, elos []int) Team {
		members := make([]Participant, len(elos))
		for i, e := range elos {
			members[i] = Participant{UserID: label + string(rune('0'+i)), Team: label, Elo: e}
		}
		return Team{Label: label, Members: members}
	}
	return []Team{mk("A", a), mk("B", b)}
}

func TestComputeEloDeltasEqualRatingsWin(t *testing.T) {
	// Equal mean ratings: expected score 0.5 each, so the winner gains K/2 and
	// the loser drops K/2 (K=32 -> ±16).
	teams := twoTeams([]int{1000, 1000}, []int{1000, 1000})
	deltas := computeEloDeltas(teams, "A")

	for _, m := range teams[0].Members {
		if deltas[m.UserID] != 16 {
			t.Errorf("winner %s delta = %d, want 16", m.UserID, deltas[m.UserID])
		}
	}
	for _, m := range teams[1].Members {
		if deltas[m.UserID] != -16 {
			t.Errorf("loser %s delta = %d, want -16", m.UserID, deltas[m.UserID])
		}
	}
}

func TestComputeEloDeltasSumToZero(t *testing.T) {
	// Lopsided ratings, but equal team sizes: the deltas across all players must
	// still net to zero (no rating is created or destroyed).
	cases := []string{"A", "B", ""}
	for _, winner := range cases {
		teams := twoTeams([]int{1200, 1100}, []int{900, 950})
		deltas := computeEloDeltas(teams, winner)
		sum := 0
		for _, d := range deltas {
			sum += d
		}
		if sum != 0 {
			t.Errorf("winner=%q: deltas sum to %d, want 0", winner, sum)
		}
	}
}

func TestComputeEloDeltasFavoriteWinsLittle(t *testing.T) {
	// A strong favourite that wins gains less than an even match would; the same
	// favourite losing drops by a lot. Underdog mirrors it.
	favWin := computeEloDeltas(twoTeams([]int{1400, 1400}, []int{1000, 1000}), "A")
	favLose := computeEloDeltas(twoTeams([]int{1400, 1400}, []int{1000, 1000}), "B")

	gain := favWin["A0"]
	loss := favLose["A0"]
	if gain <= 0 || gain >= 16 {
		t.Errorf("favourite win gain = %d, want between 1 and 15", gain)
	}
	if loss >= 0 || -loss <= 16 {
		t.Errorf("favourite loss = %d, want a drop larger than 16", loss)
	}
}

func TestComputeEloDeltasDrawShiftsTowardUnderdog(t *testing.T) {
	// A draw is a good result for the underdog (it gains) and a bad one for the
	// favourite (it drops).
	deltas := computeEloDeltas(twoTeams([]int{1300, 1300}, []int{1000, 1000}), "")
	if deltas["A0"] >= 0 {
		t.Errorf("favourite draw delta = %d, want negative", deltas["A0"])
	}
	if deltas["B0"] <= 0 {
		t.Errorf("underdog draw delta = %d, want positive", deltas["B0"])
	}
}

func TestComputeEloDeltasNonTwoTeamsIsNoop(t *testing.T) {
	one := []Team{{Label: "A", Members: []Participant{{UserID: "a", Elo: 1000}}}}
	if got := computeEloDeltas(one, "A"); got != nil {
		t.Errorf("single-team deltas = %v, want nil (no-op)", got)
	}
	if got := computeEloDeltas(nil, ""); got != nil {
		t.Errorf("zero-team deltas = %v, want nil (no-op)", got)
	}
}

func TestEloExpectedSymmetry(t *testing.T) {
	// Expected scores of the two sides always sum to 1.
	a := eloExpected(1234, 987)
	b := eloExpected(987, 1234)
	if math.Abs(a+b-1) > 1e-9 {
		t.Errorf("expected scores %f + %f != 1", a, b)
	}
}
