// Package printstruct prints structs and slices of structs in a human-readable
// format using struct tags for display names.
//
// Display name precedence (first match wins):
//  1. `label:"Display Name"`  used verbatim.
//  2. `json:"snake_case"`     prettified (underscores -> spaces, words
//     title-cased), e.g. `pm_uptime` -> "Pm Uptime".
//  3. The Go field name        prettified the same way.
//
// A `json:"-"` tag explicitly skips the field. Unexported fields are always
// skipped. Struct fields without a `label` tag are recursed into so nested
// leaves are flattened into the output (a struct field carrying a `label`
// tag is rendered as a single leaf via fmt.Sprintf).
//
// Optional value formatting via the `fmt` tag:
//   - fmt:"duration"  int64 as Unix millis, prints time since (e.g. 2h34m12s).
//   - fmt:"bytes"     int64 as bytes, prints e.g. 51.2M (1024-based).
package printstruct

import (
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"
)

// PrintStruct writes a human-readable rendering of v to w. If v is a slice (or
// array) of structs, output is a column-aligned table. If v is a single
// struct, output is a key-value list with aligned colons.
func PrintStruct(w io.Writer, v any) error {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		return printTable(w, rv)
	case reflect.Struct:
		return printDetail(w, rv)
	default:
		return fmt.Errorf("printstruct: unsupported kind %s (need struct or slice of structs)", rv.Kind())
	}
}

type field struct {
	label string
	value string
}

// extractLabels walks rv (which must be a struct) and produces the ordered
// list of (label, formatted-value) pairs by flattening nested structs.
func extractLabels(rv reflect.Value) []field {
	var out []field
	collect(rv, &out)
	return out
}

func collect(rv reflect.Value, out *[]field) {
	if !rv.IsValid() {
		return
	}
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return
	}
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		sf := rt.Field(i)
		if !sf.IsExported() {
			continue
		}
		fv := rv.Field(i)

		if labelTag, ok := sf.Tag.Lookup("label"); ok {
			*out = append(*out, field{
				label: labelTag,
				value: formatValue(fv, sf.Tag.Get("fmt")),
			})
			continue
		}

		// No explicit label. Struct fields are flattened (recursed) instead
		// of being rendered as a single value. Look through pointers and
		// interfaces to detect a struct.
		effective := fv
		for effective.Kind() == reflect.Pointer || effective.Kind() == reflect.Interface {
			if effective.IsNil() {
				break
			}
			effective = effective.Elem()
		}
		if effective.Kind() == reflect.Struct {
			collect(effective, out)
			continue
		}

		label, ok := derivedLabel(sf)
		if !ok {
			continue
		}
		*out = append(*out, field{
			label: label,
			value: formatValue(fv, sf.Tag.Get("fmt")),
		})
	}
}

// derivedLabel returns the fallback label for a field that has no `label`
// tag. It returns ok=false when the json tag is `-`, which means the field
// must be skipped entirely.
func derivedLabel(sf reflect.StructField) (string, bool) {
	if jsonTag, ok := sf.Tag.Lookup("json"); ok {
		name, _, _ := strings.Cut(jsonTag, ",")
		if name == "-" {
			return "", false
		}
		if name != "" {
			return prettify(name), true
		}
	}
	return prettify(sf.Name), true
}

// prettify converts a snake_case (or already-pretty) name to a display
// string: underscores become spaces, and the first letter of each
// space-separated word is upper-cased. Other characters are left untouched
// so existing capitalization (e.g. "URL") is preserved.
func prettify(s string) string {
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "_", " ")
	runes := []rune(s)
	capNext := true
	for i, r := range runes {
		if r == ' ' {
			capNext = true
			continue
		}
		if capNext && r >= 'a' && r <= 'z' {
			runes[i] = r - ('a' - 'A')
		}
		capNext = false
	}
	return string(runes)
}

// formatValue renders one leaf value, honoring the `fmt` tag when present.
func formatValue(rv reflect.Value, fmtTag string) string {
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return ""
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return ""
	}

	switch fmtTag {
	case "duration":
		if isInt(rv.Kind()) {
			return formatDuration(time.Since(time.UnixMilli(rv.Int())))
		}
	case "bytes":
		if isInt(rv.Kind()) {
			return formatBytes(rv.Int())
		}
		if isUint(rv.Kind()) {
			return formatBytes(int64(rv.Uint()))
		}
	}

	return fmt.Sprintf("%v", rv.Interface())
}

func isInt(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	}
	return false
}

func isUint(k reflect.Kind) bool {
	switch k {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	}
	return false
}

// formatDuration renders d (truncated to seconds) using the largest
// non-zero component as the leading unit, with smaller components
// zero-padded to 2 digits, e.g. 1h12m03s.
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	d = d.Truncate(time.Second)
	h := int64(d / time.Hour)
	d -= time.Duration(h) * time.Hour
	m := int64(d / time.Minute)
	d -= time.Duration(m) * time.Minute
	s := int64(d / time.Second)
	switch {
	case h > 0:
		return fmt.Sprintf("%dh%02dm%02ds", h, m, s)
	case m > 0:
		return fmt.Sprintf("%dm%02ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

// formatBytes renders n in IEC units (1024-based) with one decimal place,
// e.g. 51.2M. Values below 1 KiB are shown as raw bytes (e.g. 512B).
func formatBytes(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	const unit = 1024
	if n < unit {
		if neg {
			return fmt.Sprintf("-%dB", n)
		}
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	letters := "KMGTPE"
	if exp >= len(letters) {
		exp = len(letters) - 1
	}
	sign := ""
	if neg {
		sign = "-"
	}
	return fmt.Sprintf("%s%.1f%c", sign, float64(n)/float64(div), letters[exp])
}

func printDetail(w io.Writer, rv reflect.Value) error {
	fields := extractLabels(rv)
	maxLabel := 0
	for _, f := range fields {
		if len(f.label) > maxLabel {
			maxLabel = len(f.label)
		}
	}
	for _, f := range fields {
		labelCol := f.label + ":"
		// Pad so the colon column lines up at maxLabel+1, then 2 spaces, then value.
		pad := strings.Repeat(" ", maxLabel+1-len(labelCol))
		if _, err := fmt.Fprintf(w, "%s%s  %s\n", labelCol, pad, f.value); err != nil {
			return err
		}
	}
	return nil
}

func printTable(w io.Writer, rv reflect.Value) error {
	// Discover the column headers from the slice's element type so we can
	// print headers even when the slice is empty.
	et := rv.Type().Elem()
	for et.Kind() == reflect.Pointer {
		et = et.Elem()
	}
	if et.Kind() != reflect.Struct {
		return fmt.Errorf("printstruct: slice element kind %s is not a struct", et.Kind())
	}
	headers := headerLabels(et)
	if len(headers) == 0 {
		return nil
	}

	rows := make([][]string, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		elem := rv.Index(i)
		for elem.Kind() == reflect.Pointer || elem.Kind() == reflect.Interface {
			if elem.IsNil() {
				break
			}
			elem = elem.Elem()
		}
		if elem.Kind() != reflect.Struct {
			continue
		}
		fields := extractLabels(elem)
		row := make([]string, len(headers))
		for i, f := range fields {
			if i < len(row) {
				row[i] = f.value
			}
		}
		rows = append(rows, row)
	}

	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, v := range row {
			if i < len(widths) && len(v) > widths[i] {
				widths[i] = len(v)
			}
		}
	}

	writeRow := func(cells []string) error {
		for i, c := range cells {
			if i == len(cells)-1 {
				if _, err := fmt.Fprint(w, c); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(w, "%-*s  ", widths[i], c); err != nil {
					return err
				}
			}
		}
		_, err := fmt.Fprintln(w)
		return err
	}

	if err := writeRow(headers); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writeRow(row); err != nil {
			return err
		}
	}
	return nil
}

// headerLabels returns the ordered list of `label` tag values reachable by
// flattening nested structs in t.
func headerLabels(t reflect.Type) []string {
	var labels []string
	collectHeaders(t, &labels)
	return labels
}

func collectHeaders(t reflect.Type, out *[]string) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		if labelTag, ok := sf.Tag.Lookup("label"); ok {
			*out = append(*out, labelTag)
			continue
		}
		ft := sf.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct {
			collectHeaders(ft, out)
			continue
		}
		label, ok := derivedLabel(sf)
		if !ok {
			continue
		}
		*out = append(*out, label)
	}
}
