package synthetic

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// attrRow is one row of an attributes.csv table.
type attrRow struct {
	t                         time.Time
	kind, id, op, attr, value string
}

// replay calls onAttr and onSignal in time order; attribute events come
// before signals of the same time.
func replay(t *testing.T, d Dataset, onAttr func(attrRow), onSignal func(Signal)) {
	t.Helper()
	var rows []attrRow
	if err := d.ReadTable(".", "attributes.csv", func(r map[string]string) error {
		tm, err := parseTime(r["time"])
		if err != nil {
			return err
		}
		rows = append(rows, attrRow{tm, r["kind"], r["id"], r["op"], r["attribute"], r["value"]})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !slices.IsSortedFunc(rows, func(a, b attrRow) int { return a.t.Compare(b.t) }) {
		t.Fatal("attributes.csv is not ordered by time")
	}
	next := 0
	signals(t, d, func(s Signal) {
		for next < len(rows) && !rows[next].t.After(s.Time) {
			onAttr(rows[next])
			next++
		}
		onSignal(s)
	})
	for ; next < len(rows); next++ {
		onAttr(rows[next])
	}
}

// entities holds the current attributes of every entity during a replay.
type entities map[string]map[string]string

func (e entities) apply(t *testing.T, r attrRow, schema map[string]string) {
	key := r.kind + ":" + r.id
	switch {
	case r.op == "set":
		typ, ok := schema[r.kind+"."+r.attr]
		if !ok {
			t.Fatalf("attribute %s.%s not in schema", r.kind, r.attr)
		}
		checkValue(t, typ, r.value)
		if e[key] == nil {
			e[key] = map[string]string{}
		}
		e[key][r.attr] = r.value
	case r.op == "delete" && r.attr == "":
		if e[key] == nil {
			t.Fatalf("delete of unknown entity %s", key)
		}
		e[key] = nil
		e["deleted:"+key] = map[string]string{}
	case r.op == "delete":
		delete(e[key], r.attr)
	default:
		t.Fatalf("op %q", r.op)
	}
}

func checkValue(t *testing.T, typ, v string) {
	t.Helper()
	switch typ {
	case "number":
		if _, err := strconv.ParseFloat(v, 64); err != nil {
			t.Fatalf("number %q", v)
		}
	case "geo":
		lat, lon, ok := latLon(v)
		if !ok || math.Abs(lat) > 90 || math.Abs(lon) > 180 {
			t.Fatalf("geo %q", v)
		}
	case "set":
		for _, x := range strings.Split(v, "|") {
			if x == "" {
				t.Fatalf("set %q", v)
			}
		}
	default:
		if v == "" {
			t.Fatal("empty category")
		}
	}
}

func latLon(v string) (lat, lon float64, ok bool) {
	a, b, found := strings.Cut(v, ",")
	lat, err1 := strconv.ParseFloat(a, 64)
	lon, err2 := strconv.ParseFloat(b, 64)
	return lat, lon, found && err1 == nil && err2 == nil
}

func distanceKm(a, b string) float64 {
	la1, lo1, _ := latLon(a)
	la2, lo2, _ := latLon(b)
	rad := math.Pi / 180
	h := math.Pow(math.Sin((la2-la1)*rad/2), 2) + math.Cos(la1*rad)*math.Cos(la2*rad)*math.Pow(math.Sin((lo2-lo1)*rad/2), 2)
	return 2 * 6371.0088 * math.Asin(math.Sqrt(min(h, 1)))
}

func readSchema(t *testing.T, d Dataset) map[string]string {
	t.Helper()
	s := map[string]string{}
	if err := d.ReadTable(".", "schema.csv", func(r map[string]string) error {
		s[r["kind"]+"."+r["attribute"]] = r["type"]
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return s
}

func number(m map[string]string, name string) (float64, bool) {
	v, ok := m[name]
	if !ok {
		return 0, false
	}
	x, err := strconv.ParseFloat(v, 64)
	return x, err == nil
}

func within(subject, candidate map[string]string, attr, lo, hi string) bool {
	v, ok1 := number(candidate, attr)
	a, ok2 := number(subject, lo)
	b, ok3 := number(subject, hi)
	return ok1 && ok2 && ok3 && v >= a && v <= b
}

func TestPeople(t *testing.T) {
	d := mustLookup(t, "attributes/people")
	schema := readSchema(t, d)
	ents := entities{}
	liked := map[[2]string]bool{}
	type counts struct{ views, likes int }
	var inside, outside, near, far counts
	byGender := map[string]*counts{}
	messages, unmatched := 0, 0
	replay(t, d, func(r attrRow) { ents.apply(t, r, schema) }, func(s Signal) {
		u, v := ents["user:"+s.SubjectID], ents["user:"+s.ObjectID]
		if u == nil || v == nil {
			t.Fatalf("signal %v involves a deleted or unknown user", s)
		}
		like := s.Type == "like" || s.Type == "superlike"
		mutual := within(u, v, "age", "min_age", "max_age") && within(v, u, "age", "min_age", "max_age")
		close := distanceKm(u["location"], v["location"]) <= 50
		add := func(c *counts) {
			if s.Type == "view" {
				c.views++
			}
			if like {
				c.likes++
			}
		}
		switch {
		case within(u, v, "age", "min_age", "max_age") && close:
			add(&inside)
		default:
			add(&outside)
		}
		if close {
			add(&near)
		} else {
			add(&far)
		}
		if mutual && close {
			g := v["gender"]
			if byGender[g] == nil {
				byGender[g] = &counts{}
			}
			add(byGender[g])
		}
		if like {
			liked[[2]string{s.SubjectID, s.ObjectID}] = true
		}
		if s.Type == "message" {
			messages++
			if !liked[[2]string{s.ObjectID, s.SubjectID}] || !liked[[2]string{s.SubjectID, s.ObjectID}] {
				unmatched++
			}
		}
	})
	rate := func(c counts) float64 { return share(c.likes, c.views) }
	t.Logf("like rate: inside preferences and 50 km %.3f, outside %.3f; within 50 km %.3f, farther %.3f", rate(inside), rate(outside), rate(near), rate(far))
	if rate(inside) < 5*rate(outside) {
		t.Errorf("like rate inside %.3f is not 5 times outside %.3f", rate(inside), rate(outside))
	}
	if rate(near) < 2*rate(far) {
		t.Errorf("like rate near %.3f, far %.3f", rate(near), rate(far))
	}
	for g, c := range byGender {
		t.Logf("gender %s: like rate %.3f", g, rate(*c))
		if math.Abs(rate(*c)-rate(inside)) > 0.2*rate(inside) {
			t.Errorf("gender %s has like rate %.3f, overall %.3f", g, rate(*c), rate(inside))
		}
	}
	if messages == 0 || unmatched > 0 {
		t.Errorf("%d messages, %d without a mutual like", messages, unmatched)
	}
	n := 0
	for k := range ents {
		if strings.HasPrefix(k, "deleted:") {
			n++
		}
	}
	if n < 100 {
		t.Errorf("%d deleted accounts", n)
	}
}

func TestShop(t *testing.T) {
	d := mustLookup(t, "attributes/shop")
	schema := readSchema(t, d)
	ents := entities{}
	var inBudget, purchases, visits, tagged, randomVisits, randomTagged int
	shares := func(a, b string) bool {
		return slices.ContainsFunc(strings.Split(a, "|"), func(x string) bool { return slices.Contains(strings.Split(b, "|"), x) })
	}
	replay(t, d, func(r attrRow) { ents.apply(t, r, schema) }, func(s Signal) {
		u, p := ents["user:"+s.SubjectID], ents["product:"+s.ObjectID]
		if p == nil {
			t.Fatalf("signal %v for a product that is not launched or discontinued", s)
		}
		switch s.Type {
		case "purchase":
			purchases++
			if within(u, p, "price", "min_price", "max_price") {
				inBudget++
			}
		case "visit":
			visits++
			if shares(u["tags"], p["tags"]) {
				tagged++
			}
			// Baseline: a product chosen independently of the user.
			if q := ents["product:p"+strconv.Itoa(visits*7919%2500)]; q != nil {
				randomVisits++
				if shares(u["tags"], q["tags"]) {
					randomTagged++
				}
			}
		}
	})
	t.Logf("purchases in budget %.3f, visits with a shared tag %.3f, random products %.3f", share(inBudget, purchases), share(tagged, visits), share(randomTagged, randomVisits))
	if share(inBudget, purchases) < 0.95 {
		t.Errorf("purchases in budget %.3f", share(inBudget, purchases))
	}
	if share(tagged, visits) < 1.5*share(randomTagged, randomVisits) {
		t.Errorf("visits with a shared tag %.3f, random products %.3f", share(tagged, visits), share(randomTagged, randomVisits))
	}
	launched := map[string]time.Time{}
	if err := d.ReadTable(".", "products.csv", func(r map[string]string) error {
		launched[r["product_id"]], _ = parseTime(r["launch"])
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	late := 0
	for _, l := range launched {
		if l.After(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
			late++
		}
	}
	if late < 300 {
		t.Errorf("%d products launched late", late)
	}
}
