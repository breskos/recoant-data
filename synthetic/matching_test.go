package synthetic

import (
	"strconv"
	"testing"
)

// The dating set has the structure reciprocal matching needs: a like
// returns much more often between users whose styles fit both ways, and
// attractive users receive many likes but return few.
func TestDating(t *testing.T) {
	d := mustLookup(t, "matching/dating")
	type user struct {
		style, pref int
		attract     float64
	}
	users := map[string]user{}
	if err := d.ReadTable(".", "users.csv", func(r map[string]string) error {
		s, _ := strconv.Atoi(r["style"])
		p, _ := strconv.Atoi(r["preferred_style"])
		a, _ := strconv.ParseFloat(r["attractiveness"], 64)
		users[r["user_id"]] = user{s, p, a}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	liked := map[[2]string]bool{}
	signals(t, d, func(s Signal) {
		if s.Type == "like" {
			liked[[2]string{s.SubjectID, s.ObjectID}] = true
		}
	})
	var fit, other, star, rest [2]int // returned likes, likes
	for pair := range liked {
		u, v := users[pair[0]], users[pair[1]]
		back := 0
		if liked[[2]string{pair[1], pair[0]}] {
			back = 1
		}
		k := &other
		if v.pref == u.style {
			k = &fit
		}
		k[0], k[1] = k[0]+back, k[1]+1
		k = &rest
		if v.attract > 1.28 { // the top tenth of N(0, 1)
			k = &star
		}
		k[0], k[1] = k[0]+back, k[1]+1
	}
	rate := func(k [2]int) float64 { return float64(k[0]) / float64(k[1]) }
	if rate(fit) < 2.5*rate(other) {
		t.Errorf("returned likes: %.3f when the liked user prefers the liker's style, %.3f otherwise", rate(fit), rate(other))
	}
	if rate(star) > rate(rest)/2 {
		t.Errorf("returned likes: %.3f by attractive users, %.3f by the rest", rate(star), rate(rest))
	}
	t.Logf("returned: fit %.3f other %.3f star %.3f rest %.3f", rate(fit), rate(other), rate(star), rate(rest))
}
