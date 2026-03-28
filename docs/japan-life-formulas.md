# Japan Life Insurance Presets

## Purpose

This project now includes a small set of preset formulas inspired by common
Japanese life insurance pricing and reserve concepts.

These presets are intentionally **engine-friendly approximations** of standard
industry ideas. They are useful as:

- sample templates for editor users
- test data for complex insurance-style graphs
- starting points for product-specific customization

They are **not** carrier filing formulas, statutory reserving formulas, or
actuarial sign-off models.

## Research Basis

The presets were chosen from concepts that appear repeatedly in Japanese life
insurance educational and industry materials:

- Premiums are built from the three assumed rates:
  expected mortality, assumed interest, and expected expense rate.
- Gross premium is composed of net premium plus additional premium.
- Level-premium products accumulate a policy reserve over time.
- Standard policy reserves in Japan are, in principle, accumulated under the
  level net premium method.
- Surrender value is typically described as a reserve-based amount reduced by a
  deduction related to the insurance amount and elapsed duration.

## Sources

- Life Insurance Association of Japan:
  [STEP. 7 保険料と配当金の仕組み](https://www.seiho.or.jp/data/billboard/introduction/content07/)
- JAIFA:
  [保険料計算の基礎](https://www.jaifa.or.jp/knowledge/kiso_sei_83/)
- JILI consultation manual:
  [保険料と配当金 / 収支相等の原則](https://www.jili.or.jp/files/consul/consultation_manual_6.pdf)
- OLIS:
  [Learning About Life](https://www.olis.or.jp/pdf/publication202302_matsuzawa_en.pdf)
- JAIFA:
  [責任準備金](https://www.jaifa.or.jp/knowledge/kiso_sei_51/)
- JAIFA:
  [解約返戻金](https://www.jaifa.or.jp/knowledge/kiso_sei_10/)

## Preset Formulas

### 1. 日本寿险 基础保费（収支相等原則）

Engine formula:

```text
basePremium = deathBenefit * expectedDeaths / policyCount
```

Why it exists:

- Mirrors the equivalence-principle style relationship shown in Japanese
  educational material:
  premium per person × number of policyholders = benefit per person × number of deaths.

### 2. 日本寿险 粗保费分解

Engine formula:

```text
grossPremium = netPremium + acquisitionExpense + collectionExpense + maintenanceExpense
```

Why it exists:

- Reflects the Japanese explanation that gross premium is net premium plus
  additional premium, while the additional premium covers business expenses.

### 3. 日本寿险 责任准备金滚动近似

Engine formula:

```text
reserveEnd =
  reserveBegin * (1 + assumedInterestRate)
  + levelPremium
  - expectedBenefit
  - maintenanceExpense
```

Why it exists:

- This is a practical reserve roll-forward approximation derived from the
  reserve accumulation concept used in level-premium life products.
- It is not a full statutory reserve formula, but it maps cleanly into the DAG
  engine and demonstrates the same business intuition.

### 4. 日本寿险 解约返还金近似

Engine formula:

```text
surrenderValue = max(netPremiumReserve - deathBenefit * surrenderChargeRate, 0)
```

Why it exists:

- JAIFA describes surrender value as an amount based on pure-premium reserve
  with a deduction related to the insured amount and elapsed duration.
- This preset turns that explanation into a reusable graph template.
