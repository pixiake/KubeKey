table
=====

[![Build Status](https://travis-ci.org/modood/table.png)](https://travis-ci.org/modood/table)
[![Coverage Status](https://coveralls.io/repos/github/modood/table/badge.svg?branch=master)](https://coveralls.io/github/modood/table?branch=master)
[![GoDoc](https://godoc.org/github.com/modood/table?status.svg)](http://godoc.org/github.com/modood/table)

Produces a string that represents slice of structs data in a text table, inspired by gajus/table.

**Features:**

-   No dependency.
-   Cell content aligned.
-   Column width self-adaptation
-   Support type of struct field: int, float, string, bool, slice, struct, map, time.Time and everything.
-   Support custom table header by declaring optional tag: `table`.(Thanks @skyfireitdiy)

Installation
------------

```
$ go get github.com/modood/table
```

Quick start
-----------

```go
package main

import (
	"github.com/modood/table"
)

type House struct {
	Name  string `table:"Name"`
	Sigil string
	Motto string
}

func main() {
	hs := []House{
		{"Stark", "direwolf", "Winter is coming"},
		{"Targaryen", "dragon", "Fire and Blood"},
		{"Lannister", "lion", "Hear Me Roar"},
	}

	// Output to stdout
	table.Output(hs)

	// Or just return table string and then do something
	s := table.Table(hs)
	_ = s
}
```

output:
```
┌───────────┬──────────┬──────────────────┐
│ Name      │ Sigil    │ Motto            │
├───────────┼──────────┼──────────────────┤
│ Stark     │ direwolf │ Winter is coming │
│ Targaryen │ dragon   │ Fire and Blood   │
│ Lannister │ lion     │ Hear Me Roar     │
└───────────┴──────────┴──────────────────┘
```

Document
--------

-   `func Output(slice interface{})`

    formats slice of structs data and writes to standard output.(Using box drawing characters)

-   `func OutputA(slice interface{})`

    formats slice of structs data and writes to standard output.(Using standard ascii characters)

-   `func Table(slice interface{}) string`

    formats slice of structs data and returns the resulting string.(Using box drawing characters)

-   `func AsciiTable(slice interface{}) string`

    formats slice of structs data and returns the resulting string.(Using standard ascii characters)

-   compare [box drawing characters](http://unicode.org/charts/PDF/U2500.pdf) with [standard ascii characters](https://ascii.cl/)

    box drawing:
    ```
    ┌───────────┬──────────┬──────────────────┐
    │ Name      │ Sigil    │ Motto            │
    ├───────────┼──────────┼──────────────────┤
    │ Stark     │ direwolf │ Winter is coming │
    │ Targaryen │ dragon   │ Fire and Blood   │
    │ Lannister │ lion     │ Hear Me Roar     │
    └───────────┴──────────┴──────────────────┘
    ```

    standard ascii:

    ```
    +-----------+----------+------------------+
    | Name      | Sigil    | Motto            |
    +-----------+----------+------------------+
    | Stark     | direwolf | Winter is coming |
    | Targaryen | dragon   | Fire and Blood   |
    | Lannister | lion     | Hear Me Roar     |
    +-----------+----------+------------------+
    ```


Contributing
------------

1.  Fork it
2.  Create your feature branch (`git checkout -b my-new-feature`)
3.  Commit your changes (`git commit -am 'Add some feature'`)
4.  Push to the branch (`git push origin my-new-feature`)
5.  Create new Pull Request

License
-------

this repo is released under the [MIT License](http://www.opensource.org/licenses/MIT).
