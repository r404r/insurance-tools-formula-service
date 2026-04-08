# Task #018 LaTeX Formula Input Test Report

**Generated**: 2026-04-08 19:23:55
**Test Cases**: 20
**Calculation Tests**: 200/200 passed (100.0%)
**Conversion Preview Verified**: 20/20
**Fully Passed Cases**: 20/20

---

## Summary Table

| ID | Description | Conversion | Calc Pass | Errors |
|----|-------------|:----------:|:---------:|--------|
| TC-01 | Addition: \mathrm identifiers + underscore variable names | ✅ | ✅ 10/10 | — |
| TC-02 | Subtraction: premium - discount | ✅ | ✅ 10/10 | — |
| TC-03 | Multiplication: \cdot → * | ✅ | ✅ 10/10 | — |
| TC-04 | Multiplication: \times → * | ✅ | ✅ 10/10 | — |
| TC-05 | Division: \frac{annual}{12} | ✅ | ✅ 10/10 | — |
| TC-06 | Power: x^{2} | ✅ | ✅ 10/10 | — |
| TC-07 | Natural exponent: e^{x} → exp(x) | ✅ | ✅ 10/10 | — |
| TC-08 | Square root: \sqrt{x} | ✅ | ✅ 10/10 | — |
| TC-09 | Absolute value: \left|a - b\right| | ✅ | ✅ 10/10 | — |
| TC-10 | Natural log: \ln\left(x\right) | ✅ | ✅ 10/10 | — |
| TC-11 | Custom function: \operatorname{max} | ✅ | ✅ 10/10 | — |
| TC-12 | Min function: \operatorname{min} | ✅ | ✅ 10/10 | — |
| TC-13 | Parentheses: (a + b) * c | ✅ | ✅ 10/10 | — |
| TC-14 | Comparison: \ge in conditional | ✅ | ✅ 10/10 | — |
| TC-15 | Comparison: \le in conditional (discount for low risk) | ✅ | ✅ 10/10 | — |
| TC-16 | Comparison: \ne in conditional (bonus flag) | ✅ | ✅ 10/10 | — |
| TC-17 | Comparison: \geq (alternative form) in conditional | ✅ | ✅ 10/10 | — |
| TC-18 | Simple conditional: \begin{cases} | ✅ | ✅ 10/10 | — |
| TC-19 | Nested conditional: nested \begin{cases} | ✅ | ✅ 10/10 | — |
| TC-20 | Compound: \frac{sum_insured * rate}{1000} | ✅ | ✅ 10/10 | — |

---

## Detailed Results

### TC-01: Addition: \mathrm identifiers + underscore variable names — ✅ PASS

**LaTeX Input:**
```latex
\mathrm{age} + \mathrm{base\_rate}
```

**Expected Formula Text:** `age + base_rate`  
**Conversion Preview:** ✅ Verified  
**Formula ID:** bb65e441-5b01-4d06-9dae-9cf88d45fad4  
**Screenshot:** `tests/screenshots/018/tc-01-latex-panel.png`  

**Calculation Tests** (10/10 passed):

| # | Inputs | Expected | Actual | Pass |
|---|--------|----------|--------|:----:|
| 1 | `age=30, base_rate=0.05` | `30.05` | `30.05` | ✅ |
| 2 | `age=25, base_rate=0.03` | `25.03` | `25.03` | ✅ |
| 3 | `age=40, base_rate=0.1` | `40.1` | `40.1` | ✅ |
| 4 | `age=0, base_rate=0` | `0` | `0` | ✅ |
| 5 | `age=65, base_rate=0.15` | `65.15` | `65.15` | ✅ |
| 6 | `age=18, base_rate=0.02` | `18.02` | `18.02` | ✅ |
| 7 | `age=50, base_rate=0.08` | `50.08` | `50.08` | ✅ |
| 8 | `age=1, base_rate=0.001` | `1.001` | `1.001` | ✅ |
| 9 | `age=100, base_rate=0.5` | `100.5` | `100.5` | ✅ |
| 10 | `age=35, base_rate=0.06` | `35.06` | `35.06` | ✅ |

### TC-02: Subtraction: premium - discount — ✅ PASS

**LaTeX Input:**
```latex
\mathrm{premium} - \mathrm{discount}
```

**Expected Formula Text:** `premium - discount`  
**Conversion Preview:** ✅ Verified  
**Formula ID:** 878caefa-2716-46f9-9347-c762f9002b25  
**Screenshot:** `tests/screenshots/018/tc-02-latex-panel.png`  

**Calculation Tests** (10/10 passed):

| # | Inputs | Expected | Actual | Pass |
|---|--------|----------|--------|:----:|
| 1 | `premium=1000, discount=100` | `900` | `900` | ✅ |
| 2 | `premium=500, discount=50` | `450` | `450` | ✅ |
| 3 | `premium=2000, discount=0` | `2000` | `2000` | ✅ |
| 4 | `premium=750, discount=250` | `500` | `500` | ✅ |
| 5 | `premium=1500, discount=300` | `1200` | `1200` | ✅ |
| 6 | `premium=100, discount=100` | `0` | `0` | ✅ |
| 7 | `premium=3000, discount=450` | `2550` | `2550` | ✅ |
| 8 | `premium=800, discount=200` | `600` | `600` | ✅ |
| 9 | `premium=1200, discount=120` | `1080` | `1080` | ✅ |
| 10 | `premium=600, discount=60` | `540` | `540` | ✅ |

### TC-03: Multiplication: \cdot → * — ✅ PASS

**LaTeX Input:**
```latex
\mathrm{sum\_insured} \cdot \mathrm{rate}
```

**Expected Formula Text:** `sum_insured * rate`  
**Conversion Preview:** ✅ Verified  
**Formula ID:** c2bc8559-b997-41d6-8b54-009419fa2027  
**Screenshot:** `tests/screenshots/018/tc-03-latex-panel.png`  

**Calculation Tests** (10/10 passed):

| # | Inputs | Expected | Actual | Pass |
|---|--------|----------|--------|:----:|
| 1 | `sum_insured=100000, rate=0.005` | `500` | `500` | ✅ |
| 2 | `sum_insured=200000, rate=0.003` | `600` | `600` | ✅ |
| 3 | `sum_insured=50000, rate=0.01` | `500` | `500` | ✅ |
| 4 | `sum_insured=1000000, rate=0.002` | `2000` | `2000` | ✅ |
| 5 | `sum_insured=0, rate=0.005` | `0` | `0` | ✅ |
| 6 | `sum_insured=500000, rate=0.004` | `2000` | `2000` | ✅ |
| 7 | `sum_insured=75000, rate=0.006` | `450` | `450` | ✅ |
| 8 | `sum_insured=300000, rate=0.0025` | `750` | `750` | ✅ |
| 9 | `sum_insured=800000, rate=0.001` | `800` | `800` | ✅ |
| 10 | `sum_insured=150000, rate=0.007` | `1050` | `1050` | ✅ |

### TC-04: Multiplication: \times → * — ✅ PASS

**LaTeX Input:**
```latex
\mathrm{principal} \times \mathrm{factor}
```

**Expected Formula Text:** `principal * factor`  
**Conversion Preview:** ✅ Verified  
**Formula ID:** 455454d7-5581-4637-84db-71728b09f8cb  
**Screenshot:** `tests/screenshots/018/tc-04-latex-panel.png`  

**Calculation Tests** (10/10 passed):

| # | Inputs | Expected | Actual | Pass |
|---|--------|----------|--------|:----:|
| 1 | `principal=50000, factor=1.5` | `75000` | `75000` | ✅ |
| 2 | `principal=10000, factor=2` | `20000` | `20000` | ✅ |
| 3 | `principal=1000, factor=1` | `1000` | `1000` | ✅ |
| 4 | `principal=5000, factor=0.5` | `2500` | `2500` | ✅ |
| 5 | `principal=25000, factor=1.2` | `30000` | `30000` | ✅ |
| 6 | `principal=100000, factor=0.8` | `80000` | `80000` | ✅ |
| 7 | `principal=20000, factor=3` | `60000` | `60000` | ✅ |
| 8 | `principal=7500, factor=1.1` | `8250` | `8250` | ✅ |
| 9 | `principal=40000, factor=1.25` | `50000` | `50000` | ✅ |
| 10 | `principal=15000, factor=0.9` | `13500` | `13500` | ✅ |

### TC-05: Division: \frac{annual}{12} — ✅ PASS

**LaTeX Input:**
```latex
\frac{\mathrm{annual}}{12}
```

**Expected Formula Text:** `(annual) / (12)`  
**Conversion Preview:** ✅ Verified  
**Formula ID:** ebffc418-e54d-4846-9b46-92716b48bcdb  
**Screenshot:** `tests/screenshots/018/tc-05-latex-panel.png`  

**Calculation Tests** (10/10 passed):

| # | Inputs | Expected | Actual | Pass |
|---|--------|----------|--------|:----:|
| 1 | `annual=12000` | `1000` | `1000` | ✅ |
| 2 | `annual=24000` | `2000` | `2000` | ✅ |
| 3 | `annual=6000` | `500` | `500` | ✅ |
| 4 | `annual=0` | `0` | `0` | ✅ |
| 5 | `annual=3600` | `300` | `300` | ✅ |
| 6 | `annual=48000` | `4000` | `4000` | ✅ |
| 7 | `annual=1200` | `100` | `100` | ✅ |
| 8 | `annual=18000` | `1500` | `1500` | ✅ |
| 9 | `annual=9600` | `800` | `800` | ✅ |
| 10 | `annual=36000` | `3000` | `3000` | ✅ |

### TC-06: Power: x^{2} — ✅ PASS

**LaTeX Input:**
```latex
\mathrm{x}^{2}
```

**Expected Formula Text:** `x ^ (2)`  
**Conversion Preview:** ✅ Verified  
**Formula ID:** 1123016b-f36b-4e7f-8c5d-94ffff8405e1  
**Screenshot:** `tests/screenshots/018/tc-06-latex-panel.png`  

**Calculation Tests** (10/10 passed):

| # | Inputs | Expected | Actual | Pass |
|---|--------|----------|--------|:----:|
| 1 | `x=5` | `25` | `25` | ✅ |
| 2 | `x=10` | `100` | `100` | ✅ |
| 3 | `x=0` | `0` | `0` | ✅ |
| 4 | `x=1` | `1` | `1` | ✅ |
| 5 | `x=2` | `4` | `4` | ✅ |
| 6 | `x=3` | `9` | `9` | ✅ |
| 7 | `x=4` | `16` | `16` | ✅ |
| 8 | `x=7` | `49` | `49` | ✅ |
| 9 | `x=8` | `64` | `64` | ✅ |
| 10 | `x=12` | `144` | `144` | ✅ |

### TC-07: Natural exponent: e^{x} → exp(x) — ✅ PASS

**LaTeX Input:**
```latex
e^{\mathrm{r}}
```

**Expected Formula Text:** `exp(r)`  
**Conversion Preview:** ✅ Verified  
**Formula ID:** f4431f71-8230-40ad-b302-83a9aa493f4c  
**Screenshot:** `tests/screenshots/018/tc-07-latex-panel.png`  

**Calculation Tests** (10/10 passed):

| # | Inputs | Expected | Actual | Pass |
|---|--------|----------|--------|:----:|
| 1 | `r=0` | `1` | `1` | ✅ |
| 2 | `r=1` | `2.718281828459045235` | `2.718281828459045` | ✅ |
| 3 | `r=2` | `7.389056098930650227` | `7.38905609893065` | ✅ |
| 4 | `r=-1` | `0.367879441171442322` | `0.36787944117144233` | ✅ |
| 5 | `r=0.5` | `1.648721270700128147` | `1.6487212707001282` | ✅ |
| 6 | `r=3` | `20.085536923187667741` | `20.085536923187668` | ✅ |
| 7 | `r=0.1` | `1.105170918075647625` | `1.1051709180756477` | ✅ |
| 8 | `r=0.693147` | `1.999999819512220285` | `1.999999638880142` | ✅ |
| 9 | `r=-0.5` | `0.606530659712633424` | `0.6065306597126334` | ✅ |
| 10 | `r=1.5` | `4.481689070339594636` | `4.4816890703380645` | ✅ |

### TC-08: Square root: \sqrt{x} — ✅ PASS

**LaTeX Input:**
```latex
\sqrt{\mathrm{x}}
```

**Expected Formula Text:** `sqrt(x)`  
**Conversion Preview:** ✅ Verified  
**Formula ID:** e7212478-1060-45fe-8dd7-27f2b01f0c96  
**Screenshot:** `tests/screenshots/018/tc-08-latex-panel.png`  

**Calculation Tests** (10/10 passed):

| # | Inputs | Expected | Actual | Pass |
|---|--------|----------|--------|:----:|
| 1 | `x=4` | `2` | `2` | ✅ |
| 2 | `x=9` | `3` | `3` | ✅ |
| 3 | `x=16` | `4` | `4` | ✅ |
| 4 | `x=25` | `5` | `5` | ✅ |
| 5 | `x=0` | `0` | `0` | ✅ |
| 6 | `x=1` | `1` | `1` | ✅ |
| 7 | `x=100` | `10` | `10` | ✅ |
| 8 | `x=2` | `1.414213562373095049` | `1.4142135623730951` | ✅ |
| 9 | `x=3` | `1.732050808056887729` | `1.7320508075688772` | ✅ |
| 10 | `x=144` | `12` | `12` | ✅ |

### TC-09: Absolute value: \left|a - b\right| — ✅ PASS

**LaTeX Input:**
```latex
\left|\mathrm{a} - \mathrm{b}\right|
```

**Expected Formula Text:** `abs(a - b)`  
**Conversion Preview:** ✅ Verified  
**Formula ID:** 18e36230-9365-41fc-835e-392131eff175  
**Screenshot:** `tests/screenshots/018/tc-09-latex-panel.png`  

**Calculation Tests** (10/10 passed):

| # | Inputs | Expected | Actual | Pass |
|---|--------|----------|--------|:----:|
| 1 | `a=5, b=8` | `3` | `3` | ✅ |
| 2 | `a=10, b=3` | `7` | `7` | ✅ |
| 3 | `a=0, b=0` | `0` | `0` | ✅ |
| 4 | `a=-5, b=5` | `10` | `10` | ✅ |
| 5 | `a=100, b=100` | `0` | `0` | ✅ |
| 6 | `a=1, b=2` | `1` | `1` | ✅ |
| 7 | `a=50, b=30` | `20` | `20` | ✅ |
| 8 | `a=3, b=7` | `4` | `4` | ✅ |
| 9 | `a=200, b=150` | `50` | `50` | ✅ |
| 10 | `a=-10, b=-20` | `10` | `10` | ✅ |

### TC-10: Natural log: \ln\left(x\right) — ✅ PASS

**LaTeX Input:**
```latex
\ln\left(\mathrm{x}\right)
```

**Expected Formula Text:** `ln(x)`  
**Conversion Preview:** ✅ Verified  
**Formula ID:** eea22947-f383-47ec-8d97-df9a743d160b  
**Screenshot:** `tests/screenshots/018/tc-10-latex-panel.png`  

**Calculation Tests** (10/10 passed):

| # | Inputs | Expected | Actual | Pass |
|---|--------|----------|--------|:----:|
| 1 | `x=1` | `0` | `0` | ✅ |
| 2 | `x=2.718281828` | `1.000000000055511151` | `0.9999999998311266` | ✅ |
| 3 | `x=10` | `2.302585092994045684` | `2.302585092994046` | ✅ |
| 4 | `x=100` | `4.605170185988091368` | `4.605170185988092` | ✅ |
| 5 | `x=0.5` | `-0.693147180559945310` | `-0.6931471805599453` | ✅ |
| 6 | `x=2` | `0.693147180559945310` | `0.6931471805599453` | ✅ |
| 7 | `x=50` | `3.912023005428146059` | `3.912023005428146` | ✅ |
| 8 | `x=7.389` | `1.9999924078065106` | `1.9999924078065106` | ✅ |
| 9 | `x=20` | `2.995732273553990993` | `2.995732273553991` | ✅ |
| 10 | `x=1000` | `6.907755278982137052` | `6.907755278982137` | ✅ |

### TC-11: Custom function: \operatorname{max} — ✅ PASS

**LaTeX Input:**
```latex
\operatorname{max}\left(\mathrm{a}, \mathrm{b}\right)
```

**Expected Formula Text:** `max(a, b)`  
**Conversion Preview:** ✅ Verified  
**Formula ID:** 603cc874-28ff-4e48-8d6e-6a70a06c0b0f  
**Screenshot:** `tests/screenshots/018/tc-11-latex-panel.png`  

**Calculation Tests** (10/10 passed):

| # | Inputs | Expected | Actual | Pass |
|---|--------|----------|--------|:----:|
| 1 | `a=10, b=20` | `20` | `20` | ✅ |
| 2 | `a=20, b=10` | `20` | `20` | ✅ |
| 3 | `a=5, b=5` | `5` | `5` | ✅ |
| 4 | `a=0, b=1` | `1` | `1` | ✅ |
| 5 | `a=-5, b=0` | `0` | `0` | ✅ |
| 6 | `a=100, b=99` | `100` | `100` | ✅ |
| 7 | `a=1000, b=2000` | `2000` | `2000` | ✅ |
| 8 | `a=3.14, b=2.71` | `3.14` | `3.14` | ✅ |
| 9 | `a=0.1, b=0.2` | `0.2` | `0.2` | ✅ |
| 10 | `a=500, b=500` | `500` | `500` | ✅ |

### TC-12: Min function: \operatorname{min} — ✅ PASS

**LaTeX Input:**
```latex
\operatorname{min}\left(\mathrm{lo}, \mathrm{hi}\right)
```

**Expected Formula Text:** `min(lo, hi)`  
**Conversion Preview:** ✅ Verified  
**Formula ID:** e8c94134-1a98-43d5-b3f8-87b6c5599902  
**Screenshot:** `tests/screenshots/018/tc-12-latex-panel.png`  

**Calculation Tests** (10/10 passed):

| # | Inputs | Expected | Actual | Pass |
|---|--------|----------|--------|:----:|
| 1 | `lo=5, hi=10` | `5` | `5` | ✅ |
| 2 | `lo=10, hi=5` | `5` | `5` | ✅ |
| 3 | `lo=3, hi=3` | `3` | `3` | ✅ |
| 4 | `lo=0, hi=100` | `0` | `0` | ✅ |
| 5 | `lo=-10, hi=0` | `-10` | `-10` | ✅ |
| 6 | `lo=99, hi=100` | `99` | `99` | ✅ |
| 7 | `lo=1000, hi=500` | `500` | `500` | ✅ |
| 8 | `lo=0.05, hi=0.1` | `0.05` | `0.05` | ✅ |
| 9 | `lo=25, hi=75` | `25` | `25` | ✅ |
| 10 | `lo=200, hi=200` | `200` | `200` | ✅ |

### TC-13: Parentheses: (a + b) * c — ✅ PASS

**LaTeX Input:**
```latex
\left(\mathrm{a} + \mathrm{b}\right) \cdot \mathrm{c}
```

**Expected Formula Text:** `(a + b) * c`  
**Conversion Preview:** ✅ Verified  
**Formula ID:** fe07e71f-75e4-4789-a63e-c79459e0f0eb  
**Screenshot:** `tests/screenshots/018/tc-13-latex-panel.png`  

**Calculation Tests** (10/10 passed):

| # | Inputs | Expected | Actual | Pass |
|---|--------|----------|--------|:----:|
| 1 | `a=2, b=3, c=4` | `20` | `20` | ✅ |
| 2 | `a=10, b=5, c=2` | `30` | `30` | ✅ |
| 3 | `a=0, b=0, c=100` | `0` | `0` | ✅ |
| 4 | `a=1, b=1, c=1` | `2` | `2` | ✅ |
| 5 | `a=5, b=5, c=10` | `100` | `100` | ✅ |
| 6 | `a=100, b=50, c=0` | `0` | `0` | ✅ |
| 7 | `a=3, b=7, c=5` | `50` | `50` | ✅ |
| 8 | `a=20, b=30, c=2` | `100` | `100` | ✅ |
| 9 | `a=6, b=4, c=3` | `30` | `30` | ✅ |
| 10 | `a=15, b=5, c=4` | `80` | `80` | ✅ |

### TC-14: Comparison: \ge in conditional — ✅ PASS

**LaTeX Input:**
```latex
\begin{cases}
\mathrm{premium} \cdot 1.5, & \text{if } \mathrm{age} \ge 60 \\
\mathrm{premium}, & \text{otherwise}
\end{cases}
```

**Expected Formula Text:** `if age >= 60 then premium * 1.5 else premium`  
**Conversion Preview:** ✅ Verified  
**Formula ID:** 70ab6fe3-843e-4f04-aeca-3c0fc7ed38e2  
**Screenshot:** `tests/screenshots/018/tc-14-latex-panel.png`  

**Calculation Tests** (10/10 passed):

| # | Inputs | Expected | Actual | Pass |
|---|--------|----------|--------|:----:|
| 1 | `age=65, premium=1000` | `1500` | `1500` | ✅ |
| 2 | `age=60, premium=1000` | `1500` | `1500` | ✅ |
| 3 | `age=59, premium=1000` | `1000` | `1000` | ✅ |
| 4 | `age=30, premium=500` | `500` | `500` | ✅ |
| 5 | `age=70, premium=2000` | `3000` | `3000` | ✅ |
| 6 | `age=45, premium=800` | `800` | `800` | ✅ |
| 7 | `age=60, premium=600` | `900` | `900` | ✅ |
| 8 | `age=0, premium=1200` | `1200` | `1200` | ✅ |
| 9 | `age=80, premium=400` | `600` | `600` | ✅ |
| 10 | `age=59.9, premium=1000` | `1000` | `1000` | ✅ |

### TC-15: Comparison: \le in conditional (discount for low risk) — ✅ PASS

**LaTeX Input:**
```latex
\begin{cases}
\mathrm{premium} \cdot 0.9, & \text{if } \mathrm{risk} \le 0.3 \\
\mathrm{premium}, & \text{otherwise}
\end{cases}
```

**Expected Formula Text:** `if risk <= 0.3 then premium * 0.9 else premium`  
**Conversion Preview:** ✅ Verified  
**Formula ID:** 587158ed-247b-43f4-8952-cca692e840dc  
**Screenshot:** `tests/screenshots/018/tc-15-latex-panel.png`  

**Calculation Tests** (10/10 passed):

| # | Inputs | Expected | Actual | Pass |
|---|--------|----------|--------|:----:|
| 1 | `risk=0.1, premium=1000` | `900` | `900` | ✅ |
| 2 | `risk=0.3, premium=1000` | `900` | `900` | ✅ |
| 3 | `risk=0.31, premium=1000` | `1000` | `1000` | ✅ |
| 4 | `risk=0.5, premium=500` | `500` | `500` | ✅ |
| 5 | `risk=0, premium=2000` | `1800` | `1800` | ✅ |
| 6 | `risk=1, premium=800` | `800` | `800` | ✅ |
| 7 | `risk=0.2, premium=600` | `540` | `540` | ✅ |
| 8 | `risk=0.3, premium=1200` | `1080` | `1080` | ✅ |
| 9 | `risk=0.8, premium=400` | `400` | `400` | ✅ |
| 10 | `risk=0.29, premium=1000` | `900` | `900` | ✅ |

### TC-16: Comparison: \ne in conditional (bonus flag) — ✅ PASS

**LaTeX Input:**
```latex
\begin{cases}
\mathrm{bonus}, & \text{if } \mathrm{flag} \ne 0 \\
0, & \text{otherwise}
\end{cases}
```

**Expected Formula Text:** `if flag != 0 then bonus else 0`  
**Conversion Preview:** ✅ Verified  
**Formula ID:** d1edcb1a-8511-497b-a412-fa5760b830d9  
**Screenshot:** `tests/screenshots/018/tc-16-latex-panel.png`  

**Calculation Tests** (10/10 passed):

| # | Inputs | Expected | Actual | Pass |
|---|--------|----------|--------|:----:|
| 1 | `flag=1, bonus=500` | `500` | `500` | ✅ |
| 2 | `flag=0, bonus=500` | `0` | `0` | ✅ |
| 3 | `flag=2, bonus=200` | `200` | `200` | ✅ |
| 4 | `flag=-1, bonus=300` | `300` | `300` | ✅ |
| 5 | `flag=0, bonus=100` | `0` | `0` | ✅ |
| 6 | `flag=100, bonus=1000` | `1000` | `1000` | ✅ |
| 7 | `flag=0, bonus=750` | `0` | `0` | ✅ |
| 8 | `flag=1, bonus=0` | `0` | `0` | ✅ |
| 9 | `flag=5, bonus=600` | `600` | `600` | ✅ |
| 10 | `flag=0, bonus=999` | `0` | `0` | ✅ |

### TC-17: Comparison: \geq (alternative form) in conditional — ✅ PASS

**LaTeX Input:**
```latex
\begin{cases}
100, & \text{if } \mathrm{score} \geq 90 \\
\mathrm{score}, & \text{otherwise}
\end{cases}
```

**Expected Formula Text:** `if score >= 90 then 100 else score`  
**Conversion Preview:** ✅ Verified  
**Formula ID:** 2c15550f-49fb-44b8-aac6-fd71608f08e4  
**Screenshot:** `tests/screenshots/018/tc-17-latex-panel.png`  

**Calculation Tests** (10/10 passed):

| # | Inputs | Expected | Actual | Pass |
|---|--------|----------|--------|:----:|
| 1 | `score=95` | `100` | `100` | ✅ |
| 2 | `score=90` | `100` | `100` | ✅ |
| 3 | `score=89` | `89` | `89` | ✅ |
| 4 | `score=100` | `100` | `100` | ✅ |
| 5 | `score=0` | `0` | `0` | ✅ |
| 6 | `score=50` | `50` | `50` | ✅ |
| 7 | `score=89.9` | `89.9` | `89.9` | ✅ |
| 8 | `score=90.1` | `100` | `100` | ✅ |
| 9 | `score=75` | `75` | `75` | ✅ |
| 10 | `score=91` | `100` | `100` | ✅ |

### TC-18: Simple conditional: \begin{cases} — ✅ PASS

**LaTeX Input:**
```latex
\begin{cases}
1.5, & \text{if } \mathrm{age} \ge 65 \\
1, & \text{otherwise}
\end{cases}
```

**Expected Formula Text:** `if age >= 65 then 1.5 else 1`  
**Conversion Preview:** ✅ Verified  
**Formula ID:** 23d90863-d209-419c-8c92-ddd608deaa5e  
**Screenshot:** `tests/screenshots/018/tc-18-latex-panel.png`  

**Calculation Tests** (10/10 passed):

| # | Inputs | Expected | Actual | Pass |
|---|--------|----------|--------|:----:|
| 1 | `age=70` | `1.5` | `1.5` | ✅ |
| 2 | `age=65` | `1.5` | `1.5` | ✅ |
| 3 | `age=64` | `1` | `1` | ✅ |
| 4 | `age=30` | `1` | `1` | ✅ |
| 5 | `age=0` | `1` | `1` | ✅ |
| 6 | `age=100` | `1.5` | `1.5` | ✅ |
| 7 | `age=66` | `1.5` | `1.5` | ✅ |
| 8 | `age=64.9` | `1` | `1` | ✅ |
| 9 | `age=65.1` | `1.5` | `1.5` | ✅ |
| 10 | `age=18` | `1` | `1` | ✅ |

### TC-19: Nested conditional: nested \begin{cases} — ✅ PASS

**LaTeX Input:**
```latex
\begin{cases}
2, & \text{if } \mathrm{age} \ge 65 \\
\begin{cases}
1.5, & \text{if } \mathrm{age} \ge 45 \\
1, & \text{otherwise}
\end{cases}, & \text{otherwise}
\end{cases}
```

**Expected Formula Text:** `if age >= 65 then 2 else if age >= 45 then 1.5 else 1`  
**Conversion Preview:** ✅ Verified  
**Formula ID:** da8d430d-2b3b-4d2a-bcb9-a047390341c5  
**Screenshot:** `tests/screenshots/018/tc-19-latex-panel.png`  

**Calculation Tests** (10/10 passed):

| # | Inputs | Expected | Actual | Pass |
|---|--------|----------|--------|:----:|
| 1 | `age=70` | `2` | `2` | ✅ |
| 2 | `age=65` | `2` | `2` | ✅ |
| 3 | `age=64` | `1.5` | `1.5` | ✅ |
| 4 | `age=50` | `1.5` | `1.5` | ✅ |
| 5 | `age=45` | `1.5` | `1.5` | ✅ |
| 6 | `age=44` | `1` | `1` | ✅ |
| 7 | `age=30` | `1` | `1` | ✅ |
| 8 | `age=0` | `1` | `1` | ✅ |
| 9 | `age=80` | `2` | `2` | ✅ |
| 10 | `age=44.9` | `1` | `1` | ✅ |

### TC-20: Compound: \frac{sum_insured * rate}{1000} — ✅ PASS

**LaTeX Input:**
```latex
\frac{\mathrm{sum\_insured} \cdot \mathrm{rate}}{1000}
```

**Expected Formula Text:** `(sum_insured * rate) / (1000)`  
**Conversion Preview:** ✅ Verified  
**Formula ID:** 3bc15043-8969-4f33-b8aa-e7f4dce3122b  
**Screenshot:** `tests/screenshots/018/tc-20-latex-panel.png`  

**Calculation Tests** (10/10 passed):

| # | Inputs | Expected | Actual | Pass |
|---|--------|----------|--------|:----:|
| 1 | `sum_insured=100000, rate=5` | `500` | `500` | ✅ |
| 2 | `sum_insured=200000, rate=3` | `600` | `600` | ✅ |
| 3 | `sum_insured=500000, rate=2` | `1000` | `1000` | ✅ |
| 4 | `sum_insured=1000000, rate=1` | `1000` | `1000` | ✅ |
| 5 | `sum_insured=50000, rate=10` | `500` | `500` | ✅ |
| 6 | `sum_insured=300000, rate=4` | `1200` | `1200` | ✅ |
| 7 | `sum_insured=0, rate=5` | `0` | `0` | ✅ |
| 8 | `sum_insured=750000, rate=2` | `1500` | `1500` | ✅ |
| 9 | `sum_insured=80000, rate=7.5` | `600` | `600` | ✅ |
| 10 | `sum_insured=400000, rate=3.5` | `1400` | `1400` | ✅ |

---

## Screenshots

Screenshots are saved in `tests/screenshots/018/`:

- `tc-01-latex-panel.png` — LaTeX input panel for TC-01
- `tc-01-graph.png` — Visual formula graph for TC-01
- `tc-02-latex-panel.png` — LaTeX input panel for TC-02
- `tc-02-graph.png` — Visual formula graph for TC-02
- `tc-03-latex-panel.png` — LaTeX input panel for TC-03
- `tc-03-graph.png` — Visual formula graph for TC-03
- `tc-04-latex-panel.png` — LaTeX input panel for TC-04
- `tc-04-graph.png` — Visual formula graph for TC-04
- `tc-05-latex-panel.png` — LaTeX input panel for TC-05
- `tc-05-graph.png` — Visual formula graph for TC-05
- `tc-06-latex-panel.png` — LaTeX input panel for TC-06
- `tc-06-graph.png` — Visual formula graph for TC-06
- `tc-07-latex-panel.png` — LaTeX input panel for TC-07
- `tc-07-graph.png` — Visual formula graph for TC-07
- `tc-08-latex-panel.png` — LaTeX input panel for TC-08
- `tc-08-graph.png` — Visual formula graph for TC-08
- `tc-09-latex-panel.png` — LaTeX input panel for TC-09
- `tc-09-graph.png` — Visual formula graph for TC-09
- `tc-10-latex-panel.png` — LaTeX input panel for TC-10
- `tc-10-graph.png` — Visual formula graph for TC-10
- `tc-11-latex-panel.png` — LaTeX input panel for TC-11
- `tc-11-graph.png` — Visual formula graph for TC-11
- `tc-12-latex-panel.png` — LaTeX input panel for TC-12
- `tc-12-graph.png` — Visual formula graph for TC-12
- `tc-13-latex-panel.png` — LaTeX input panel for TC-13
- `tc-13-graph.png` — Visual formula graph for TC-13
- `tc-14-latex-panel.png` — LaTeX input panel for TC-14
- `tc-14-graph.png` — Visual formula graph for TC-14
- `tc-15-latex-panel.png` — LaTeX input panel for TC-15
- `tc-15-graph.png` — Visual formula graph for TC-15
- `tc-16-latex-panel.png` — LaTeX input panel for TC-16
- `tc-16-graph.png` — Visual formula graph for TC-16
- `tc-17-latex-panel.png` — LaTeX input panel for TC-17
- `tc-17-graph.png` — Visual formula graph for TC-17
- `tc-18-latex-panel.png` — LaTeX input panel for TC-18
- `tc-18-graph.png` — Visual formula graph for TC-18
- `tc-19-latex-panel.png` — LaTeX input panel for TC-19
- `tc-19-graph.png` — Visual formula graph for TC-19
- `tc-20-latex-panel.png` — LaTeX input panel for TC-20
- `tc-20-graph.png` — Visual formula graph for TC-20
