package printstruct

import (
	"bytes"
	"strings"
	"testing"
	"time"
	"github.com/wow-look-at-my/testify/assert"
	"github.com/wow-look-at-my/testify/require"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		in	int64
		want	string
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
		got := formatBytes(tt.in)
		assert.Equal(t, tt.want, got)

	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		in	time.Duration
		want	string
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
		got := formatDuration(tt.in)
		assert.Equal(t, tt.want, got)

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
		got := prettify(tt.in)
		assert.Equal(t, tt.want, got)

	}
}

// processFixture mirrors the example struct from the spec, plus an
// explicitly-skipped field via `json:"-"`. The Namespace field has a json
// tag but no label tag, exercising the json-tag fallback.
type processFixture struct {
	Name	string	`json:"name" label:"Name"`
	PM2Env	struct {
		Namespace	string	`json:"namespace"`
		Status		string	`json:"status" label:"Status"`
		PMUptime	int64	`json:"pm_uptime" label:"Uptime" fmt:"duration"`
		RestartTime	int	`json:"restart_time" label:"Restarts"`
	}	`json:"pm2_env"`
	Monit	struct {
		Memory int64 `json:"memory" label:"Memory" fmt:"bytes"`
	}	`json:"monit"`
	Internal	string	`json:"-"`	// explicitly skipped
}

func newFixture(name, status string, restarts int, mem int64, uptime time.Duration) processFixture {
	var p processFixture
	p.Name = name
	p.PM2Env.Namespace = "default"
	p.PM2Env.Status = status
	p.PM2Env.PMUptime = time.Now().Add(-uptime).UnixMilli()
	p.PM2Env.RestartTime = restarts
	p.Monit.Memory = mem
	p.Internal = "should not appear"
	return p
}

func TestPrintStruct_Detail(t *testing.T) {
	p := newFixture("caddy", "online", 0, 53687091, 2*time.Hour+34*time.Minute+12*time.Second)

	var buf bytes.Buffer
	require.NoError(t, PrintStruct(&buf, p))

	got := buf.String()

	// Uptime uses time.Since so we mask its value before comparison.
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	require.Equal(t, 6, len(lines))

	assert.True(t, strings.HasPrefix(lines[3], "Uptime:     "))

	lines[3] = "Uptime:     <UPTIME>"

	want := "Name:       caddy\n" +
		"Namespace:  default\n" +
		"Status:     online\n" +
		"Uptime:     <UPTIME>\n" +
		"Restarts:   0\n" +
		"Memory:     51.2M\n"
	cleaned := strings.Join(lines, "\n") + "\n"
	assert.Equal(t, want, cleaned)

	assert.NotContains(t, got, "should not appear")

}

func TestPrintStruct_Table(t *testing.T) {
	procs := []processFixture{
		newFixture("caddy", "online", 0, 53687091, 2*time.Hour+34*time.Minute+12*time.Second),
		newFixture("restfox", "online", 1, 29779558, 1*time.Hour+12*time.Minute+3*time.Second),
	}

	var buf bytes.Buffer
	require.NoError(t, PrintStruct(&buf, procs))

	got := buf.String()

	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	require.Equal(t, 3, len(lines))

	wantHeader := "Name     Namespace  Status  Uptime    Restarts  Memory"
	assert.Equal(t, wantHeader, lines[0])

	for i, row := range lines[1:] {
		cells := splitCells(row)
		assert.Equal(t, 6, len(cells))

		want := procs[i]
		assert.Equal(t, want.Name, cells[0])

		assert.Equal(t, "default", cells[1])

		assert.Equal(t, "online", cells[2])

		assert.True(t, looksLikeDuration(cells[3]))

		switch i {
		case 0:
			assert.Equal(t, "0", cells[4])

			assert.Equal(t, "51.2M", cells[5])

		case 1:
			assert.Equal(t, "1", cells[4])

			assert.Equal(t, "28.4M", cells[5])

		}
	}

	assert.NotContains(t, got, "should not appear")

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
		FullName	string	// no tags at all -> "FullName"
		Total		int	`json:"total_count"`	// json tag -> "Total Count"
	}
	v := fallback{FullName: "ada", Total: 42}
	var buf bytes.Buffer
	require.NoError(t, PrintStruct(&buf, v))

	want := "FullName:     ada\n" +
		"Total Count:  42\n"
	got := buf.String()
	assert.Equal(t, want, got)

}

func TestPrintStruct_PointerToStruct(t *testing.T) {
	p := newFixture("caddy", "online", 0, 1024, time.Minute)
	var buf bytes.Buffer
	require.NoError(t, PrintStruct(&buf, &p))

	assert.Contains(t, buf.String(), "Name:")

}

func TestPrintStruct_PointerToSlice(t *testing.T) {
	procs := []processFixture{newFixture("caddy", "online", 0, 1024, time.Minute)}
	var buf bytes.Buffer
	require.NoError(t, PrintStruct(&buf, &procs))

	assert.True(t, strings.HasPrefix(buf.String(), "Name"))

}

func TestPrintStruct_EmptySlice(t *testing.T) {
	var procs []processFixture
	var buf bytes.Buffer
	require.NoError(t, PrintStruct(&buf, procs))

	out := strings.TrimRight(buf.String(), "\n")
	want := "Name  Namespace  Status  Uptime  Restarts  Memory"
	assert.Equal(t, want, out)

}

func TestPrintStruct_UnsupportedType(t *testing.T) {
	var buf bytes.Buffer
	err := PrintStruct(&buf, 42)
	require.NotNil(t, err)

}

func TestPrintStruct_NilPointer(t *testing.T) {
	var p *processFixture
	var buf bytes.Buffer
	require.NoError(t, PrintStruct(&buf, p))

	assert.Equal(t, 0, buf.Len())

}

func TestPrintStruct_NestedFlattening(t *testing.T) {
	type deepest struct {
		Value string `label:"Deep"`
	}
	type middle struct {
		Deepest	deepest
		Other	string	`label:"Other"`
	}
	type top struct {
		Top	string	`label:"Top"`
		Middle	middle
	}
	v := top{
		Top:	"t",
		Middle:	middle{Deepest: deepest{Value: "d"}, Other: "o"},
	}
	var buf bytes.Buffer
	require.NoError(t, PrintStruct(&buf, v))

	want := "Top:    t\n" +
		"Deep:   d\n" +
		"Other:  o\n"
	got := buf.String()
	assert.Equal(t, want, got)

}

func TestPrintStruct_DurationIntegration(t *testing.T) {
	type s struct {
		Started int64 `label:"Uptime" fmt:"duration"`
	}
	v := s{Started: time.Now().Add(-5 * time.Second).UnixMilli()}
	var buf bytes.Buffer
	require.NoError(t, PrintStruct(&buf, v))

	out := strings.TrimSpace(buf.String())
	assert.True(t, strings.HasSuffix(out, "s"))

}
