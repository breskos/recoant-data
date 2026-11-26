package gen

import (
	"time"

	"github.com/breskos/recoant-data/synthetic"
)

// EdgeCases holds valid but tricky rows for loaders and ingestion.
func EdgeCases() *Set {
	t := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			panic(err)
		}
		return v
	}
	sig := func(ts, sk, sid, ok, oid, typ string, v float64, sess string) synthetic.Signal {
		return synthetic.Signal{Time: t(ts), SubjectKind: sk, SubjectID: sid, ObjectKind: ok, ObjectID: oid, Type: typ, Value: v, Session: sess}
	}
	rows := []synthetic.Signal{
		sig("2026-01-01T10:00:00Z", "user", "u1", "item", "i1", "visit", 1, "s1"),
		sig("2026-01-01T09:00:00Z", "user", "u1", "item", "i2", "visit", 1, "s1"),            // out of order
		sig("2026-01-01T10:00:00Z", "user", "u1", "item", "i1", "visit", 1, "s1"),            // exact duplicate
		sig("2026-01-01T10:05:00Z", "user", "u1", "item", "i1", "dwell", 0, "s1"),            // zero dwell
		sig("2026-01-01T10:06:00Z", "user", "u1", "item", "i1", "dwell", 86400, "s1"),        // implausibly long dwell
		sig("2026-01-01T10:07:00Z", "user", "u1", "item", "i1", "share", 1, ""),              // unknown type, no session
		sig("2026-01-01T11:00:00+01:00", "user", "u2", "item", "i1", "click", 1, ""),         // time zone offset
		sig("2026-01-01T10:10:00Z", "user", "nutzer-ä", "item", "artikel-ß", "visit", 1, ""), // non-ASCII IDs
		sig("2026-01-01T10:11:00Z", "user", "id,with,commas", "item", "\"quoted\"", "visit", 1, ""),
		sig("2026-01-01T10:12:00Z", "user", "u3", "item", "i3", "purchase", 3, ""),        // quantity 3
		sig("2026-01-01T10:13:00Z", "user", "u3", "item", "i3", "rating", 4.5, ""),        // fractional value
		sig("2026-01-01T10:14:00Z", "user", "u3", "item", "i3", "return", -1, ""),         // negative value
		sig("2026-01-01T10:15:00Z", "user", "u3", "user", "u1", "follow", 1, ""),          // user → user
		sig("2026-01-01T10:16:00Z", "item", "i1", "item", "i2", "bought_together", 1, ""), // entity → entity
		sig("2026-01-01T10:17:00Z", "user", "u3", "user", "u3", "visit", 1, ""),           // self-loop
		sig("1999-12-31T23:59:59Z", "user", "u4", "item", "i1", "visit", 1, ""),           // far past
		sig("2030-01-01T00:00:00Z", "user", "u4", "item", "i2", "visit", 1, ""),           // future
		sig("2026-01-01T10:18:00Z", "User", "U1", "Item", "I1", "Visit", 1, ""),           // case differs: distinct kinds, IDs and type
	}
	return &Set{
		Meta: synthetic.Dataset{
			Name: "loading/edge-cases", Folder: "loading", Ordered: false,
			Description: "Valid but tricky signals: out-of-order times, an exact duplicate, zero and implausibly long dwell, an unknown type without session, a time zone offset, non-ASCII IDs and IDs with commas and quotes, quantities, fractional and negative values, user → user and item → item signals, a self-loop, far past and future times, and kinds, IDs and types that differ only in case.",
			Expected:    "Every row must be accepted by ingestion. What happens to each is a policy question (e.g. ignore unknown types, cap dwell time); tests should state it explicitly.",
		},
		Signals: rows,
	}
}

// Invalid holds rows that must be rejected, one problem per row.
func Invalid() *Set {
	return &Set{
		Meta: synthetic.Dataset{
			Name: "loading/invalid", Folder: "loading", Invalid: true,
			Description: "One problem per row: unparseable time, empty subject kind, empty subject ID, empty object kind, empty object ID, empty type, non-numeric value, NaN, infinite value, missing column.",
			Expected:    "Every row must be rejected with an error; none may panic or be silently accepted.",
		},
	}
}

// InvalidRows are the raw rows of loading/invalid.
var InvalidRows = [][]string{
	{"yesterday", "user", "u1", "item", "i1", "visit", "1", ""},
	{"2026-01-01T00:00:00Z", "", "u1", "item", "i1", "visit", "1", ""},
	{"2026-01-01T00:00:00Z", "user", "", "item", "i1", "visit", "1", ""},
	{"2026-01-01T00:00:00Z", "user", "u1", "", "i1", "visit", "1", ""},
	{"2026-01-01T00:00:00Z", "user", "u1", "item", "", "visit", "1", ""},
	{"2026-01-01T00:00:00Z", "user", "u1", "item", "i1", "", "1", ""},
	{"2026-01-01T00:00:00Z", "user", "u1", "item", "i1", "visit", "one", ""},
	{"2026-01-01T00:00:00Z", "user", "u1", "item", "i1", "visit", "NaN", ""},
	{"2026-01-01T00:00:00Z", "user", "u1", "item", "i1", "visit", "+Inf", ""},
	{"2026-01-01T00:00:00Z", "user", "u1", "item", "i1", "visit", "1"},
}
