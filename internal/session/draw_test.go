package session

import (
	"math/rand"
	"reflect"
	"testing"
)

// TestDrawTeamsCompleteAndEven checks the draw is complete (every player appears
// exactly once) and even (each team holds teamSize players, with the right
// number of teams), and that it does not mutate the input.
func TestDrawTeamsCompleteAndEven(t *testing.T) {
	ids := []string{"a", "b", "c", "d"}
	rng := rand.New(rand.NewSource(42))

	teams := drawTeams(ids, 2, rng)

	if len(teams) != 2 {
		t.Fatalf("got %d teams, want 2", len(teams))
	}
	seen := map[string]int{}
	for _, team := range teams {
		if len(team) != 2 {
			t.Fatalf("team has %d members, want 2", len(team))
		}
		for _, id := range team {
			seen[id]++
		}
	}
	if len(seen) != len(ids) {
		t.Fatalf("got %d distinct players, want %d", len(seen), len(ids))
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("player %q appears %d times, want 1", id, n)
		}
	}
	if !reflect.DeepEqual(ids, []string{"a", "b", "c", "d"}) {
		t.Errorf("input slice was mutated: %v", ids)
	}
}

// TestDrawTeamsDeterministic verifies the same seed reproduces the same draw, so
// tests (and any future replay) are deterministic.
func TestDrawTeamsDeterministic(t *testing.T) {
	ids := []string{"a", "b", "c", "d", "e", "f"}

	first := drawTeams(ids, 3, rand.New(rand.NewSource(7)))
	second := drawTeams(ids, 3, rand.New(rand.NewSource(7)))

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same seed gave different draws:\n%v\n%v", first, second)
	}
}

// TestDrawTeamsShuffles checks the draw actually randomizes order: across a range
// of seeds it must at least once produce an order different from the input.
func TestDrawTeamsShuffles(t *testing.T) {
	ids := []string{"a", "b", "c", "d"}

	shuffled := false
	for seed := int64(0); seed < 50; seed++ {
		teams := drawTeams(ids, 2, rand.New(rand.NewSource(seed)))
		flat := append(append([]string{}, teams[0]...), teams[1]...)
		if !reflect.DeepEqual(flat, ids) {
			shuffled = true
			break
		}
	}
	if !shuffled {
		t.Fatal("draw never changed the input order across 50 seeds")
	}
}

func TestTeamLabel(t *testing.T) {
	for i, want := range []string{"A", "B", "C"} {
		if got := teamLabel(i); got != want {
			t.Errorf("teamLabel(%d) = %q, want %q", i, got, want)
		}
	}
}
