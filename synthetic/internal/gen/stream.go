package gen

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/breskos/recoant-data/synthetic"
)

// TrendingNews is a news stream: articles are published continuously and
// their attractiveness decays with an article-specific half-life.
func TrendingNews() *Set {
	const (
		users, days, perHour, readsPerHour = 10000, 14, 10, 400
		window                             = 72 * time.Hour
	)
	rng := newRand(7)
	type article struct {
		pub      time.Time
		attract  float64
		halfLife float64 // hours
	}
	var arts []article
	e := &emitter{rng: rng}
	userZ := newZipf(rng, 0.7, users) // some users read much more
	userPerm := rng.Perm(users)
	first := 0 // first article that may still be active
	w := []float64{}
	for h := range days * 24 {
		hour := Start.Add(time.Duration(h) * time.Hour)
		for range perHour {
			arts = append(arts, article{
				pub:      randTime(rng, hour, time.Hour),
				attract:  math.Exp(rng.NormFloat64()),
				halfLife: 2 + rng.Float64()*22,
			})
		}
		for first < len(arts) && hour.Sub(arts[first].pub) > window {
			first++
		}
		for range readsPerHour {
			t := randTime(rng, hour, time.Hour)
			w = w[:0]
			for _, a := range arts[first:] {
				age := t.Sub(a.pub).Hours()
				if age < 0 {
					w = append(w, 0)
					continue
				}
				w = append(w, a.attract*math.Exp2(-age/a.halfLife))
			}
			c := newCategorical(w)
			ai := first + c.draw(rng)
			if t.Before(arts[ai].pub) {
				continue
			}
			uid := id("u", userPerm[userZ.draw()])
			oid := id("a", ai)
			e.emit("user", uid, "article", oid, "visit", 1, t, "")
			if rng.Float64() < 0.6 {
				dwell := math.Round(math.Exp(math.Log(45)+0.8*rng.NormFloat64())*10) / 10
				e.emit("user", uid, "article", oid, "dwell", dwell, t.Add(time.Duration(dwell*float64(time.Second))), "")
			}
		}
	}
	sortByTime(e.out)
	at := Table{Name: "articles.csv", Header: []string{"article_id", "published", "attractiveness", "half_life_hours"}}
	for i, a := range arts {
		at.Rows = append(at.Rows, []string{id("a", i), a.pub.Format(time.RFC3339), f(a.attract, 4), f(a.halfLife, 2)})
	}
	return &Set{
		Meta: synthetic.Dataset{
			Name: "stream/trending-news", Folder: "stream", Ordered: true,
			Description: fmt.Sprintf("A news site over %d days: %d new articles per hour, %d reads per hour from %d users (heavy and light readers). A read picks an article published in the last 72 hours with probability ∝ attractiveness · 2^(−age / half-life); attractiveness is log-normal, the half-life uniform in 2–24 hours. Reads are a visit and, in 60 %% of cases, a dwell signal.", days, perHour, readsPerHour, users),
			Expected:    "Read the stream in order. Trending (popularity with a half-life of a few hours) should clearly beat all-time popularity at predicting the next hour's reads; all-time popularity keeps recommending stale articles. Most articles are new, so this is also a cold-start test. articles.csv holds the true attractiveness and half-life.",
		},
		Signals: e.out,
		Tables:  []Table{at},
	}
}

// PreferenceDrift switches every user to another cluster halfway through.
func PreferenceDrift() *Set {
	const users, items, clusters, per, days = 3000, 1500, 10, 30, 60
	rng := newRand(8)
	members := clusterMembers(rng, items, clusters)
	zs := make([]zipfSampler, clusters)
	for c := range clusters {
		zs[c] = newZipf(rng, 1.0, len(members[c]))
	}
	switchAt := Start.Add(days / 2 * day)
	e := &emitter{rng: rng}
	ut := Table{Name: "users.csv", Header: []string{"user_id", "cluster_before", "cluster_after"}}
	for u := range users {
		before := u % clusters
		after := (before + 1 + rng.IntN(clusters-1)) % clusters
		ut.Rows = append(ut.Rows, []string{id("u", u), strconv.Itoa(before), strconv.Itoa(after)})
		for range per {
			t := randTime(rng, Start, days*day)
			c := before
			if !t.Before(switchAt) {
				c = after
			}
			if rng.Float64() >= 0.85 {
				c = rng.IntN(clusters)
			}
			e.interact(id("u", u), "item", id("i", members[c][zs[c].draw()]), t, defaultFunnel, "")
		}
	}
	sortByTime(e.out)
	it := Table{Name: "items.csv", Header: []string{"item_id", "cluster"}}
	for i := range items {
		it.Rows = append(it.Rows, []string{id("i", i), strconv.Itoa(i % clusters)})
	}
	return &Set{
		Meta: synthetic.Dataset{
			Name: "stream/preference-drift", Folder: "stream", Ordered: true,
			Description: fmt.Sprintf("Like basic/taste-clusters (%d users, %d items, %d clusters, 85 %% loyalty) over %d days, but on day %d (2026-01-31) every user switches to another cluster.", users, items, clusters, days, days/2),
			Expected:    "Read in order and evaluate test-then-train. After the switch, models with time decay (short half-life) or drift detection should recover quickly; models without decay keep recommending the old cluster. users.csv holds both clusters of every user.",
		},
		Signals: e.out,
		Tables:  []Table{ut, it},
	}
}
