package printstruct

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0B"},
		{1, "1B"},
		{512, "512B"},
		{1023, "1023B"},
		{1024, "1.0K"},
		{1536, "1.5K"},
		{1024 * 1024, "1.0M"},
		{53687091, "51.2M"},
		{29779558, "28.4M"},
		{1024 * 1024 * 1024, "1.0G"},
		{int64(1024) * 1024 * 1024 * 1024, "1.0T"},
		{-1024, "-1.0K"},
		{-256, "-256B"},
	}
	for _, tt := range tests {
		if got := formatBytes(tt.in); got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{0, "0s"},
		{30 * time.Second, "30s"},
		{1 * time.Minute, "1m00s"},
		{time.Minute + 5*time.Second, "1m05s"},
		{time.Hour + 12*time.Minute + 3*time.Second, "1h12m03s"},
		{2*time.Hour + 34*time.Minute + 12*time.Second, "2h34m12s"},
		// sub-second portion truncated.
		{time.Second + 999*time.Millisecond, "1s"},
		// negative magnitude is rendered as a positive duration.
		{-(time.Hour + time.Minute), "1h01m00s"},
	}
	for _, tt := range tests {
		if got := formatDuration(tt.in); got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPrettify(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"name", "Name"},
		{"Name", "Name"},
		{"pm_uptime", "Pm Uptime"},
		{"restart_time", "Restart Time"},
		{"http_status_code", "Http Status Code"},
		{"URL", "URL"},
		{"Already Pretty", "Already Pretty"},
		{"snake_with_extra__underscore", "Snake With Extra  Underscore"},
	}
	for _, tt := range tests {
		if got := prettify(tt.in); got != tt.want {
			t.Errorf("prettify(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// processFixture mirrors the example struct from the spec, plus an
// explicitly-skipped field via `json:"-"`. The Namespace field has a json
// tag but no label tag, exercising the json-tag fallback.
type processFixture struct {
	Name   string `json:"name" label:"Name"`
	PM2Env struct {
		Namespace   string `json:"namespace"`
		Status      string `json:"status" label:"Status"`
		PMUptime    int64  `json:"pm_uptime" label:"Uptime" fmt:"duration"`
		RestartTime int    `json:"restart_time" label:"Restarts"`
	} `json:"pm2_env"`
	Monit struct {
		Memory int64 `json:"memory" label:"Memory" fmt:"bytes"`
	} `json:"monit"`
	Internal string `json:"-"` // explicitly skipped
}

func newFixture(name, status string, restarts int, mem int64) processFixture {
	var p processFixture
	p.Name = name
	p.PM2Env.Namespace = "default"
	p.PM2Env.Status = status
	p.PM2Env.RestartTime = restarts
	p.Monit.Memory = mem
	p.Internal = "should not appear"
	return p
}

func TestPrintStruct_Detail(t *testing.T) {
	p := newFixture("caddy", "online", 0, 53687091)

	var buf bytes.Buffer
	if err := PrintStruct(&buf, p); err != nil {
		t.Fatal(err)
	}
	got := buf.String()

	// Uptime uses time.Since so we mask its value before comparison.
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 6 {
		t.Fatalf("want 6 lines, got %d:\n%s", len(lines), got)
	}
	if !strings.HasPrefix(lines[3], "Uptime:     ") {
		t.Errorf("uptime line %q is not aligned correctly", lines[3])
	}
	lines[3] = "Uptime:     <UPTIME>"

	want := "Name:       caddy\n" +
		"Namespace:  default\n" +
		"Status:     online\n" +
		"Uptime:     <UPTIME>\n" +
		"Restarts:   0\n" +
		"Memory:     51.2M\n"
	if cleaned := strings.Join(lines, "\n") + "\n"; cleaned != want {
		t.Errorf("detail output mismatch:\ngot:\n%s\nwant:\n%s", cleaned, want)
	}

	if strings.Contains(got, "should not appear") {
		t.Errorf("Internal field leaked into output: %s", got)
	}
}

func TestPrintStruct_Table(t *testing.T) {
	procs := []processFixture{
		newFixture("caddy", "online", 0, 53687091),
		newFixture("restfox", "online", 1, 29779558),
	}

	var buf bytes.Buffer
	if err := PrintStruct(&buf, procs); err != nil {
		t.Fatal(err)
	}
	got := buf.String()

	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 lines (header + 2 rows), got %d:\n%s", len(lines), got)
	}

	wantHeader := "Name     Namespace  Status  Uptime    Restarts  Memory"
	if lines[0] != wantHeader {
		t.Errorf("header mismatch:\ngot:  %q\nwant: %q", lines[0], wantHeader)
	}

	for i, row := range lines[1:] {
		cells := splitCells(row)
		if len(cells) != 6 {
			t.Errorf("row %d: want 6 cells, got %d (%q)", i, len(cells), row)
			continue
		}
		want := procs[i]
		if cells[0] != want.Name {
			t.Errorf("row %d Name = %q, want %q", i, cells[0], want.Name)
		}
		if cells[1] != "default" {
			t.Errorf("row %d Namespace = %q, want %q", i, cells[1], "default")
		}
		if cells[2] != "online" {
			t.Errorf("row %d Status = %q", i, cells[2])
		}
		if !looksLikeDuration(cells[3]) {
			t.Errorf("row %d Uptime = %q, does not look like a duration", i, cells[3])
		}
		switch i {
		case 0:
			if cells[4] != "0" {
				t.Errorf("row %d Restarts = %q", i, cells[4])
			}
			if cells[5] != "51.2M" {
				t.Errorf("row %d Memory = %q", i, cells[5])
			}
		case 1:
			if cells[4] != "1" {
				t.Errorf("row %d Restarts = %q", i, cells[4])
			}
			if cells[5] != "28.4M" {
				t.Errorf("row %d Memory = %q", i, cells[5])
			}
		}
	}

	if strings.Contains(got, "should not appear") {
		t.Errorf("Internal field leaked into output: %s", got)
	}
}

// splitCells splits a table row on runs of 2+ spaces to recover cell values.
func splitCells(s string) []string {
	var out []string
	for _, c := range strings.Split(s, "  ") {
		c = strings.TrimSpace(c)
		if c != "" {
			out = append(out, c)
		}
	}
	return out
}

func looksLikeDuration(s string) bool {
	if !strings.HasSuffix(s, "s") {
		return false
	}
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

func TestPrintStruct_FallbackToFieldName(t *testing.T) {
	type fallback struct {
		FullName string // no tags at all -> "FullName"
		Total    int    `json:"total_count"` // json tag -> "Total Count"
	}
	v := fallback{FullName: "ada", Total: 42}
	var buf bytes.Buffer
	if err := PrintStruct(&buf, v); err != nil {
		t.Fatal(err)
	}
	want := "FullName:     ada\n" +
		"Total Count:  42\n"
	if got := buf.String(); got != want {
		t.Errorf("fallback output mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestPrintStruct_PointerToStruct(t *testing.T) {
	p := newFixture("caddy", "online", 0, 1024)
	var buf bytes.Buffer
	if err := PrintStruct(&buf, &p); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Name:") {
		t.Errorf("expected detail output for pointer to struct, got:\n%s", buf.String())
	}
}

func TestPrintStruct_PointerToSlice(t *testing.T) {
	procs := []processFixture{newFixture("caddy", "online", 0, 1024)}
	var buf bytes.Buffer
	if err := PrintStruct(&buf, &procs); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), "Name") {
		t.Errorf("expected table header, got:\n%s", buf.String())
	}
}

func TestPrintStruct_EmptySlice(t *testing.T) {
	var procs []processFixture
	var buf bytes.Buffer
	if err := PrintStruct(&buf, procs); err != nil {
		t.Fatal(err)
	}
	out := strings.TrimRight(buf.String(), "\n")
	want := "Name  Namespace  Status  Uptime  Restarts  Memory"
	if out != want {
		t.Errorf("empty slice header mismatch:\ngot:  %q\nwant: %q", out, want)
	}
}

func TestPrintStruct_UnsupportedType(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintStruct(&buf, 42); err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestPrintStruct_NilPointer(t *testing.T) {
	var p *processFixture
	var buf bytes.Buffer
	if err := PrintStruct(&buf, p); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output for nil pointer, got: %q", buf.String())
	}
}

func TestPrintStruct_NestedFlattening(t *testing.T) {
	type deepest struct {
		Value string `label:"Deep"`
	}
	type middle struct {
		Deepest deepest
		Other   string `label:"Other"`
	}
	type top struct {
		Top    string `label:"Top"`
		Middle middle
	}
	v := top{
		Top:    "t",
		Middle: middle{Deepest: deepest{Value: "d"}, Other: "o"},
	}
	var buf bytes.Buffer
	if err := PrintStruct(&buf, v); err != nil {
		t.Fatal(err)
	}
	want := "Top:    t\n" +
		"Deep:   d\n" +
		"Other:  o\n"
	if got := buf.String(); got != want {
		t.Errorf("nested flattening mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestPrintStruct_DurationIntegration(t *testing.T) {
	type s struct {
		Started int64 `label:"Uptime" fmt:"duration"`
	}
	v := s{Started: time.Now().Add(-5 * time.Second).UnixMilli()}
	var buf bytes.Buffer
	if err := PrintStruct(&buf, v); err != nil {
		t.Fatal(err)
	}
	out := strings.TrimSpace(buf.String())
	if !strings.HasSuffix(out, "s") {
		t.Errorf("expected duration-formatted output, got: %q", out)
	}
}
