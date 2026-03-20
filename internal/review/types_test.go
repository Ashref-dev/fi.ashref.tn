package review

import "testing"

func TestScoreFindingsAndBlockerCount(t *testing.T) {
	cases := []struct {
		name         string
		findings     []Finding
		wantScore    int
		wantBlockers int
	}{
		{name: "clean", findings: nil, wantScore: 5, wantBlockers: 0},
		{name: "low", findings: []Finding{{Severity: "low"}}, wantScore: 4, wantBlockers: 0},
		{name: "medium", findings: []Finding{{Severity: "medium"}}, wantScore: 3, wantBlockers: 0},
		{name: "high", findings: []Finding{{Severity: "high"}}, wantScore: 2, wantBlockers: 0},
		{name: "blocker", findings: []Finding{{Severity: "blocker"}}, wantScore: 1, wantBlockers: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ScoreFindings(tc.findings); got != tc.wantScore {
				t.Fatalf("expected score %d, got %d", tc.wantScore, got)
			}
			if got := CountBlockers(tc.findings); got != tc.wantBlockers {
				t.Fatalf("expected blocker count %d, got %d", tc.wantBlockers, got)
			}
		})
	}
}
