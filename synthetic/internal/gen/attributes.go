package gen

import (
	"cmp"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/breskos/recoant-data/synthetic"
)

// AttributeHeader is the header of the attribute event tables.
var AttributeHeader = []string{"time", "kind", "id", "op", "attribute", "value"}

// SchemaHeader is the header of the attribute schema tables.
var SchemaHeader = []string{"kind", "attribute", "type", "sensitive"}

// attrEvent is one row of an attributes.csv table.
type attrEvent struct {
	t                         time.Time
	kind, id, op, attr, value string
}

func setAttr(t time.Time, kind, id, attr, value string) attrEvent {
	return attrEvent{t: t, kind: kind, id: id, op: "set", attr: attr, value: value}
}

// attrTable sorts events by time (stable) and returns the table.
func attrTable(ev []attrEvent) Table {
	slices.SortStableFunc(ev, func(a, b attrEvent) int { return a.t.Compare(b.t) })
	t := Table{Name: "attributes.csv", Header: AttributeHeader}
	for _, e := range ev {
		t.Rows = append(t.Rows, []string{e.t.Format(time.RFC3339), e.kind, e.id, e.op, e.attr, e.value})
	}
	return t
}

func schemaTable(rows ...[]string) Table {
	return Table{Name: "schema.csv", Header: SchemaHeader, Rows: rows}
}

func num(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func geo(p point) string { return f(p.lat, 4) + "," + f(p.lon, 4) }

func setValue(v []string) string { return strings.Join(v, "|") }

type point struct{ lat, lon float64 }

// cities are far apart (at least 150 km): a distance limit of 50 km means
// "same city".
var cities = []struct {
	name string
	at   point
}{
	{"berlin", point{52.5200, 13.4050}},
	{"hamburg", point{53.5511, 9.9937}},
	{"munich", point{48.1372, 11.5755}},
	{"cologne", point{50.9375, 6.9603}},
	{"frankfurt", point{50.1109, 8.6821}},
}

// near returns a point within about 15 km of city c.
func near(rng *rand.Rand, c int) point {
	at := cities[c].at
	r := 15 * math.Sqrt(rng.Float64()) // km, uniform in the disc
	a := 2 * math.Pi * rng.Float64()
	return point{
		lat: math.Round((at.lat+r*math.Cos(a)/111.2)*1e4) / 1e4,
		lon: math.Round((at.lon+r*math.Sin(a)/(111.2*math.Cos(at.lat*math.Pi/180)))*1e4) / 1e4,
	}
}

var interestNames = []string{
	"hiking", "jazz", "cooking", "chess", "running", "cinema", "travel", "yoga", "photography", "climbing",
	"techno", "reading", "gaming", "theatre", "cycling", "painting", "football", "dancing", "wine", "gardening",
}

func pick(rng *rand.Rand, from []string, n int) []string {
	out := make([]string, 0, n)
	for _, i := range rng.Perm(len(from))[:n] {
		out = append(out, from[i])
	}
	slices.Sort(out)
	return out
}

func overlap(a, b []string) int {
	n := 0
	for _, x := range a {
		if slices.Contains(b, x) {
			n++
		}
	}
	return n
}

func sigmoid(x float64) float64 { return 1 / (1 + math.Exp(-x)) }

// person is the state of a user in People.
type person struct {
	age, minAge, maxAge int
	city                int
	interests           []string
	attract             float64
	signup, deleted     time.Time // deleted is zero for active users
}

func (p *person) alive(t time.Time) bool {
	return !t.Before(p.signup) && (p.deleted.IsZero() || t.Before(p.deleted))
}

func (p *person) accepts(age int) bool { return age >= p.minAge && age <= p.maxAge }

// during returns a random time at least a day after from and before to,
// or false if the two are less than two days apart.
func during(rng *rand.Rand, from, to time.Time) (time.Time, bool) {
	if to.Sub(from) < 2*day {
		return time.Time{}, false
	}
	return randTime(rng, from.Add(day), to.Sub(from)-day), true
}

// drawWeighted draws an index with probability ∝ w, or false if all
// weights are zero.
func drawWeighted(rng *rand.Rand, w []float64) (int, bool) {
	if slices.Max(w) == 0 {
		return 0, false
	}
	return newCategorical(w).draw(rng), true
}

// timeline step: either a view of user u or a scheduled change.
type step struct {
	t      time.Time
	u      int
	change func()
}

// People is a people-discovery app with attributes that matter: age,
// preferred age range, location and interests, plus profile updates,
// moves and account deletions.
func People() *Set {
	const users, span = 2500, 120 * day
	rng := newRand(14)
	end := Start.Add(span)
	ps := make([]*person, users)
	var ev []attrEvent
	ut := Table{Name: "users.csv", Header: []string{"user_id", "attractiveness", "signup", "city_at_signup", "deleted"}}
	var steps []step
	for u := range users {
		age := 20 + rng.IntN(36)
		p := &person{
			age: age, minAge: max(18, age-3-rng.IntN(8)), maxAge: age + 3 + rng.IntN(8),
			city: rng.IntN(len(cities)), interests: pick(rng, interestNames, 3+rng.IntN(3)),
			attract: math.Round(rng.NormFloat64()*1000) / 1000,
			signup:  randTime(rng, Start, 30*day),
		}
		if rng.Float64() < 0.06 {
			p.deleted = randTime(rng, Start.Add(30*day), span-30*day)
		}
		ps[u] = p
		uid := id("u", u)
		gender := []string{"f", "m", "x"}[rng.IntN(3)]
		ev = append(ev,
			setAttr(p.signup, "user", uid, "age", strconv.Itoa(p.age)),
			setAttr(p.signup, "user", uid, "min_age", strconv.Itoa(p.minAge)),
			setAttr(p.signup, "user", uid, "max_age", strconv.Itoa(p.maxAge)),
			setAttr(p.signup, "user", uid, "gender", gender),
			setAttr(p.signup, "user", uid, "location", geo(near(rng, p.city))),
			setAttr(p.signup, "user", uid, "interests", setValue(p.interests)),
		)
		deleted := ""
		if !p.deleted.IsZero() {
			deleted = p.deleted.Format(time.RFC3339)
			ev = append(ev, attrEvent{t: p.deleted, kind: "user", id: uid, op: "delete"})
		}
		ut.Rows = append(ut.Rows, []string{uid, f(p.attract, 3), p.signup.Format(time.RFC3339), cities[p.city].name, deleted})

		// Profile changes during the period, while the account exists.
		life := func() (time.Time, bool) {
			last := end
			if !p.deleted.IsZero() {
				last = p.deleted
			}
			return during(rng, p.signup, last)
		}
		if x := rng.Float64(); x < 0.2 {
			if t, ok := life(); ok {
				lo := max(18, p.minAge-5+rng.IntN(11))
				hi := max(lo+4, p.maxAge-5+rng.IntN(11))
				ev = append(ev, setAttr(t, "user", uid, "min_age", strconv.Itoa(lo)), setAttr(t, "user", uid, "max_age", strconv.Itoa(hi)))
				steps = append(steps, step{t: t, change: func() { p.minAge, p.maxAge = lo, hi }})
			}
		}
		if rng.Float64() < 0.08 {
			if t, ok := life(); ok {
				c := (p.city + 1 + rng.IntN(len(cities)-1)) % len(cities)
				ev = append(ev, setAttr(t, "user", uid, "location", geo(near(rng, c))))
				steps = append(steps, step{t: t, change: func() { p.city = c }})
			}
		}
		switch x := rng.Float64(); {
		case x < 0.1:
			if t, ok := life(); ok {
				in := pick(rng, interestNames, len(p.interests))
				ev = append(ev, setAttr(t, "user", uid, "interests", setValue(in)))
				steps = append(steps, step{t: t, change: func() { p.interests = in }})
			}
		case x < 0.13:
			if t, ok := life(); ok {
				ev = append(ev, attrEvent{t: t, kind: "user", id: uid, op: "delete", attr: "interests"})
				steps = append(steps, step{t: t, change: func() { p.interests = nil }})
			}
		}
		for range 20 + rng.IntN(31) {
			steps = append(steps, step{t: randTime(rng, p.signup, end.Sub(p.signup)), u: u})
		}
	}
	slices.SortStableFunc(steps, func(a, b step) int { return a.t.Compare(b.t) })

	e := &emitter{rng: rng}
	viewed := make([]map[int]bool, users)
	liked := make([]map[int]bool, users)
	for u := range users {
		viewed[u], liked[u] = map[int]bool{}, map[int]bool{}
	}
	for _, s := range steps {
		if s.change != nil {
			s.change()
			continue
		}
		u, t := s.u, s.t
		pu := ps[u]
		if !pu.alive(t) {
			continue
		}
		sameCity := rng.Float64() < 0.85
		v := -1
		for range 30 {
			c := rng.IntN(users)
			pc := ps[c]
			if c == u || viewed[u][c] || !pc.alive(t) || (pc.city == pu.city) != sameCity {
				continue
			}
			v = c
			break
		}
		if v < 0 {
			continue
		}
		viewed[u][v] = true
		pv := ps[v]
		uid, vid := id("u", u), id("u", v)
		e.emit("user", uid, "user", vid, "view", 1, t, "")
		p := sigmoid(-1.2 + 1.2*pv.attract + 0.5*float64(overlap(pu.interests, pv.interests)))
		if !pu.accepts(pv.age) {
			p *= 0.05
		}
		if pv.city != pu.city {
			p *= 0.2
		}
		t = t.Add(time.Duration(3+rng.IntN(40)) * time.Second)
		switch {
		case rng.Float64() < p:
			typ := "like"
			if rng.Float64() < 0.1 {
				typ = "superlike"
			}
			e.emit("user", uid, "user", vid, typ, 1, t, "")
			liked[u][v] = true
			if liked[v][u] && rng.Float64() < 0.6 {
				e.emit("user", uid, "user", vid, "message", 1, t.Add(time.Duration(1+rng.IntN(120))*time.Minute), "")
			}
		case rng.Float64() < 0.7:
			typ := "pass"
			if rng.Float64() < 0.01 {
				typ = "block"
			}
			e.emit("user", uid, "user", vid, typ, 1, t, "")
		}
	}
	sortByTime(e.out)
	return &Set{
		Meta: synthetic.Dataset{
			Name: "attributes/people", Folder: "attributes", Ordered: true,
			Description: fmt.Sprintf("A people-discovery app: %d users sign up in the first 30 days and are active for %d days. Attributes (attributes.csv, schema in schema.csv): age 20–55, preferred age range min_age–max_age, gender (no influence on behavior), location in one of 5 cities (all at least 150 km apart, users within 15 km of the center) and 3–5 of 20 interests; age, gender and location are sensitive. A user views 20–50 profiles, 85 %% in the own city. Like probability σ(−1.2 + 1.2·attractiveness + 0.5·shared interests), ×0.05 outside the user's preferred age range, ×0.2 in another city; 10 %% of likes are superlikes; otherwise 70 %% pass (1 %% of them block). A like that completes a mutual like is followed by a message in 60 %% of cases. During the period 20 %% change their age preferences, 8 %% move to another city, 10 %% replace their interests, 3 %% delete them, and 6 %% delete their account: no signal involves them afterwards.", users, int(span/day)),
			Expected:    "Filters: {\"attribute\": \"age\", \"within\": [\"min_age\", \"max_age\"], \"mutual\": true} and {\"attribute\": \"location\", \"max_distance_km\": 50} should keep most likes and drop most passes: inside the subject's preferred age range and within 50 km 34 % of views become likes, outside 2 %, about 18 times less; mutual preferences matter for matches, because both sides decide. Apply attributes.csv in time order together with the signals: preferences, locations and interests change, and the user rows (signals, attributes) of deleted accounts must be removed at their deletion time (users.csv). Mutual likes (match) are the target for reciprocal matching (v0.6). With a retention of 60 days on the likes relation, pairs whose last signal is older than 60 days before the end drop out. Gender has no effect: models and filters should not need it. users.csv holds the hidden attractiveness.",
		},
		Signals: e.out,
		Tables: []Table{
			schemaTable(
				[]string{"user", "age", "number", "true"},
				[]string{"user", "min_age", "number", "false"},
				[]string{"user", "max_age", "number", "false"},
				[]string{"user", "gender", "category", "true"},
				[]string{"user", "location", "geo", "true"},
				[]string{"user", "interests", "set", "false"},
			),
			attrTable(ev),
			ut,
		},
	}
}

// product is the state of a product in Shop.
type product struct {
	cat, brand          int
	price, quality      float64
	tags                []string
	launch, discontinue time.Time // discontinue is zero for products still sold
}

func (p *product) sold(t time.Time) bool {
	return !t.Before(p.launch) && (p.discontinue.IsZero() || t.Before(p.discontinue))
}

// Shop is a shop where prices, budgets and tags matter, with price
// changes, product launches and discontinued products.
func Shop() *Set {
	const users, products, categories, brands, tagCount, per, span = 4000, 2500, 15, 60, 30, 20, 90 * day
	rng := newRand(15)
	end := Start.Add(span)
	tagNames := make([]string, tagCount)
	for i := range tagNames {
		tagNames[i] = id("t", i)
	}
	base := make([]float64, categories) // median price per category, 10–500
	for c := range base {
		base[c] = 10 * math.Pow(50, rng.Float64())
	}
	var ev []attrEvent
	pt := Table{Name: "products.csv", Header: []string{"product_id", "quality", "launch", "discontinued"}}
	ps := make([]*product, products)
	byCat := make([][]int, categories)
	type change struct {
		t     time.Time
		apply func()
	}
	var changes []change
	for i := range products {
		p := &product{
			cat: rng.IntN(categories), brand: rng.IntN(brands),
			quality: math.Round(rng.NormFloat64()*1000) / 1000,
			tags:    pick(rng, tagNames, 2+rng.IntN(3)),
			launch:  Start,
		}
		p.price = math.Round(base[p.cat]*math.Exp(0.4*rng.NormFloat64())*100) / 100
		if rng.Float64() < 0.2 {
			p.launch = randTime(rng, Start.Add(30*day), 60*day)
		}
		if p.launch.Equal(Start) && rng.Float64() < 0.05 {
			p.discontinue = randTime(rng, Start.Add(45*day), 45*day)
		}
		ps[i] = p
		byCat[p.cat] = append(byCat[p.cat], i)
		pid := id("p", i)
		ev = append(ev,
			setAttr(p.launch, "product", pid, "category", id("c", p.cat)),
			setAttr(p.launch, "product", pid, "brand", id("b", p.brand)),
			setAttr(p.launch, "product", pid, "price", num(p.price)),
			setAttr(p.launch, "product", pid, "tags", setValue(p.tags)),
		)
		if rng.Float64() < 0.3 {
			ev = append(ev, setAttr(p.launch, "product", pid, "store", geo(near(rng, rng.IntN(len(cities))))))
		}
		last := end
		if !p.discontinue.IsZero() {
			last = p.discontinue
			ev = append(ev, attrEvent{t: p.discontinue, kind: "product", id: pid, op: "delete"})
		}
		if rng.Float64() < 0.1 && last.Sub(p.launch) > 2*day {
			t := randTime(rng, p.launch.Add(day), last.Sub(p.launch)-day)
			np := math.Round(p.price*(0.5+rng.Float64())*100) / 100
			ev = append(ev, setAttr(t, "product", pid, "price", num(np)))
			changes = append(changes, change{t, func() { p.price = np }})
		}
		disc := ""
		if !p.discontinue.IsZero() {
			disc = p.discontinue.Format(time.RFC3339)
		}
		pt.Rows = append(pt.Rows, []string{pid, f(p.quality, 3), p.launch.Format(time.RFC3339), disc})
	}

	type shopper struct {
		lo, hi float64
		tags   []string
		cats   [2]int
	}
	ut := Table{Name: "users.csv", Header: []string{"user_id", "category_1", "category_2"}}
	us := make([]*shopper, users)
	type visit struct {
		t time.Time
		u int
	}
	var visits []visit
	for u := range users {
		center := 15 * math.Pow(20, rng.Float64())
		s := &shopper{
			lo: math.Round(center/3*100) / 100, hi: math.Round(center*1.5*100) / 100,
			tags: pick(rng, tagNames, 3),
		}
		perm := rng.Perm(categories)
		s.cats = [2]int{perm[0], perm[1]}
		us[u] = s
		uid := id("u", u)
		ev = append(ev,
			setAttr(Start, "user", uid, "min_price", num(s.lo)),
			setAttr(Start, "user", uid, "max_price", num(s.hi)),
			setAttr(Start, "user", uid, "tags", setValue(s.tags)),
			setAttr(Start, "user", uid, "location", geo(near(rng, rng.IntN(len(cities))))),
		)
		if rng.Float64() < 0.1 {
			t := randTime(rng, Start.Add(day), span-day)
			scale := 0.5 + rng.Float64()*1.5
			lo, hi := math.Round(s.lo*scale*100)/100, math.Round(s.hi*scale*100)/100
			ev = append(ev, setAttr(t, "user", uid, "min_price", num(lo)), setAttr(t, "user", uid, "max_price", num(hi)))
			changes = append(changes, change{t, func() { s.lo, s.hi = lo, hi }})
		}
		ut.Rows = append(ut.Rows, []string{uid, id("c", s.cats[0]), id("c", s.cats[1])})
		for range per {
			visits = append(visits, visit{randTime(rng, Start, span), u})
		}
	}
	slices.SortStableFunc(changes, func(a, b change) int { return a.t.Compare(b.t) })
	slices.SortStableFunc(visits, func(a, b visit) int {
		return cmp.Or(a.t.Compare(b.t), cmp.Compare(a.u, b.u))
	})

	e := &emitter{rng: rng}
	weights := make([]float64, 0, products)
	next := 0
	for _, v := range visits {
		for next < len(changes) && !changes[next].t.After(v.t) {
			changes[next].apply()
			next++
		}
		s := us[v.u]
		cands := byCat[s.cats[rng.IntN(2)]]
		if rng.Float64() < 0.3 {
			cands = byCat[rng.IntN(categories)]
		}
		weights = weights[:0]
		for _, i := range cands {
			p := ps[i]
			w := 0.0
			if p.sold(v.t) {
				w = math.Exp(0.5*p.quality + 1.0*float64(overlap(s.tags, p.tags)))
				if p.price < s.lo || p.price > s.hi {
					w *= 0.03
				}
			}
			weights = append(weights, w)
		}
		// No visit if no product of the category is sold at the time.
		if k, ok := drawWeighted(rng, weights); ok {
			p := ps[cands[k]]
			fn := defaultFunnel
			if p.price < s.lo || p.price > s.hi {
				fn.cart *= 0.1
				fn.purchase *= 0.1
			}
			e.interact(id("u", v.u), "product", id("p", cands[k]), v.t, fn, "")
		}
	}
	sortByTime(e.out)
	return &Set{
		Meta: synthetic.Dataset{
			Name: "attributes/shop", Folder: "attributes", Ordered: true,
			Description: fmt.Sprintf("A shop over %d days: %d products in %d categories by %d brands with a price (log-normal around a median of 10–500 per category), 2–4 of %d tags and, for 30 %%, a pickup store location. %d users have a budget min_price–max_price (max = 4.5·min), 3 preferred tags, a location and 2 hidden favorite categories (users.csv). Each user makes %d visits: 70 %% in a favorite category; the product is drawn ∝ exp(0.5·quality + shared tags), ×0.03 outside the budget, and walks down the funnel visit → click → dwell → cart → purchase, with cart and purchase ×0.1 outside the budget. 20 %% of the products launch in days 30–90 (attributes at launch, no signals before), 5 %% are discontinued in days 45–90, 10 %% change their price once, 10 %% of the users change their budget once (all in attributes.csv).", int(span/day), products, categories, brands, tagCount, users, per),
			Expected:    "The filter {\"attribute\": \"price\", \"within\": [\"min_price\", \"max_price\"]} should keep nearly all purchases (99 %) while dropping most products; {\"attribute\": \"tags\", \"same\": true} keeps the products users interact with: 54 % of visits share a tag with the user, against 28 % for a random product. Products launched late have only attributes at first: a cold-start case for feature-based models (v0.10). Discontinued products (products.csv) must be deleted at their time and never recommended afterwards. products.csv holds the hidden quality.",
		},
		Signals: e.out,
		Tables: []Table{
			schemaTable(
				[]string{"product", "category", "category", "false"},
				[]string{"product", "brand", "category", "false"},
				[]string{"product", "price", "number", "false"},
				[]string{"product", "tags", "set", "false"},
				[]string{"product", "store", "geo", "false"},
				[]string{"user", "min_price", "number", "false"},
				[]string{"user", "max_price", "number", "false"},
				[]string{"user", "tags", "set", "false"},
				[]string{"user", "location", "geo", "true"},
			),
			attrTable(ev),
			ut,
			pt,
		},
	}
}
