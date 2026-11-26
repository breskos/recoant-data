package synthetic

import (
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordRoundTrip(t *testing.T) {
	s := Signal{Time: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), SubjectKind: "user", SubjectID: "u1",
		ObjectKind: "item", ObjectID: "i,1", Type: "dwell", Value: 12.5, Session: "s1"}
	back, err := ParseSignal(s.Record())
	if err != nil || back != s {
		t.Errorf("round trip: %+v %v", back, err)
	}
}

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func gz(t *testing.T, data string) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "x.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := gzip.NewWriter(f)
	_, _ = z.Write([]byte(data))
	_ = z.Close()
	_ = f.Close()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestReadErrors(t *testing.T) {
	header := "time,subject_kind,subject_id,object_kind,object_id,type,value,session\n"
	none := func(Signal) error { return nil }
	stop := errors.New("stop")
	cases := map[string]struct {
		path string
		fn   func(Signal) error
	}{
		"missing":      {filepath.Join(t.TempDir(), "nope.csv"), none},
		"empty":        {writeTemp(t, "e.csv", nil), none},
		"header":       {writeTemp(t, "h.csv", []byte("a,b\n")), none},
		"bad gzip":     {writeTemp(t, "b.csv.gz", []byte("not gzip")), none},
		"bad row":      {writeTemp(t, "r.csv", []byte(header+"x,user,u,item,i,visit,1,\n")), none},
		"bad csv":      {writeTemp(t, "c.csv", []byte(header+"\"unterminated\n")), none},
		"stop":         {writeTemp(t, "s.csv", []byte(header+"2026-01-01T00:00:00Z,user,u,item,i,visit,1,\n")), func(Signal) error { return stop }},
		"gzip content": {writeTemp(t, "g.csv.gz", gz(t, "a,b\n")), none},
	}
	for name, c := range cases {
		if err := ReadSignals(c.path, c.fn); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	n := 0
	path := writeTemp(t, "ok.csv.gz", gz(t, header+"2026-01-01T00:00:00Z,user,u,item,i,visit,1,\n"))
	if err := ReadSignals(path, func(Signal) error { n++; return nil }); err != nil || n != 1 {
		t.Errorf("gzip: %d %v", n, err)
	}
	if err := ReadTable(writeTemp(t, "t.csv", []byte("a\n1\n")), func(map[string]string) error { return stop }); !errors.Is(err, stop) {
		t.Errorf("ReadTable stop: %v", err)
	}
}
