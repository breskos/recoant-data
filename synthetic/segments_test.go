package synthetic

import (
	"strconv"
	"testing"
)

// Each hidden segment of segments/customers has its age range and mostly
// buys its own categories; segments that share categories differ in age.
func TestCustomers(t *testing.T) {
	d := mustLookup(t, "segments/customers")
	seg := map[string]string{}
	if err := d.ReadTable(".", "users.csv", func(r map[string]string) error {
		seg[r["user_id"]] = r["segment_name"]
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ages := map[string][2]int{}
	category := map[string]string{}
	if err := d.ReadTable(".", "attributes.csv", func(r map[string]string) error {
		switch r["attribute"] {
		case "age":
			a, _ := strconv.Atoi(r["value"])
			s := seg[r["id"]]
			lo, hi := ages[s][0], ages[s][1]
			if lo == 0 || a < lo {
				lo = a
			}
			ages[s] = [2]int{lo, max(hi, a)}
		case "category":
			category[r["id"]] = r["value"]
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want := map[string][2]int{"students": {18, 25}, "techies": {30, 45}, "families": {30, 45}, "retirees": {62, 80}, "outdoor": {25, 55}, "gourmets": {35, 65}}
	for s, r := range want {
		if ages[s] != r {
			t.Errorf("%s: ages %v, want %v", s, ages[s], r)
		}
	}
	own := map[string][]string{"students": {"games", "electronics"}, "techies": {"games", "electronics"}, "families": {"garden", "household"},
		"retirees": {"garden", "household"}, "outdoor": {"sport", "books"}, "gourmets": {"kitchen", "books"}}
	hits, n := 0, 0
	signals(t, d, func(s Signal) {
		if s.Type != "view" {
			return
		}
		n++
		c := category[s.ObjectID]
		for _, x := range own[seg[s.SubjectID]] {
			if x == c {
				hits++
			}
		}
	})
	if share := float64(hits) / float64(n); share < 0.8 {
		t.Errorf("own categories %.2f of views", share)
	}
}
