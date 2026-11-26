package gen

import (
	"fmt"
	"strconv"
	"time"

	"github.com/breskos/recoant-data/synthetic"
)

// ZipfPopularity has no personal taste at all: every user draws items from
// the same Zipf distribution.
func ZipfPopularity() *Set {
	const users, items, per, s = 2000, 1000, 25, 1.1
	rng := newRand(1)
	perm := rng.Perm(items) // rank → item index
	z := newZipf(rng, s, items)
	e := &emitter{rng: rng}
	for u := range users {
		for range per {
			item := perm[z.draw()]
			e.interact(id("u", u), "item", id("i", item), randTime(rng, Start, 30*day), defaultFunnel, "")
		}
	}
	sortByTime(e.out)
	shares := zipfShares(s, items)
	tbl := Table{Name: "items.csv", Header: []string{"item_id", "rank", "share"}}
	for rank, item := range perm {
		tbl.Rows = append(tbl.Rows, []string{id("i", item), strconv.Itoa(rank + 1), f(shares[rank], 6)})
	}
	return &Set{
		Meta: synthetic.Dataset{
			Name: "basic/zipf-popularity", Folder: "basic", Ordered: true,
			Description: fmt.Sprintf("%d users draw %d interactions each from the same Zipf distribution (exponent %.1f) over %d items, over 30 days. Each interaction walks down the funnel visit → click → dwell → cart → purchase. There is no personal taste.", users, per, s, items),
			Expected:    "Popularity is the best possible model here. Personalized models should not beat it by more than noise; if they do, they overfit. items.csv holds the true rank and expected share of each item.",
		},
		Signals: e.out,
		Tables:  []Table{tbl},
	}
}

// tasteClusters generates users and items in clusters; users mostly
// interact with items of their cluster.
func tasteClusters(seed uint64, users, items, clusters, per int, loyalty, zipfS float64, span time.Duration) ([]synthetic.Signal, Table, Table) {
	rng := newRand(seed)
	members := clusterMembers(rng, items, clusters)
	zs := make([]zipfSampler, clusters)
	for c := range clusters {
		zs[c] = newZipf(rng, zipfS, len(members[c]))
	}
	e := &emitter{rng: rng}
	for u := range users {
		for range per {
			c := u % clusters
			if rng.Float64() >= loyalty {
				c = rng.IntN(clusters)
			}
			item := members[c][zs[c].draw()]
			e.interact(id("u", u), "item", id("i", item), randTime(rng, Start, span), defaultFunnel, "")
		}
	}
	sortByTime(e.out)
	ut := Table{Name: "users.csv", Header: []string{"user_id", "cluster"}}
	for u := range users {
		ut.Rows = append(ut.Rows, []string{id("u", u), strconv.Itoa(u % clusters)})
	}
	it := Table{Name: "items.csv", Header: []string{"item_id", "cluster", "rank_in_cluster"}}
	rank := make([]int, items)
	for _, m := range members {
		for r, i := range m {
			rank[i] = r + 1
		}
	}
	for i := range items {
		it.Rows = append(it.Rows, []string{id("i", i), strconv.Itoa(i % clusters), strconv.Itoa(rank[i])})
	}
	return e.out, ut, it
}

// TasteClusters is the standard set for personalized models.
func TasteClusters() *Set {
	const users, items, clusters, per, loyalty = 5000, 2000, 10, 20, 0.85
	sigs, ut, it := tasteClusters(2, users, items, clusters, per, loyalty, 1.0, 30*day)
	return &Set{
		Meta: synthetic.Dataset{
			Name: "basic/taste-clusters", Folder: "basic", Ordered: true,
			Description: fmt.Sprintf("%d users and %d items in %d taste clusters (user u and item i belong to cluster index %% %d). Each user has %d interactions over 30 days; %.0f %% stay in the user's cluster, the rest go to a random cluster. Within a cluster, item popularity follows a Zipf law (exponent 1.0). Funnel signals visit → click → dwell → cart → purchase.", users, items, clusters, clusters, per, loyalty*100),
			Expected:    "Personalized models (item-kNN, user-kNN, matrix factorization) should clearly beat popularity, which ignores the clusters. users.csv and items.csv hold the true clusters; similar items or users should mostly share a cluster.",
		},
		Signals: sigs,
		Tables:  []Table{ut, it},
	}
}

// Large is the benchmark set.
func Large() *Set {
	const users, items, clusters, per, loyalty = 200000, 50000, 100, 8, 0.85
	sigs, ut, it := tasteClusters(14, users, items, clusters, per, loyalty, 1.0, 90*day)
	return &Set{
		Meta: synthetic.Dataset{
			Name: "benchmark/large", Folder: "benchmark", Ordered: true,
			Description: fmt.Sprintf("Like basic/taste-clusters at scale: %d users, %d items, %d clusters, %d interactions per user over 90 days. Gzip-compressed.", users, items, clusters, per),
			Expected:    "For throughput and memory measurements of ingestion, training and recommendation. Quality expectations as for basic/taste-clusters.",
		},
		Signals: sigs,
		Tables:  []Table{ut, it},
		Gzip:    true,
	}
}
