// Package synthetic reads the synthetic data sets of recoant-data.
//
// Every data set is a folder with a signals file and, depending on the set,
// tables with the hidden structure that generated it (user clusters, latent
// factors, item quality, …), so you can tell how well a recommender should
// do. [Datasets] lists every set; [ReadSignals] and [ReadTable] stream the
// files.
//
// The package only uses the standard library and does not depend on
// recoant. A signal maps one to one onto recoant.Signal:
//
//	recoant.Signal{
//		Subject: recoant.Ref{Kind: recoant.Kind(s.SubjectKind), ID: s.SubjectID},
//		Object:  recoant.Ref{Kind: recoant.Kind(s.ObjectKind), ID: s.ObjectID},
//		Type:    recoant.SignalType(s.Type),
//		Value:   s.Value, Time: s.Time, Session: s.Session,
//	}
package synthetic

import (
	"bufio"
	"compress/gzip"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// SignalHeader is the header of every signals file.
var SignalHeader = []string{"time", "subject_kind", "subject_id", "object_kind", "object_id", "type", "value", "session"}

// ErrInvalid is returned for a row that is not a valid signal.
var ErrInvalid = errors.New("synthetic: invalid signal")

// Signal is one row of a signals file.
type Signal struct {
	Time        time.Time
	SubjectKind string
	SubjectID   string
	ObjectKind  string
	ObjectID    string
	Type        string
	Value       float64
	Session     string // may be empty
}

// Record returns the signal as a CSV record in the column order of
// [SignalHeader].
func (s Signal) Record() []string {
	return []string{
		s.Time.Format(time.RFC3339),
		s.SubjectKind, s.SubjectID, s.ObjectKind, s.ObjectID, s.Type,
		strconv.FormatFloat(s.Value, 'f', -1, 64), s.Session,
	}
}

// ParseSignal parses a CSV record in the column order of [SignalHeader].
func ParseSignal(rec []string) (Signal, error) {
	if len(rec) != len(SignalHeader) {
		return Signal{}, fmt.Errorf("%w: %d fields, want %d", ErrInvalid, len(rec), len(SignalHeader))
	}
	t, err := time.Parse(time.RFC3339, rec[0])
	if err != nil {
		return Signal{}, fmt.Errorf("%w: time: %w", ErrInvalid, err)
	}
	for i := 1; i <= 5; i++ {
		if rec[i] == "" {
			return Signal{}, fmt.Errorf("%w: empty %s", ErrInvalid, SignalHeader[i])
		}
	}
	v, err := strconv.ParseFloat(rec[6], 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return Signal{}, fmt.Errorf("%w: value %q", ErrInvalid, rec[6])
	}
	return Signal{
		Time: t, SubjectKind: rec[1], SubjectID: rec[2], ObjectKind: rec[3], ObjectID: rec[4],
		Type: rec[5], Value: v, Session: rec[7],
	}, nil
}

// File describes one file of a data set.
type File struct {
	Name   string   // file name within the data set folder
	Rows   int      // rows without the header
	Header []string // column names
}

// Dataset describes one data set.
type Dataset struct {
	// Name is the folder of the data set relative to this folder, e.g.
	// "basic/taste-clusters".
	Name string
	// Folder is the purpose: basic, latent, funnel, stream, session,
	// relations, loading or benchmark.
	Folder      string
	Description string
	// Expected says what a recommender should achieve on the set.
	Expected string
	// Signals is the signals file.
	Signals File
	// Tables hold the hidden structure (ground truth) and relations.
	Tables []File
	// SubjectKinds and ObjectKinds list the kinds in the signals.
	SubjectKinds, ObjectKinds []string
	// Types lists the signal types.
	Types []string
	// Ordered marks signals files sorted by time.
	Ordered bool
	// Invalid marks files whose rows are all invalid on purpose.
	Invalid bool
}

// Lookup returns the data set with the given name.
func Lookup(name string) (Dataset, bool) {
	i := slices.IndexFunc(Datasets, func(d Dataset) bool { return d.Name == name })
	if i < 0 {
		return Dataset{}, false
	}
	return Datasets[i], true
}

// InFolder returns the data sets of one folder, e.g. "stream".
func InFolder(folder string) []Dataset {
	var out []Dataset
	for _, d := range Datasets {
		if d.Folder == folder {
			out = append(out, d)
		}
	}
	return out
}

// Path returns the path of a file of the data set below root, the folder
// of this package.
func (d Dataset) Path(root, file string) string {
	return filepath.Join(root, filepath.FromSlash(d.Name), file)
}

// ReadSignals calls fn for every signal of the data set in file order.
func (d Dataset) ReadSignals(root string, fn func(Signal) error) error {
	return ReadSignals(d.Path(root, d.Signals.Name), fn)
}

// ReadTable calls fn for every row of a table of the data set.
func (d Dataset) ReadTable(root, name string, fn func(row map[string]string) error) error {
	return ReadTable(d.Path(root, name), fn)
}

// ReadSignals calls fn for every signal in a signals file (.csv or
// .csv.gz). It stops at the first invalid row or error from fn.
func ReadSignals(path string, fn func(Signal) error) error {
	return readCSV(path, func(line int, header, rec []string) error {
		if line == 1 {
			if !slices.Equal(header, SignalHeader) {
				return fmt.Errorf("%s: header %v, want %v", path, header, SignalHeader)
			}
			return nil
		}
		s, err := ParseSignal(rec)
		if err != nil {
			return fmt.Errorf("%s:%d: %w", path, line, err)
		}
		return fn(s)
	})
}

// ReadTable calls fn for every row of a CSV table with a header, as a map
// from column name to value.
func ReadTable(path string, fn func(row map[string]string) error) error {
	return readCSV(path, func(line int, header, rec []string) error {
		if line == 1 {
			return nil
		}
		row := make(map[string]string, len(header))
		for i, h := range header {
			row[h] = rec[i]
		}
		return fn(row)
	})
}

func readCSV(path string, fn func(line int, header, rec []string) error) (err error) {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	var r io.Reader = bufio.NewReaderSize(f, 1<<20)
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(r)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		defer func() { _ = gz.Close() }()
		r = gz
	}
	cr := csv.NewReader(r)
	cr.ReuseRecord = true
	header, err := cr.Read()
	if err != nil {
		return fmt.Errorf("%s: header: %w", path, err)
	}
	header = slices.Clone(header)
	if err := fn(1, header, nil); err != nil {
		return err
	}
	for line := 2; ; line++ {
		rec, err := cr.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if err := fn(line, header, rec); err != nil {
			return err
		}
	}
}

func parseTime(s string) (time.Time, error) { return time.Parse(time.RFC3339, s) }
