# Test Fixtures

## sample-go

Located at `/testdata/sample-go/`.

A small Go module with:

- `calc/add.go` — `Add(a, b int) int` (covered)
- `calc/sub.go` — `Sub(a, b int) int` (covered)
- `calc/mul.go` — `Mul(a, b int) int` (uncovered)
- `calc/add_test.go` — table test for `Add` with golden files
- `calc/sub_test.go` — simple test for `Sub`

### Expected Symbols

| Qualified Name | File | Covered | CoveredBy |
|---|---|---|---|
| calc.Add | calc/add.go | true | go:calc/add_test.go:calc.TestAdd |
| calc.Sub | calc/sub.go | true | go:calc/sub_test.go:calc.TestSub |
| calc.Mul | calc/mul.go | false | — |

### Expected Tests

| ID | Name | File |
|---|---|---|
| go:calc/add_test.go:calc.TestAdd | TestAdd | calc/add_test.go |
| go:calc/sub_test.go:calc.TestSub | TestSub | calc/sub_test.go |

### Expected Golden Cases (TestAdd)

- `positive` — `positive.in.json` + `positive.out.json`
- `negative` — `negative.in.json` + `negative.out.json`

## sample-python

Located at `/testdata/sample-python/`.

A small pytest project mirroring the Go fixture:

- `calc/add.py` — `add(a, b)` (covered)
- `calc/sub.py` — `sub(a, b)` (covered)
- `calc/mul.py` — `mul(a, b)` (uncovered)
- `test_add.py` — parametrized test for `add` with golden files
- `calc_test.py` — simple test for `sub`

### Expected Golden Cases (test_add)

- `positive` — `positive.in.json` + `positive.out.json`
- `negative` — `negative.in.json` + `negative.out.json`
