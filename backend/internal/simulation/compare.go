package simulation

import (
	"fmt"
	"math"
)

// IndicatorDelta aligns one indicator of two results.
type IndicatorDelta struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Unit  string `json:"unit"`
	// A and B scenario values and their own reference/change.
	AReference float64 `json:"a_reference"`
	AScenario  float64 `json:"a_scenario"`
	AChange    float64 `json:"a_change"`
	BReference float64 `json:"b_reference"`
	BScenario  float64 `json:"b_scenario"`
	BChange    float64 `json:"b_change"`
	// Delta = B.scenario - A.scenario (what choosing B instead of A would change).
	Delta float64 `json:"delta"`
	// ReferenceDiffers is true when the two results used different baselines (different region or
	// reference period): then Delta compares scenarios against different references.
	ReferenceDiffers bool `json:"reference_differs"`
}

// Comparison is the side-by-side comparison of two results.
type Comparison struct {
	AID          string           `json:"a_id"`
	BID          string           `json:"b_id"`
	AName        string           `json:"a_name"`
	BName        string           `json:"b_name"`
	AParams      Params           `json:"a_params"`
	BParams      Params           `json:"b_params"`
	SameBaseline bool             `json:"same_baseline"`
	Deltas       []IndicatorDelta `json:"deltas"`
	// OnlyInA / OnlyInB list indicator keys that could not be aligned.
	OnlyInA []string `json:"only_in_a"`
	OnlyInB []string `json:"only_in_b"`
}

// Compare aligns the indicators of two successful results by key and returns their deltas. It
// refuses results without numbers (insufficient_data / error).
func Compare(a, b Result) (Comparison, error) {
	if a.Status != StatusOK || b.Status != StatusOK {
		return Comparison{}, fmt.Errorf("%w: only results with status ok can be compared", ErrInvalid)
	}
	same := a.Model == b.Model && a.Params.RegionID == b.Params.RegionID &&
		a.Params.ReferenceFrom == b.Params.ReferenceFrom && a.Params.ReferenceTo == b.Params.ReferenceTo &&
		a.Params.Scale == b.Params.Scale
	c := Comparison{AID: a.ID, BID: b.ID, AName: a.Name, BName: b.Name, AParams: a.Params, BParams: b.Params,
		SameBaseline: same, Deltas: []IndicatorDelta{}, OnlyInA: []string{}, OnlyInB: []string{}}
	bi := make(map[string]Indicator, len(b.Indicators))
	for _, i := range b.Indicators {
		bi[i.Key] = i
	}
	used := map[string]bool{}
	for _, ia := range a.Indicators {
		ib, ok := bi[ia.Key]
		if !ok {
			c.OnlyInA = append(c.OnlyInA, ia.Key)
			continue
		}
		used[ia.Key] = true
		c.Deltas = append(c.Deltas, IndicatorDelta{
			Key: ia.Key, Label: ia.Label, Unit: ia.Unit,
			AReference: ia.Reference, AScenario: ia.Scenario, AChange: ia.Change,
			BReference: ib.Reference, BScenario: ib.Scenario, BChange: ib.Change,
			Delta:            math.Round((ib.Scenario-ia.Scenario)*100) / 100,
			ReferenceDiffers: ia.Reference != ib.Reference,
		})
	}
	for _, i := range b.Indicators {
		if !used[i.Key] {
			c.OnlyInB = append(c.OnlyInB, i.Key)
		}
	}
	return c, nil
}
