package session

import "math/rand"

// drawTeams randomly partitions userIDs into consecutive teams of teamSize each.
// It shuffles a copy (the input slice is left untouched) with the supplied RNG,
// then slices the shuffled order into teams. The number of teams is
// len(userIDs)/teamSize; callers guarantee len(userIDs) is a multiple of teamSize
// (the activity defines requiredPlayers = teamSize * numTeams).
//
// The RNG is injected so tests can seed it and assert a deterministic outcome.
func drawTeams(userIDs []string, teamSize int, rng *rand.Rand) [][]string {
	shuffled := make([]string, len(userIDs))
	copy(shuffled, userIDs)
	rng.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	teams := make([][]string, 0, len(shuffled)/teamSize)
	for i := 0; i+teamSize <= len(shuffled); i += teamSize {
		teams = append(teams, shuffled[i:i+teamSize])
	}
	return teams
}

// teamLabel maps a zero-based team index to its display label: 0 -> "A", 1 -> "B".
func teamLabel(i int) string {
	return string(rune('A' + i))
}
