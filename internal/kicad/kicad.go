package kicad

import (
	"encoding/csv"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

func ExpandPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "~" || strings.HasPrefix(p, "~/") {
		u, err := user.Current()
		if err != nil {
			return p, err
		}
		p = filepath.Join(u.HomeDir, strings.TrimPrefix(p, "~"))
	}
	return filepath.Clean(p), nil
}

func readCSV(path string) ([][]string, error) {
	full, err := ExpandPath(path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	r.LazyQuotes = true
	return r.ReadAll()
}

func normHeader(h string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(h) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '#' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func matchCol(header []string, keys ...string) int {
	norm := make([]string, len(header))
	for i, h := range header {
		norm[i] = normHeader(h)
	}
	for _, k := range keys {
		for i, n := range norm {
			if n == k {
				return i
			}
		}
	}
	for _, k := range keys {
		for i, n := range norm {
			if n != "" && strings.Contains(n, k) {
				return i
			}
		}
	}
	return -1
}

func field(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func rowEmpty(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func atoi(s string) int {
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}
