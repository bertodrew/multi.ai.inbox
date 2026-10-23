package core

import "testing"

// The growth plan sits between builder and team on every limit (doc 11, 5.2).
func TestGrowthPlanBetweenBuilderAndTeam(t *testing.T) {
	b, g, tm := Plans["builder"], Plans["growth"], Plans["team"]
	if g.Name != "growth" || g.Footer {
		t.Fatalf("growth plan: %+v", g)
	}
	for name, v := range map[string][3]int{
		"inboxes":         {b.Inboxes, g.Inboxes, tm.Inboxes},
		"domains":         {b.Domains, g.Domains, tm.Domains},
		"sends_per_day":   {b.SendsPerDay, g.SendsPerDay, tm.SendsPerDay},
		"sends_per_month": {b.SendsPerMonth, g.SendsPerMonth, tm.SendsPerMonth},
	} {
		if !(v[0] < v[1] && v[1] < v[2]) {
			t.Errorf("%s: builder %d, growth %d, team %d", name, v[0], v[1], v[2])
		}
	}
	for _, p := range PaidPlans {
		if _, ok := Plans[p]; !ok {
			t.Errorf("paid plan %q has no limits", p)
		}
	}
}
