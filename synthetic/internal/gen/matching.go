package gen

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/breskos/recoant-data/synthetic"
)

// Dating is a two-sided market where both sides decide: every user has a
// hidden style and prefers one style in others, attractive users get
// most of the attention and are pickier themselves.
func Dating() *Set {
	const users, styles, span = 3000, 6, 90 * day
	rng := newRand(21)
	style, pref := make([]int, users), make([]int, users)
	attract, base := make([]float64, users), make([]float64, users)
	weight := make([]float64, users)
	for u := range users {
		style[u], pref[u] = rng.IntN(styles), rng.IntN(styles)
		attract[u] = math.Round(rng.NormFloat64()*1000) / 1000
		base[u] = -2.0 - 0.8*max(attract[u], 0) // attractive users are pickier
		weight[u] = math.Exp(attract[u])        // and shown more often
	}
	type view struct {
		t    time.Time
		u, v int
	}
	var views []view
	popular := newCategorical(weight)
	seen := make([]map[int]bool, users)
	for u := range users {
		seen[u] = map[int]bool{u: true}
		for range 60 {
			v := rng.IntN(users)
			if rng.Float64() < 0.3 {
				v = popular.draw(rng)
			}
			if seen[u][v] {
				continue
			}
			seen[u][v] = true
			views = append(views, view{t: randTime(rng, Start, span), u: u, v: v})
		}
	}
	slices.SortStableFunc(views, func(a, b view) int { return a.t.Compare(b.t) })

	e := &emitter{rng: rng}
	liked := make([]map[int]bool, users)
	for u := range liked {
		liked[u] = map[int]bool{}
	}
	received, given := make([]int, users), make([]int, users)
	// decide: u views v at t and likes or passes. A like is shown to v,
	// who looks at u in half of the cases ("who liked you").
	var decide func(u, v int, t time.Time)
	decide = func(u, v int, t time.Time) {
		uid, vid := id("u", u), id("u", v)
		e.emit("user", uid, "user", vid, "view", 1, t, "")
		x := base[u] + 0.6*attract[v]
		if style[v] == pref[u] {
			x += 3.0
		}
		t = t.Add(time.Duration(3+rng.IntN(40)) * time.Second)
		if rng.Float64() < sigmoid(x) {
			e.emit("user", uid, "user", vid, "like", 1, t, "")
			liked[u][v] = true
			received[v]++
			given[u]++
			if liked[v][u] && rng.Float64() < 0.7 {
				e.emit("user", uid, "user", vid, "message", 1, t.Add(time.Duration(1+rng.IntN(120))*time.Minute), "")
			}
			if !seen[v][u] && rng.Float64() < 0.5 {
				seen[v][u] = true
				decide(v, u, t.Add(time.Duration(1+rng.IntN(48*60))*time.Minute))
			}
			return
		}
		if rng.Float64() < 0.8 {
			e.emit("user", uid, "user", vid, "pass", 1, t, "")
		}
	}
	for _, w := range views {
		decide(w.u, w.v, w.t)
	}
	sortByTime(e.out)

	// Statistics for the description: how concentrated the likes are and
	// how often a like is returned, for the most attractive tenth and the
	// rest.
	order := make([]int, users)
	for i := range order {
		order[i] = i
	}
	slices.SortFunc(order, func(a, b int) int { return cmp.Compare(attract[b], attract[a]) })
	top := map[int]bool{}
	for _, u := range order[:users/10] {
		top[u] = true
	}
	likes, topLikes := 0, 0
	var back, n [2]int // returned likes and likes, to the rest [0] and to the top [1]
	for u := range users {
		for v := range liked[u] {
			likes++
			k := 0
			if top[v] {
				topLikes, k = topLikes+1, 1
			}
			n[k]++
			if liked[v][u] {
				back[k]++
			}
		}
	}
	share := func(a, b int) float64 { return 100 * float64(a) / float64(max(b, 1)) }

	ut := Table{Name: "users.csv", Header: []string{"user_id", "style", "preferred_style", "attractiveness", "likes_received", "likes_given"}}
	for u := range users {
		ut.Rows = append(ut.Rows, []string{id("u", u), strconv.Itoa(style[u]), strconv.Itoa(pref[u]), f(attract[u], 3), strconv.Itoa(received[u]), strconv.Itoa(given[u])})
	}
	return &Set{
		Meta: synthetic.Dataset{
			Name: "matching/dating", Folder: "matching", Ordered: true,
			Description: fmt.Sprintf("A dating app where both sides decide, over %d days. %d users each have a hidden style (one of %d) and prefer one style in others; attractiveness is N(0, 1). Each user views up to 60 profiles: 70 %% drawn at random, 30 %% weighted by e^attractiveness, as an app that ranks by popularity would. A like is shown to the liked user, who looks at the liker in 50 %% of the cases (\"who liked you\") and decides the same way. Like probability σ(−2 − 0.8·max(own attractiveness, 0) + 0.6·attractiveness of the other + 3·[the other has the preferred style]): attractive profiles get more likes and are pickier. Otherwise 80 %% pass. A like that completes a mutual like is followed by a message in 70 %% of cases. The most attractive 10 %% receive %.0f %% of all likes; a like to them is returned in %.1f %% of cases, a like to the others in %.1f %%.", int(span/day), users, styles, share(topLikes, likes), share(back[1], n[1]), share(back[0], n[0])),
			Expected:    "The target is matches, not likes: a recommendation succeeds if the user likes the profile and the profile likes the user back. One-way models (popularity, ALS on likes) recommend what the user likes, which includes many attractive profiles who rarely like back; reciprocal matching (recoant v0.6: interest in both directions, combined with a harmonic mean) should raise the mutual hit rate, and exposure balancing should spread recommendations (lower Gini) without losing matches. users.csv holds the hidden style, preferred style and attractiveness: a perfect matcher recommends profiles whose style the user prefers and who prefer the user's style.",
		},
		Signals: e.out,
		Tables:  []Table{ut},
	}
}
