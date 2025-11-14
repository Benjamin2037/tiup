package precheck

import "testing"

func TestSummary(t *testing.T) {
	r := &RiskReport{
		High:   []RiskItem{{}, {}},
		Medium: []RiskItem{{}},
		Low:    nil,
	}
	s := r.Summary()
	if s.High != 2 || s.Medium != 1 || s.Low != 0 || s.Total != 3 {
		t.Fatalf("unexpected summary: %+v", s)
	}
}
