# go-utils

Small Go utilities, organized as one package per util.

## Packages

### `printstruct`

Pretty-print structs (or slices of structs) using struct tags for column /
label names.

```go
import "github.com/wow-look-at-my/go-utils/printstruct"

type Process struct {
    Name   string `json:"name" label:"Name"`
    PM2Env struct {
        Status   string `json:"status" label:"Status"`
        PMUptime int64  `json:"pm_uptime" label:"Uptime" fmt:"duration"`
    } `json:"pm2_env"`
    Monit struct {
        Memory int64 `json:"memory" label:"Memory" fmt:"bytes"`
    } `json:"monit"`
}

// Single struct -> aligned key/value list.
printstruct.PrintStruct(os.Stdout, p)

// Slice of structs -> aligned table.
printstruct.PrintStruct(os.Stdout, ps)
```

#### Tags

| Tag                  | Effect                                                        |
| -------------------- | ------------------------------------------------------------- |
| `label:"Foo"`        | Display name for the field.                                   |
| `fmt:"duration"`     | Treat `int64` as Unix millis, render as time-since duration.  |
| `fmt:"bytes"`        | Treat `int64` as bytes, render as `51.2M` (1024-based).       |
| `json:"foo_bar"`     | Used when no `label` is present, prettified to `"Foo Bar"`.   |
| `json:"-"`           | Field is skipped entirely.                                    |

If neither `label` nor `json` is present, the Go field name is prettified
(`"FullName"` stays `"FullName"` since there are no underscores to expand).

Untagged struct fields are recursed into so nested leaves flatten into one
flat row / list.

## Development

CI uses [`wow-look-at-my/go-toolchain`](https://github.com/wow-look-at-my/go-toolchain).
Run `go-toolchain` locally to mod-tidy, vet, test (with coverage), and build.
