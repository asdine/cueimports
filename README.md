# cueimports

cueimports is a [CUE](https://github.com/cue-lang/cue) tool that updates your import lines, adding missing ones and removing unused ones.

It scans through:

- your local packages
- the cue.mod directory packages
- the standard library packages

## Install

```bash
go install github.com/asdine/cueimports/cmd/cueimports
```

## Usage

```bash
$ echo "data: json.Marshal({a: math.Sqrt(7)})" | cueimports
```

```
package hello

import (
	"encoding/json"
	"math"
)

data: json.Marshal({a: math.Sqrt(7)})
```
