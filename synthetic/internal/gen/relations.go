package gen

import (
	"fmt"
	"strconv"
	"time"

	"github.com/breskos/recoant-data/synthetic"
)

// SessionSequences holds anonymous sessions that follow item chains.
func SessionSequences() *Set {
	const items, chains, sessions = 2000, 250, 20000
	rng := newRand(9)
	perm := rng.Perm(items)
	var chain [][]int
	next := 0
	for range chains {
		n := 4 + rng.IntN(7)
		chain = append(chain, perm[next:next+n])
		next += n
	}
	e := &emitter{rng: rng}
	for s := range sessions {
		sid := fmt.Sprintf("s%06d", s)
		c := chain[rng.IntN(chains)]
		pos := rng.IntN(len(c) - 2)
		t := randTime(rng, Start, 30*day)
		for range 3 + rng.IntN(4) {
			var item int
			if rng.Float64() < 0.15 || pos >= len(c) {
				item = rng.IntN(items)
			} else {
				item = c[pos]
				pos++
			}
			oid := id("i", item)
			e.emit("session", sid, "item", oid, "visit", 1, t, sid)
			if rng.Float64() < 0.6 {
				e.emit("session", sid, "item", oid, "click", 1, t.Add(3*time.Second), sid)
			}
			t = t.Add(time.Duration(20+rng.IntN(120)) * time.Second)
		}
	}
	sortByTime(e.out)
	ct := Table{Name: "chains.csv", Header: []string{"chain", "position", "item_id"}}
	for ci, c := range chain {
		for p, item := range c {
			ct.Rows = append(ct.Rows, []string{strconv.Itoa(ci), strconv.Itoa(p), id("i", item)})
		}
	}
	return &Set{
		Meta: synthetic.Dataset{
			Name: "session/sequences", Folder: "session", Ordered: true,
			Description: fmt.Sprintf("%d anonymous sessions (subject kind \"session\") over %d items, 30 days. %d disjoint chains of 4–10 items; a session starts somewhere in a chain and follows it for 3–6 steps, with a 15 %% chance per step of jumping to a random item. Every step is a visit, 60 %% also a click.", sessions, items, chains),
			Expected:    "Next-item and session-based models (sequential co-occurrence, session-kNN) should predict the next chain item (chains.csv) for most steps; plain co-occurrence ignoring order does worse, popularity is close to useless.",
		},
		Signals: e.out,
		Tables:  []Table{ct},
	}
}

// Social adds a homophilous follow graph to taste clusters.
func Social() *Set {
	const users, items, clusters, per = 4000, 2000, 8, 15
	sigs, ut, it := tasteClusters(10, users, items, clusters, per, 0.8, 1.0, 30*day)
	rng := newRand(110)
	e := &emitter{rng: rng, out: sigs}
	byCluster := make([][]int, clusters)
	for u := range users {
		byCluster[u%clusters] = append(byCluster[u%clusters], u)
	}
	for u := range users {
		seen := map[int]bool{u: true}
		for range 5 + rng.IntN(26) {
			v := rng.IntN(users)
			if rng.Float64() < 0.8 {
				m := byCluster[u%clusters]
				v = m[rng.IntN(len(m))]
			}
			if seen[v] {
				continue
			}
			seen[v] = true
			e.emit("user", id("u", u), "user", id("u", v), "follow", 1, randTime(rng, Start, 30*day), "")
		}
	}
	sortByTime(e.out)
	return &Set{
		Meta: synthetic.Dataset{
			Name: "relations/social", Folder: "relations", Ordered: true,
			Description: fmt.Sprintf("%d users and %d items in %d taste clusters (80 %% loyalty, %d item interactions per user) plus a follow graph: every user follows 5–30 others (signal type \"follow\", object kind \"user\"), 80 %% of them in the same cluster.", users, items, clusters, per),
			Expected:    "User-to-user recommendations (who to follow) should mostly suggest users of the same cluster (users.csv), both from the follow graph and from similar item interactions. Combining both should beat either alone. Item recommendations can use the follow graph as side information.",
		},
		Signals: e.out,
		Tables:  []Table{ut, it},
	}
}

// CatalogRelations has entity–entity relations (category, brand,
// accessory) and users with brand preferences.
func CatalogRelations() *Set {
	const users, products, categories, brands, per = 5000, 3000, 30, 100, 20
	rng := newRand(11)
	cat, brand := make([]int, products), make([]int, products)
	byBrand := make([][]int, brands)
	rt := Table{Name: "relations.csv", Header: []string{"subject_kind", "subject_id", "relation", "object_kind", "object_id"}}
	for p := range products {
		cat[p], brand[p] = rng.IntN(categories), rng.IntN(brands)
		byBrand[brand[p]] = append(byBrand[brand[p]], p)
		rt.Rows = append(rt.Rows,
			[]string{"product", id("p", p), "in_category", "category", id("c", cat[p])},
			[]string{"product", id("p", p), "by_brand", "brand", id("b", brand[p])},
		)
	}
	for p := range products {
		if rng.Float64() < 0.1 {
			// Accessory of another product of the same brand.
			m := byBrand[brand[p]]
			if q := m[rng.IntN(len(m))]; q != p {
				rt.Rows = append(rt.Rows, []string{"product", id("p", p), "accessory_of", "product", id("p", q)})
			}
		}
	}
	e := &emitter{rng: rng}
	ut := Table{Name: "users.csv", Header: []string{"user_id", "brand_1", "brand_2", "brand_3"}}
	for u := range users {
		fav := rng.Perm(brands)[:3]
		ut.Rows = append(ut.Rows, []string{id("u", u), id("b", fav[0]), id("b", fav[1]), id("b", fav[2])})
		for range per {
			p := rng.IntN(products)
			if rng.Float64() < 0.7 {
				m := byBrand[fav[rng.IntN(3)]]
				p = m[rng.IntN(len(m))]
			}
			e.interact(id("u", u), "product", id("p", p), randTime(rng, Start, 30*day), defaultFunnel, "")
		}
	}
	sortByTime(e.out)
	return &Set{
		Meta: synthetic.Dataset{
			Name: "relations/catalog", Folder: "relations", Ordered: true,
			Description: fmt.Sprintf("%d products with relations in relations.csv: every product is in one of %d categories and by one of %d brands; about 10 %% are accessories of another product of the same brand. %d users have 3 favorite brands; 70 %% of their %d interactions go to products of these brands.", products, categories, brands, users, per),
			Expected:    "Graph models (random walks over user–product–brand) should find the brand preference (users.csv) and recommend unseen products of the favorite brands, also for products with few interactions. Category relations carry no preference signal and should not help much.",
		},
		Signals: e.out,
		Tables:  []Table{ut, rt},
	}
}
