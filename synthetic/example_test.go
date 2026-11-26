package synthetic_test

import (
	"fmt"

	"github.com/breskos/recoant-data/synthetic"
)

// Count the signal types of a data set.
func Example() {
	d, _ := synthetic.Lookup("basic/taste-clusters")
	counts := map[string]int{}
	err := d.ReadSignals(".", func(s synthetic.Signal) error {
		counts[s.Type]++
		return nil
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(len(d.Types), "types,", counts["visit"], "visits")
	// Output: 5 types, 100000 visits
}

// Read the hidden structure: the true cluster of every user.
func ExampleDataset_ReadTable() {
	d, _ := synthetic.Lookup("basic/taste-clusters")
	clusters := map[string]string{}
	_ = d.ReadTable(".", "users.csv", func(row map[string]string) error {
		clusters[row["user_id"]] = row["cluster"]
		return nil
	})
	fmt.Println(len(clusters), "users, u42 is in cluster", clusters["u42"])
	// Output: 5000 users, u42 is in cluster 2
}
