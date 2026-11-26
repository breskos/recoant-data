// Package gen generates the synthetic data sets. Every generator is
// deterministic: the same seed produces identical files.
package gen

import (
	"cmp"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"time"

	"github.com/breskos/recoant-data/synthetic"
)

// Start is the time of the first signal of every data set.
var Start = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

const day = 24 * time.Hour

// Table is a CSV table of a data set.
type Table struct {
	Name   string
	Header []string
	Rows   [][]string
}

// Set is a generated data set.
type Set struct {
	Meta    synthetic.Dataset // Signals and Tables are filled in when writing
	Signals []synthetic.Signal
	Tables  []Table
	Gzip    bool
}

// All returns the generators in catalog order.
func All() []func() *Set {
	return []func() *Set{
		ZipfPopularity,
		TasteClusters,
		ExplicitRatings,
		ImplicitFeedback,
		Ecommerce,
		SignalRelevance,
		TrendingNews,
		PreferenceDrift,
		SessionSequences,
		Social,
		CatalogRelations,
		People,
		Shop,
		EdgeCases,
		Invalid,
		Large,
	}
}

func newRand(seed uint64) *rand.Rand { return rand.New(rand.NewPCG(seed, seed*0x9e3779b97f4a7c15+1)) }

func id(prefix string, i int) string { return prefix + strconv.Itoa(i) }

func f(v float64, prec int) string { return strconv.FormatFloat(v, 'f', prec, 64) }

// sortByTime sorts signals by time, keeping the order of equal times.
func sortByTime(s []synthetic.Signal) {
	slices.SortStableFunc(s, func(a, b synthetic.Signal) int { return a.Time.Compare(b.Time) })
}

// funnel describes how deep an interaction goes.
type funnel struct {
	click    float64 // P(click | visit)
	dwellMu  float64 // log-normal dwell time after a click: median e^mu seconds
	dwellSig float64
	cart     float64 // P(cart | click)
	purchase float64 // P(purchase | cart)
}

var defaultFunnel = funnel{click: 0.5, dwellMu: math.Log(30), dwellSig: 0.8, cart: 0.25, purchase: 0.5}

// emitter appends signals of one subject–object interaction with small
// time gaps between the funnel stages.
type emitter struct {
	rng  *rand.Rand
	out  []synthetic.Signal
	sess int
}

func (e *emitter) emit(sk, sid, ok, oid, typ string, v float64, t time.Time, session string) {
	e.out = append(e.out, synthetic.Signal{
		Time: t, SubjectKind: sk, SubjectID: sid, ObjectKind: ok, ObjectID: oid,
		Type: typ, Value: v, Session: session,
	})
}

func (e *emitter) session() string {
	e.sess++
	return fmt.Sprintf("s%07d", e.sess)
}

// interact emits visit → click → dwell → cart → purchase with the
// probabilities of fn and returns the time after the last signal.
func (e *emitter) interact(sid, ok, oid string, t time.Time, fn funnel, session string) time.Time {
	gap := func() time.Duration { return time.Duration(1+e.rng.IntN(30)) * time.Second }
	e.emit("user", sid, ok, oid, "visit", 1, t, session)
	t = t.Add(gap())
	if e.rng.Float64() >= fn.click {
		return t
	}
	e.emit("user", sid, ok, oid, "click", 1, t, session)
	dwell := math.Round(math.Exp(fn.dwellMu+fn.dwellSig*e.rng.NormFloat64())*10) / 10
	t = t.Add(time.Duration(dwell * float64(time.Second)))
	e.emit("user", sid, ok, oid, "dwell", dwell, t, session)
	t = t.Add(gap())
	if e.rng.Float64() >= fn.cart {
		return t
	}
	e.emit("user", sid, ok, oid, "cart", 1, t, session)
	t = t.Add(gap())
	if e.rng.Float64() >= fn.purchase {
		return t
	}
	e.emit("user", sid, ok, oid, "purchase", 1, t, session)
	return t.Add(gap())
}

// randTime returns a uniform time in [from, from+span), rounded to seconds.
func randTime(rng *rand.Rand, from time.Time, span time.Duration) time.Time {
	return from.Add(time.Duration(rng.Int64N(int64(span/time.Second))) * time.Second)
}

// zipfSampler draws ranks 0..n-1 with P(k) ∝ (k+1)^-s, for any s ≥ 0.
type zipfSampler struct {
	rng *rand.Rand
	c   categorical
}

func newZipf(rng *rand.Rand, s float64, n int) zipfSampler {
	return zipfSampler{rng: rng, c: newCategorical(zipfShares(s, n))}
}

func (z zipfSampler) draw() int { return z.c.draw(z.rng) }

// zipfShares returns the expected share of each rank k of n with exponent
// s: P(k) ∝ (1+k)^-s.
func zipfShares(s float64, n int) []float64 {
	p := make([]float64, n)
	var sum float64
	for k := range n {
		p[k] = math.Pow(1+float64(k), -s)
		sum += p[k]
	}
	for k := range p {
		p[k] /= sum
	}
	return p
}

// categorical samples indexes with given non-negative weights.
type categorical struct{ cum []float64 }

func newCategorical(w []float64) categorical {
	cum := make([]float64, len(w))
	var s float64
	for i, x := range w {
		s += x
		cum[i] = s
	}
	return categorical{cum}
}

func (c categorical) draw(rng *rand.Rand) int {
	x := rng.Float64() * c.cum[len(c.cum)-1]
	i, _ := slices.BinarySearch(c.cum, x)
	return min(i, len(c.cum)-1)
}

// clusterMembers returns, per cluster, the indexes i with i % k == c in a
// random order, so that rank and ID are unrelated.
func clusterMembers(rng *rand.Rand, n, k int) [][]int {
	out := make([][]int, k)
	for i := range n {
		out[i%k] = append(out[i%k], i)
	}
	for _, m := range out {
		rng.Shuffle(len(m), func(a, b int) { m[a], m[b] = m[b], m[a] })
	}
	return out
}

// Kinds returns the subject kinds, object kinds and signal types of signals.
func Kinds(s []synthetic.Signal) (subjects, objects, types []string) {
	add := func(list []string, v string) []string {
		if !slices.Contains(list, v) {
			list = append(list, v)
		}
		return list
	}
	for _, x := range s {
		subjects = add(subjects, x.SubjectKind)
		objects = add(objects, x.ObjectKind)
		types = add(types, x.Type)
	}
	slices.Sort(subjects)
	slices.Sort(objects)
	order := map[string]int{"visit": 1, "click": 2, "dwell": 3, "cart": 4, "purchase": 5}
	slices.SortFunc(types, func(a, b string) int {
		oa, ob := cmp.Or(order[a], 99), cmp.Or(order[b], 99)
		if oa != ob {
			return oa - ob
		}
		return cmp.Compare(a, b)
	})
	return subjects, objects, types
}
