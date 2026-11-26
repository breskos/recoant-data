package gen

import (
	"fmt"
	"strconv"
	"time"

	"github.com/breskos/recoant-data/synthetic"
)

// customerSegment is a hidden segment of Customers.
type customerSegment struct {
	name           string
	categories     []int // preferred product categories
	ageMin, ageMax int
	premium        float64 // share with a premium membership
}

// Customers is a shop whose customers fall into six hidden segments. Pairs
// of segments buy the same categories and differ only in age, and age
// ranges overlap across pairs: neither behavior nor attributes alone
// separate the segments, both together do.
func Customers() *Set {
	const users, perCategory, span = 4000, 50, 90 * day
	categories := []string{"games", "electronics", "garden", "household", "sport", "kitchen", "books", "fashion"}
	segments := []customerSegment{
		{"students", []int{0, 1}, 18, 25, 0.05},
		{"techies", []int{0, 1}, 30, 45, 0.4},
		{"families", []int{2, 3}, 30, 45, 0.2},
		{"retirees", []int{2, 3}, 62, 80, 0.3},
		{"outdoor", []int{4, 6}, 25, 55, 0.2},
		{"gourmets", []int{5, 6}, 35, 65, 0.5},
	}
	cities := []string{"Berlin", "Hamburg", "Munich", "Cologne", "Leipzig"}
	rng := newRand(31)
	zipf := newZipf(rng, 1.0, perCategory)

	var ev []attrEvent
	for c, name := range categories {
		for i := range perCategory {
			pid := id("p", c*perCategory+i)
			ev = append(ev, setAttr(Start, "product", pid, "category", name),
				setAttr(Start, "product", pid, "price", num(float64(5+rng.IntN(200)))))
		}
	}
	ut := Table{Name: "users.csv", Header: []string{"user_id", "segment", "segment_name"}}
	e := &emitter{rng: rng}
	for u := range users {
		s := rng.IntN(len(segments))
		seg := segments[s]
		uid := id("u", u)
		membership := "basic"
		if rng.Float64() < seg.premium {
			membership = "premium"
		}
		signup := randTime(rng, Start, 10*day)
		ev = append(ev,
			setAttr(signup, "user", uid, "age", strconv.Itoa(seg.ageMin+rng.IntN(seg.ageMax-seg.ageMin+1))),
			setAttr(signup, "user", uid, "city", cities[rng.IntN(len(cities))]),
			setAttr(signup, "user", uid, "membership", membership),
		)
		ut.Rows = append(ut.Rows, []string{uid, strconv.Itoa(s), seg.name})
		for range 15 + rng.IntN(26) {
			c := seg.categories[rng.IntN(len(seg.categories))]
			if rng.Float64() < 0.2 {
				c = rng.IntN(len(categories))
			}
			pid := id("p", c*perCategory+zipf.draw())
			t := randTime(rng, signup, Start.Add(span).Sub(signup))
			e.emit("user", uid, "product", pid, "view", 1, t, "")
			if rng.Float64() < 0.3 {
				e.emit("user", uid, "product", pid, "purchase", 1, t.Add(time.Duration(1+rng.IntN(30))*time.Minute), "")
			}
		}
	}
	sortByTime(e.out)
	return &Set{
		Meta: synthetic.Dataset{
			Name: "segments/customers", Folder: "segments", Ordered: true,
			Description: fmt.Sprintf("A shop with %d customers in 6 hidden segments over %d days: students (age 18–25) and techies (30–45) buy games and electronics; families (30–45) and retirees (62–80) garden and household; outdoor people (25–55) sport and books; gourmets (35–65) kitchen and books. %d products in 8 categories (%d each, Zipf popularity within a category, price 5–204). Each customer views 15–40 products, 80 %% from the segment's categories, and buys 30 %% of them. Attributes: age (sensitive), city (no influence) and membership (premium share 5–50 %% by segment).", users, int(span/day), len(categories)*perCategory, perCategory),
			Expected:    "Segments with k = 6 should recover the hidden segments only from behavior and attributes together: taste vectors alone merge the pairs that buy the same categories (students and techies, families and retirees), age alone merges segments whose age ranges overlap (techies, families, outdoor, gourmets). City carries no information and should not help. Age is sensitive: clustering on it needs an explicit decision (allow_sensitive). users.csv holds the true segment of every customer.",
		},
		Signals: e.out,
		Tables: []Table{
			schemaTable(
				[]string{"user", "age", "number", "true"},
				[]string{"user", "city", "category", "false"},
				[]string{"user", "membership", "category", "false"},
				[]string{"product", "category", "category", "false"},
				[]string{"product", "price", "number", "false"},
			),
			attrTable(ev),
			ut,
		},
	}
}
