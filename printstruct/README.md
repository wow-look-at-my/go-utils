# printstruct

Pretty-print Go structs and slices of structs in a human-readable format using
struct tags for display names.

## Installation

```
go get github.com/wow-look-at-my/go-utils/printstruct
```

## Usage

```go
import (
    "os"
    "github.com/wow-look-at-my/go-utils/printstruct"
)

type Process struct {
    Name   string `json:"name" label:"Name"`
    PM2Env struct {
        Namespace string `json:"namespace"`
        Status    string `json:"status" label:"Status"`
        PMUptime  int64  `json:"pm_uptime" label:"Uptime" fmt:"duration"`
        Restarts  int    `json:"restart_time" label:"Restarts"`
    } `json:"pm2_env"`
    Monit struct {
        Memory int64 `json:"memory" label:"Memory" fmt:"bytes"`
    } `json:"monit"`
}

// Single struct → aligned key/value list.
printstruct.PrintStruct(os.Stdout, p)

// Slice of structs → aligned table.
printstruct.PrintStruct(os.Stdout, ps)
```

### Single struct output

```
Name:       caddy
Namespace:  default
Status:     online
Uptime:     2h34m12s
Restarts:   0
Memory:     51.2M
```

### Slice of structs output

```
Name     Namespace  Status  Uptime    Restarts  Memory
caddy    default    online  2h34m12s  0         51.2M
restfox  default    online  1h12m03s  1         28.4M
```

## Struct tags

| Tag              | Effect                                                       |
| ---------------- | ------------------------------------------------------------ |
| `label:"Foo"`    | Display name used verbatim.                                  |
| `json:"foo_bar"` | Fallback when no `label` is set; prettified to `"Foo Bar"`.  |
| `json:"-"`       | Field is skipped entirely.                                   |
| `fmt:"duration"` | Render `int64` as Unix millis → time-since (e.g. `2h34m12s`). |
| `fmt:"bytes"`    | Render `int64`/`uint64` as IEC bytes (e.g. `51.2M`).        |

### Label resolution order

1. `label:"…"` tag — used verbatim.
2. `json:"…"` tag — prettified (underscores → spaces, each word title-cased),
   e.g. `pm_uptime` → `"Pm Uptime"`.
3. Go field name — prettified the same way.

### Nested struct flattening

Struct fields **without** a `label` tag are recursed into so their leaf fields
are flattened into the output. A struct field that carries a `label` tag is
treated as a single leaf and rendered via `fmt.Sprintf`.

```go
type Top struct {
    Title  string `label:"Title"`
    Nested struct {
        Deep string `label:"Deep"`
    }
}
```

```
Title:  hello
Deep:   world
```

## API

```go
func PrintStruct(w io.Writer, v any) error
```

Writes a human-readable rendering of `v` to `w`.

- If `v` is a **struct** (or pointer to one), output is a key-value list with
  aligned colons.
- If `v` is a **slice or array** of structs (or pointers to structs), output is
  a column-aligned table with a header row derived from the element type.
- Any other kind returns an error.

Unexported fields are always skipped. A `nil` pointer input produces no output
and no error.
