package session

import "math"

// ELO rating constants. EloDefault must match the DB default in the
// 0004_user_elo migration: it is the single source of truth for a fresh
// player's rating, the migration only mirrors it for the SQL default.
const (
	EloDefault = 1000 // starting rating for every player
	eloK       = 32   // K-factor: the maximum rating swing of a single match
)

// eloExpected returns the expected score (between 0 and 1) of a side rated
// `rating` against an opponent rated `opp`, via the standard logistic formula.
// It is the probability-like share of the win the rating difference predicts.
func eloExpected(rating, opp float64) float64 {
	return 1 / (1 + math.Pow(10, (opp-rating)/400))
}

// meanElo is the average current rating of a team's members, used as the team's
// rating for the match. An empty team has no rating and yields the default.
func meanElo(members []Participant) float64 {
	if len(members) == 0 {
		return EloDefault
	}
	sum := 0
	for _, m := range members {
		sum += m.Elo
	}
	return float64(sum) / float64(len(members))
}

// computeEloDeltas returns the rating change to apply to every member of a
// two-team match, keyed by user id. Each team is scored as a whole against the
// opponent's mean rating; the resulting team delta is handed to each of its
// members. winnerLabel is the winning team's label, or "" for a draw (each side
// scores 0.5). It is deliberately limited to exactly two teams — the only shape
// the seeded activity produces — and returns nil (a no-op) for any other count,
// leaving the caller to log it.
//
// The two team deltas are exact negatives of each other (the expected scores
// sum to 1, the actual scores sum to 1, and rounding is symmetric), so with
// equal team sizes the deltas across all players sum to zero.
func computeEloDeltas(teams []Team, winnerLabel string) map[string]int {
	if len(teams) != 2 {
		return nil
	}

	ratingA := meanElo(teams[0].Members)
	ratingB := meanElo(teams[1].Members)
	expectedA := eloExpected(ratingA, ratingB)
	expectedB := 1 - expectedA

	var scoreA, scoreB float64
	switch winnerLabel {
	case teams[0].Label:
		scoreA, scoreB = 1, 0
	case teams[1].Label:
		scoreA, scoreB = 0, 1
	default: // draw (winnerLabel == "")
		scoreA, scoreB = 0.5, 0.5
	}

	deltaA := int(math.Round(eloK * (scoreA - expectedA)))
	deltaB := int(math.Round(eloK * (scoreB - expectedB)))

	deltas := make(map[string]int, len(teams[0].Members)+len(teams[1].Members))
	for _, m := range teams[0].Members {
		deltas[m.UserID] = deltaA
	}
	for _, m := range teams[1].Members {
		deltas[m.UserID] = deltaB
	}
	return deltas
}
