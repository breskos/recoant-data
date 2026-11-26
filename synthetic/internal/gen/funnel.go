package gen

import (
	"fmt"
	"math"
	"time"

	"github.com/breskos/recoant-data/synthetic"
)

// Ecommerce models shopping sessions where every product has a hidden
// quality that drives its funnel rates.
func Ecommerce() *Set {
	const users, products, categories = 8000, 3000, 20
	rng := newRand(5)
	quality := make([]float64, products)
	cat := make([]int, products)
	byCat := make([][]int, categories)
	for p := range products {
		quality[p] = rng.Float64()
		cat[p] = rng.IntN(categories)
		byCat[cat[p]] = append(byCat[cat[p]], p)
	}
	rates := func(p int) funnel {
		q := quality[p]
		return funnel{click: 0.15 + 0.5*q, dwellMu: math.Log(15 + 60*q), dwellSig: 0.7, cart: 0.03 + 0.3*q, purchase: 0.25 + 0.5*q}
	}
	e := &emitter{rng: rng}
	ut := Table{Name: "users.csv", Header: []string{"user_id", "category_1", "category_2"}}
	for u := range users {
		c1 := rng.IntN(categories)
		c2 := (c1 + 1 + rng.IntN(categories-1)) % categories
		ut.Rows = append(ut.Rows, []string{id("u", u), id("c", c1), id("c", c2)})
		sessions := 1 + int(rng.ExpFloat64()*2)
		for range min(sessions, 8) {
			sess := e.session()
			t := randTime(rng, Start, 30*day)
			for range 1 + rng.IntN(8) {
				c := c1
				switch x := rng.Float64(); {
				case x < 0.35:
					c = c2
				case x < 0.5:
					c = rng.IntN(categories)
				}
				p := byCat[c][rng.IntN(len(byCat[c]))]
				t = e.interact(id("u", u), "product", id("p", p), t, rates(p), sess)
				t = t.Add(time.Duration(5+rng.IntN(60)) * time.Second)
			}
		}
	}
	sortByTime(e.out)
	pt := Table{Name: "products.csv", Header: []string{"product_id", "category", "quality", "click_rate", "cart_rate", "purchase_rate"}}
	for p := range products {
		r := rates(p)
		pt.Rows = append(pt.Rows, []string{id("p", p), id("c", cat[p]), f(quality[p], 4), f(r.click, 4), f(r.cart, 4), f(r.purchase, 4)})
	}
	return &Set{
		Meta: synthetic.Dataset{
			Name: "funnel/ecommerce", Folder: "funnel", Ordered: true,
			Description: fmt.Sprintf("Shopping sessions of %d users over %d products in %d categories, 30 days. Every user prefers two categories (50 %% and 35 %% of views, 15 %% random). Every product has a hidden quality q ∈ [0, 1] that sets its funnel: click rate 0.15 + 0.5q, median dwell 15 + 60q seconds, cart rate 0.03 + 0.3q after a click, purchase rate 0.25 + 0.5q after the cart. Signals carry session IDs.", users, products, categories),
			Expected:    "Funnel modeling should recover the product rates in products.csv from the signals (click-through, add-to-cart and conversion per product). Ranking by conversion should beat ranking by raw visit counts when the target is purchases. users.csv holds the preferred categories.",
		},
		Signals: e.out,
		Tables:  []Table{ut, pt},
	}
}

// SignalRelevance mixes informative and uninformative signal types before
// purchases, so that the relevance of each type can be learned.
func SignalRelevance() *Set {
	const users, items, intents, browse, hovers = 5000, 2000, 3, 30, 15
	rng := newRand(6)
	e := &emitter{rng: rng}
	it := Table{Name: "intents.csv", Header: []string{"user_id", "item_id"}}
	for u := range users {
		uid := id("u", u)
		intent := rng.Perm(items)[:intents]
		for _, i := range intent {
			it.Rows = append(it.Rows, []string{uid, id("i", i)})
		}
		// Browsing phase: days 0–20.
		for range browse {
			t := randTime(rng, Start, 21*day)
			onIntent := rng.Float64() < 0.3
			i := rng.IntN(items)
			if onIntent {
				i = intent[rng.IntN(intents)]
			}
			oid := id("i", i)
			e.emit("user", uid, "item", oid, "visit", 1, t, "")
			click, dwellMu, share := 0.4, math.Log(12), 0.02
			if onIntent {
				click, dwellMu, share = 0.7, math.Log(120), 0.25
			}
			if rng.Float64() < click {
				t = t.Add(5 * time.Second)
				e.emit("user", uid, "item", oid, "click", 1, t, "")
				dwell := math.Round(math.Exp(dwellMu+0.5*rng.NormFloat64())*10) / 10
				t = t.Add(time.Duration(dwell * float64(time.Second)))
				e.emit("user", uid, "item", oid, "dwell", dwell, t, "")
			}
			if rng.Float64() < share {
				e.emit("user", uid, "item", oid, "share", 1, t.Add(10*time.Second), "")
			}
		}
		for range hovers {
			e.emit("user", uid, "item", id("i", rng.IntN(items)), "hover", 1, randTime(rng, Start, 21*day), "")
		}
		// Purchase phase: days 21–27.
		for _, i := range intent {
			if rng.Float64() < 0.8 {
				e.emit("user", uid, "item", id("i", i), "purchase", 1, randTime(rng, Start.Add(21*day), 7*day), "")
			}
		}
	}
	sortByTime(e.out)
	tt := Table{Name: "signal-types.csv", Header: []string{"type", "informative", "description"}}
	tt.Rows = append(tt.Rows, [][]string{
		{"visit", "weak", "30 % of visits go to intent items, 70 % to random items"},
		{"click", "medium", "click rate 0.7 on intent items, 0.4 otherwise"},
		{"dwell", "strong", "median dwell 120 s on intent items, 12 s otherwise"},
		{"share", "strong", "share rate 0.25 on intent items, 0.02 otherwise"},
		{"hover", "none", "random items, independent of intent"},
		{"purchase", "target", "80 % of intent items are bought in days 21–27"},
	}...)
	return &Set{
		Meta: synthetic.Dataset{
			Name: "funnel/signal-relevance", Folder: "funnel", Ordered: true,
			Description: fmt.Sprintf("%d users, %d items. Every user has %d hidden intent items. Days 0–20 are browsing with signal types of different informativeness (visit, click, dwell, share, and pure-noise hover); in days 21–27 users buy 80 %% of their intent items. signal-types.csv says how each type relates to intent.", users, items, intents),
			Expected:    "Split at day 21 (2026-01-22) and learn signal relevance with purchase as target: the learned weights should rank share and long dwell highest, then click, then visit, with hover near zero. Recommending the items with the strongest weighted browsing signals should predict the purchases (intents.csv) far better than counting all signals equally.",
		},
		Signals: e.out,
		Tables:  []Table{it, tt},
	}
}
