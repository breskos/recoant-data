package gen

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/breskos/recoant-data/synthetic"
)

// Every generator is deterministic: two runs produce identical data. The
// large benchmark set runs once (in -short mode not at all), only checked
// for its size.
func TestDeterministic(t *testing.T) {
	for i, g := range All() {
		if i == len(All())-1 && testing.Short() {
			continue // benchmark/large, the last generator
		}
		a := g()
		t.Run(a.Meta.Name, func(t *testing.T) {
			if a.Meta.Name == "benchmark/large" {
				if len(a.Signals) < 3_000_000 || !a.Gzip {
					t.Errorf("%d signals, gzip %v", len(a.Signals), a.Gzip)
				}
				return
			}
			b := g()
			if !reflect.DeepEqual(a, b) {
				t.Error("two runs differ")
			}
			if len(a.Signals) == 0 && !a.Meta.Invalid {
				t.Error("no signals")
			}
		})
	}
}

func TestKinds(t *testing.T) {
	sigs := []synthetic.Signal{
		{SubjectKind: "user", ObjectKind: "item", Type: "share"},
		{SubjectKind: "session", ObjectKind: "item", Type: "purchase"},
		{SubjectKind: "user", ObjectKind: "user", Type: "visit"},
		{SubjectKind: "user", ObjectKind: "item", Type: "follow"},
	}
	subjects, objects, types := Kinds(sigs)
	if !slices.Equal(subjects, []string{"session", "user"}) || !slices.Equal(objects, []string{"item", "user"}) ||
		!slices.Equal(types, []string{"visit", "purchase", "follow", "share"}) {
		t.Errorf("Kinds = %v %v %v", subjects, objects, types)
	}
}

func TestDuring(t *testing.T) {
	rng := newRand(1)
	if _, ok := during(rng, Start, Start.Add(day)); ok {
		t.Error("window shorter than two days")
	}
	got, ok := during(rng, Start, Start.Add(10*day))
	if !ok || got.Before(Start.Add(day)) || !got.Before(Start.Add(10*day)) {
		t.Errorf("during = %v %v", got, ok)
	}
}

func TestDrawWeighted(t *testing.T) {
	rng := newRand(1)
	if _, ok := drawWeighted(rng, []float64{0, 0}); ok {
		t.Error("all weights zero")
	}
	if i, ok := drawWeighted(rng, []float64{0, 3}); !ok || i != 1 {
		t.Errorf("drawWeighted = %d %v", i, ok)
	}
}

func TestMustTime(t *testing.T) {
	if !mustTime("2026-01-01T00:00:00Z").Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Error("mustTime")
	}
	defer func() {
		if recover() == nil {
			t.Error("no panic on a bad time")
		}
	}()
	mustTime("yesterday")
}
