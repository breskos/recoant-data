package synthetic

import (
	"encoding/csv"
	"errors"
	"math"
	"os"
	"slices"
	"strconv"
	"testing"
)

// TestDatasets reads every file completely and checks it against the
// catalog: row counts, headers, kinds, types and ordering.
func TestDatasets(t *testing.T) {
	if len(Datasets) == 0 {
		t.Fatal("empty catalog")
	}
	for _, d := range Datasets {
		t.Run(d.Name, func(t *testing.T) {
			if d.Name == "benchmark/large" && testing.Short() {
				t.Skip("large file")
			}
			if d.Invalid {
				checkInvalid(t, d)
				return
			}
			rows := 0
			var subjects, objects, types []string
			var last Signal
			err := d.ReadSignals(".", func(s Signal) error {
				rows++
				if d.Ordered && rows > 1 && s.Time.Before(last.Time) {
					t.Fatalf("row %d: not ordered by time", rows)
				}
				last = s
				for _, x := range []struct {
					list *[]string
					v    string
				}{{&subjects, s.SubjectKind}, {&objects, s.ObjectKind}, {&types, s.Type}} {
					if !slices.Contains(*x.list, x.v) {
						*x.list = append(*x.list, x.v)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if rows != d.Signals.Rows {
				t.Errorf("%d signals, catalog says %d", rows, d.Signals.Rows)
			}
			for _, x := range []struct {
				name      string
				got, want []string
			}{{"subject kinds", subjects, d.SubjectKinds}, {"object kinds", objects, d.ObjectKinds}, {"types", types, d.Types}} {
				slices.Sort(x.got)
				want := slices.Sorted(slices.Values(x.want))
				if !slices.Equal(x.got, want) {
					t.Errorf("%s %v, catalog says %v", x.name, x.got, want)
				}
			}
			for _, tbl := range d.Tables {
				n := 0
				err := d.ReadTable(".", tbl.Name, func(row map[string]string) error {
					n++
					if len(row) != len(tbl.Header) {
						t.Fatalf("%s row %d: %d columns", tbl.Name, n, len(row))
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				if n != tbl.Rows {
					t.Errorf("%s: %d rows, catalog says %d", tbl.Name, n, tbl.Rows)
				}
			}
		})
	}
}

func checkInvalid(t *testing.T, d Dataset) {
	f, err := os.Open(d.Path(".", d.Signals.Name))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs)-1 != d.Signals.Rows {
		t.Fatalf("%d rows, catalog says %d", len(recs)-1, d.Signals.Rows)
	}
	for i, rec := range recs[1:] {
		if _, err := ParseSignal(rec); !errors.Is(err, ErrInvalid) {
			t.Errorf("row %d %v accepted: %v", i+1, rec, err)
		}
	}
	if err := d.ReadSignals(".", func(Signal) error { return nil }); err == nil {
		t.Error("ReadSignals accepted an invalid file")
	}
}

func mustLookup(t *testing.T, name string) Dataset {
	t.Helper()
	d, ok := Lookup(name)
	if !ok {
		t.Fatalf("no data set %q", name)
	}
	return d
}

// column reads one column of a table into a map from the first column.
func column(t *testing.T, d Dataset, table, key, col string) map[string]string {
	t.Helper()
	m := map[string]string{}
	if err := d.ReadTable(".", table, func(r map[string]string) error { m[r[key]] = r[col]; return nil }); err != nil {
		t.Fatal(err)
	}
	return m
}

func signals(t *testing.T, d Dataset, fn func(Signal)) {
	t.Helper()
	if err := d.ReadSignals(".", func(s Signal) error { fn(s); return nil }); err != nil {
		t.Fatal(err)
	}
}

func share(hit, total int) float64 { return float64(hit) / float64(max(total, 1)) }

// The following tests check that the hidden structure each data set
// promises is really in the signals.

func TestZipfPopularity(t *testing.T) {
	d := mustLookup(t, "basic/zipf-popularity")
	rank := column(t, d, "items.csv", "item_id", "rank")
	visits := map[string]int{}
	signals(t, d, func(s Signal) {
		if s.Type == "visit" {
			visits[s.ObjectID]++
		}
	})
	top, mid := 0, 0
	for item, n := range visits {
		switch rank[item] {
		case "1":
			top = n
		case "100":
			mid = n
		}
	}
	if top < 10*mid || top == 0 {
		t.Errorf("rank 1 has %d visits, rank 100 has %d", top, mid)
	}
}

func inCluster(t *testing.T, name string) {
	d := mustLookup(t, name)
	uc := column(t, d, "users.csv", "user_id", "cluster")
	ic := column(t, d, "items.csv", "item_id", "cluster")
	hit, total := 0, 0
	signals(t, d, func(s Signal) {
		if s.Type == "visit" && s.ObjectKind == "item" {
			total++
			if uc[s.SubjectID] == ic[s.ObjectID] {
				hit++
			}
		}
	})
	if sh := share(hit, total); sh < 0.75 {
		t.Errorf("in-cluster share %.3f", sh)
	}
}

func TestTasteClusters(t *testing.T) { inCluster(t, "basic/taste-clusters") }

func TestExplicitRatings(t *testing.T) {
	d := mustLookup(t, "latent/explicit-ratings")
	signals(t, d, func(s Signal) {
		if s.Value != math.Round(s.Value) || s.Value < 1 || s.Value > 5 {
			t.Fatalf("rating %v", s.Value)
		}
	})
}

func TestImplicitFeedback(t *testing.T) {
	d := mustLookup(t, "latent/implicit-feedback")
	bias := column(t, d, "items.csv", "item_id", "bias")
	// Purchased items have a higher bias on average than visited ones.
	var sumV, sumP float64
	var nV, nP int
	signals(t, d, func(s Signal) {
		b, _ := strconv.ParseFloat(bias[s.ObjectID], 64)
		switch s.Type {
		case "visit":
			sumV += b
			nV++
		case "purchase":
			sumP += b
			nP++
		}
	})
	if sumP/float64(nP) <= sumV/float64(nV) {
		t.Errorf("mean bias of purchases %.3f ≤ visits %.3f", sumP/float64(nP), sumV/float64(nV))
	}
}

func TestEcommerce(t *testing.T) {
	d := mustLookup(t, "funnel/ecommerce")
	q := column(t, d, "products.csv", "product_id", "quality")
	visits, clicks := [2]int{}, [2]int{}
	signals(t, d, func(s Signal) {
		v, _ := strconv.ParseFloat(q[s.ObjectID], 64)
		b := 0
		if v >= 0.5 {
			b = 1
		}
		switch s.Type {
		case "visit":
			visits[b]++
		case "click":
			clicks[b]++
		}
		if s.Session == "" {
			t.Fatal("signal without session")
		}
	})
	low, high := share(clicks[0], visits[0]), share(clicks[1], visits[1])
	if high < low+0.15 {
		t.Errorf("click rate low quality %.3f, high quality %.3f", low, high)
	}
}

func TestSignalRelevance(t *testing.T) {
	d := mustLookup(t, "funnel/signal-relevance")
	intent := map[[2]string]bool{}
	if err := d.ReadTable(".", "intents.csv", func(r map[string]string) error {
		intent[[2]string{r["user_id"], r["item_id"]}] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	hit, total := map[string]int{}, map[string]int{}
	signals(t, d, func(s Signal) {
		total[s.Type]++
		if intent[[2]string{s.SubjectID, s.ObjectID}] {
			hit[s.Type]++
		}
	})
	rate := func(typ string) float64 { return share(hit[typ], total[typ]) }
	if !(rate("share") > rate("click") && rate("click") > rate("visit") && rate("visit") > 5*rate("hover")) {
		t.Errorf("intent rates: share %.3f click %.3f visit %.3f hover %.4f", rate("share"), rate("click"), rate("visit"), rate("hover"))
	}
	if rate("purchase") != 1 {
		t.Errorf("purchases outside intents: %.3f", rate("purchase"))
	}
}

func TestTrendingNews(t *testing.T) {
	d := mustLookup(t, "stream/trending-news")
	pub := column(t, d, "articles.csv", "article_id", "published")
	fresh, total := 0, 0
	signals(t, d, func(s Signal) {
		if s.Type != "visit" {
			return
		}
		total++
		p, _ := parseTime(pub[s.ObjectID])
		age := s.Time.Sub(p).Hours()
		if age < 0 {
			t.Fatalf("read before publication: %v", s)
		}
		if age < 24 {
			fresh++
		}
	})
	if sh := share(fresh, total); sh < 0.65 {
		t.Errorf("share of reads within 24 h of publication %.3f", sh)
	}
}

func TestPreferenceDrift(t *testing.T) {
	d := mustLookup(t, "stream/preference-drift")
	before := column(t, d, "users.csv", "user_id", "cluster_before")
	after := column(t, d, "users.csv", "user_id", "cluster_after")
	ic := column(t, d, "items.csv", "item_id", "cluster")
	switchAt, _ := parseTime("2026-01-31T00:00:00Z")
	var hitB, nB, hitA, nA int
	signals(t, d, func(s Signal) {
		if s.Type != "visit" {
			return
		}
		if s.Time.Before(switchAt) {
			nB++
			if ic[s.ObjectID] == before[s.SubjectID] {
				hitB++
			}
		} else {
			nA++
			if ic[s.ObjectID] == after[s.SubjectID] {
				hitA++
			}
		}
	})
	if share(hitB, nB) < 0.8 || share(hitA, nA) < 0.8 {
		t.Errorf("in-cluster before %.3f, after %.3f", share(hitB, nB), share(hitA, nA))
	}
}

func TestSessionSequences(t *testing.T) {
	d := mustLookup(t, "session/sequences")
	next := map[string]string{}
	var prevChain, prevItem string
	prevPos := -1
	if err := d.ReadTable(".", "chains.csv", func(r map[string]string) error {
		pos, _ := strconv.Atoi(r["position"])
		if r["chain"] == prevChain && pos == prevPos+1 {
			next[prevItem] = r["item_id"]
		}
		prevChain, prevItem, prevPos = r["chain"], r["item_id"], pos
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	last := map[string]string{}
	follow, steps := 0, 0
	signals(t, d, func(s Signal) {
		if s.Type != "visit" {
			return
		}
		if p, ok := last[s.SubjectID]; ok {
			steps++
			if next[p] == s.ObjectID {
				follow++
			}
		}
		last[s.SubjectID] = s.ObjectID
	})
	if sh := share(follow, steps); sh < 0.6 {
		t.Errorf("share of steps that follow a chain %.3f", sh)
	}
}

func TestSocial(t *testing.T) {
	inCluster(t, "relations/social")
	d := mustLookup(t, "relations/social")
	uc := column(t, d, "users.csv", "user_id", "cluster")
	hit, total := 0, 0
	signals(t, d, func(s Signal) {
		if s.Type == "follow" {
			total++
			if uc[s.SubjectID] == uc[s.ObjectID] {
				hit++
			}
			if s.SubjectID == s.ObjectID {
				t.Fatal("self-follow")
			}
		}
	})
	if sh := share(hit, total); sh < 0.75 {
		t.Errorf("same-cluster follows %.3f", sh)
	}
}

func TestCatalogRelations(t *testing.T) {
	d := mustLookup(t, "relations/catalog")
	brand := map[string]string{}
	if err := d.ReadTable(".", "relations.csv", func(r map[string]string) error {
		if r["relation"] == "by_brand" {
			brand[r["subject_id"]] = r["object_id"]
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	fav := map[string][]string{}
	if err := d.ReadTable(".", "users.csv", func(r map[string]string) error {
		fav[r["user_id"]] = []string{r["brand_1"], r["brand_2"], r["brand_3"]}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	hit, total := 0, 0
	signals(t, d, func(s Signal) {
		if s.Type == "visit" {
			total++
			if slices.Contains(fav[s.SubjectID], brand[s.ObjectID]) {
				hit++
			}
		}
	})
	if sh := share(hit, total); sh < 0.65 {
		t.Errorf("favorite-brand share %.3f", sh)
	}
}

func TestEdgeCases(t *testing.T) {
	d := mustLookup(t, "loading/edge-cases")
	var got []Signal
	signals(t, d, func(s Signal) { got = append(got, s) })
	if got[6].Time.Format("-07:00") != "+01:00" {
		t.Errorf("time zone offset lost: %v", got[6].Time)
	}
	if got[8].SubjectID != "id,with,commas" || got[8].ObjectID != `"quoted"` {
		t.Errorf("quoting: %q %q", got[8].SubjectID, got[8].ObjectID)
	}
}

func TestLookup(t *testing.T) {
	if _, ok := Lookup("nope"); ok {
		t.Error("Lookup of unknown name")
	}
	if len(InFolder("stream")) != 2 {
		t.Errorf("InFolder(stream) = %d", len(InFolder("stream")))
	}
}
