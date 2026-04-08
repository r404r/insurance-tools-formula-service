/**
 * Task #018 — LaTeX Formula Input E2E Test Suite
 *
 * Tests 20 LaTeX syntax cases end-to-end:
 *  1. Inputs LaTeX → converts → screenshots visual formula graph
 *  2. Runs 10 calculation API calls per formula → verifies expected results
 *
 * Prerequisites: dev server running on localhost:5173, backend on localhost:8080
 * Run: npx playwright test --config=playwright.config.ts
 */

import { test, expect, type Page } from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

const API_BASE = 'http://localhost:8080/api/v1'
const SCREENSHOTS_DIR = path.join(__dirname, '../screenshots/018')
const REPORT_DATA_FILE = path.join(__dirname, '../reports/018-data.json')

// Admin credentials (created during backend setup)
const ADMIN_USER = 'admin'
const ADMIN_PASS = 'admin99999'

// ---------------------------------------------------------------------------
// Test case definitions
// ---------------------------------------------------------------------------

interface CalcCase {
  inputs: Record<string, string>
  expectedOutputKey: string
  expectedValue: string
}

interface TestCase {
  id: string
  description: string
  latex: string
  expectedFormulaText: string
  calcCases: CalcCase[]
}

const TEST_CASES: TestCase[] = [
  // TC-01: Addition — variable identifiers with \mathrm, underscore variable names
  {
    id: 'TC-01',
    description: 'Addition: \\mathrm identifiers + underscore variable names',
    latex: '\\mathrm{age} + \\mathrm{base\\_rate}',
    expectedFormulaText: 'age + base_rate',
    calcCases: [
      { inputs: { age: '30', base_rate: '0.05' }, expectedOutputKey: 'out', expectedValue: '30.05' },
      { inputs: { age: '25', base_rate: '0.03' }, expectedOutputKey: 'out', expectedValue: '25.03' },
      { inputs: { age: '40', base_rate: '0.1' }, expectedOutputKey: 'out', expectedValue: '40.1' },
      { inputs: { age: '0', base_rate: '0' }, expectedOutputKey: 'out', expectedValue: '0' },
      { inputs: { age: '65', base_rate: '0.15' }, expectedOutputKey: 'out', expectedValue: '65.15' },
      { inputs: { age: '18', base_rate: '0.02' }, expectedOutputKey: 'out', expectedValue: '18.02' },
      { inputs: { age: '50', base_rate: '0.08' }, expectedOutputKey: 'out', expectedValue: '50.08' },
      { inputs: { age: '1', base_rate: '0.001' }, expectedOutputKey: 'out', expectedValue: '1.001' },
      { inputs: { age: '100', base_rate: '0.5' }, expectedOutputKey: 'out', expectedValue: '100.5' },
      { inputs: { age: '35', base_rate: '0.06' }, expectedOutputKey: 'out', expectedValue: '35.06' },
    ],
  },

  // TC-02: Subtraction
  {
    id: 'TC-02',
    description: 'Subtraction: premium - discount',
    latex: '\\mathrm{premium} - \\mathrm{discount}',
    expectedFormulaText: 'premium - discount',
    calcCases: [
      { inputs: { premium: '1000', discount: '100' }, expectedOutputKey: 'out', expectedValue: '900' },
      { inputs: { premium: '500', discount: '50' }, expectedOutputKey: 'out', expectedValue: '450' },
      { inputs: { premium: '2000', discount: '0' }, expectedOutputKey: 'out', expectedValue: '2000' },
      { inputs: { premium: '750', discount: '250' }, expectedOutputKey: 'out', expectedValue: '500' },
      { inputs: { premium: '1500', discount: '300' }, expectedOutputKey: 'out', expectedValue: '1200' },
      { inputs: { premium: '100', discount: '100' }, expectedOutputKey: 'out', expectedValue: '0' },
      { inputs: { premium: '3000', discount: '450' }, expectedOutputKey: 'out', expectedValue: '2550' },
      { inputs: { premium: '800', discount: '200' }, expectedOutputKey: 'out', expectedValue: '600' },
      { inputs: { premium: '1200', discount: '120' }, expectedOutputKey: 'out', expectedValue: '1080' },
      { inputs: { premium: '600', discount: '60' }, expectedOutputKey: 'out', expectedValue: '540' },
    ],
  },

  // TC-03: Multiplication with \cdot
  {
    id: 'TC-03',
    description: 'Multiplication: \\cdot → *',
    latex: '\\mathrm{sum\\_insured} \\cdot \\mathrm{rate}',
    expectedFormulaText: 'sum_insured * rate',
    calcCases: [
      { inputs: { sum_insured: '100000', rate: '0.005' }, expectedOutputKey: 'out', expectedValue: '500' },
      { inputs: { sum_insured: '200000', rate: '0.003' }, expectedOutputKey: 'out', expectedValue: '600' },
      { inputs: { sum_insured: '50000', rate: '0.01' }, expectedOutputKey: 'out', expectedValue: '500' },
      { inputs: { sum_insured: '1000000', rate: '0.002' }, expectedOutputKey: 'out', expectedValue: '2000' },
      { inputs: { sum_insured: '0', rate: '0.005' }, expectedOutputKey: 'out', expectedValue: '0' },
      { inputs: { sum_insured: '500000', rate: '0.004' }, expectedOutputKey: 'out', expectedValue: '2000' },
      { inputs: { sum_insured: '75000', rate: '0.006' }, expectedOutputKey: 'out', expectedValue: '450' },
      { inputs: { sum_insured: '300000', rate: '0.0025' }, expectedOutputKey: 'out', expectedValue: '750' },
      { inputs: { sum_insured: '800000', rate: '0.001' }, expectedOutputKey: 'out', expectedValue: '800' },
      { inputs: { sum_insured: '150000', rate: '0.007' }, expectedOutputKey: 'out', expectedValue: '1050' },
    ],
  },

  // TC-04: Multiplication with \times
  {
    id: 'TC-04',
    description: 'Multiplication: \\times → *',
    latex: '\\mathrm{principal} \\times \\mathrm{factor}',
    expectedFormulaText: 'principal * factor',
    calcCases: [
      { inputs: { principal: '50000', factor: '1.5' }, expectedOutputKey: 'out', expectedValue: '75000' },
      { inputs: { principal: '10000', factor: '2' }, expectedOutputKey: 'out', expectedValue: '20000' },
      { inputs: { principal: '1000', factor: '1' }, expectedOutputKey: 'out', expectedValue: '1000' },
      { inputs: { principal: '5000', factor: '0.5' }, expectedOutputKey: 'out', expectedValue: '2500' },
      { inputs: { principal: '25000', factor: '1.2' }, expectedOutputKey: 'out', expectedValue: '30000' },
      { inputs: { principal: '100000', factor: '0.8' }, expectedOutputKey: 'out', expectedValue: '80000' },
      { inputs: { principal: '20000', factor: '3' }, expectedOutputKey: 'out', expectedValue: '60000' },
      { inputs: { principal: '7500', factor: '1.1' }, expectedOutputKey: 'out', expectedValue: '8250' },
      { inputs: { principal: '40000', factor: '1.25' }, expectedOutputKey: 'out', expectedValue: '50000' },
      { inputs: { principal: '15000', factor: '0.9' }, expectedOutputKey: 'out', expectedValue: '13500' },
    ],
  },

  // TC-05: Division with \frac
  {
    id: 'TC-05',
    description: 'Division: \\frac{annual}{12}',
    latex: '\\frac{\\mathrm{annual}}{12}',
    expectedFormulaText: '(annual) / (12)',
    calcCases: [
      { inputs: { annual: '12000' }, expectedOutputKey: 'out', expectedValue: '1000' },
      { inputs: { annual: '24000' }, expectedOutputKey: 'out', expectedValue: '2000' },
      { inputs: { annual: '6000' }, expectedOutputKey: 'out', expectedValue: '500' },
      { inputs: { annual: '0' }, expectedOutputKey: 'out', expectedValue: '0' },
      { inputs: { annual: '3600' }, expectedOutputKey: 'out', expectedValue: '300' },
      { inputs: { annual: '48000' }, expectedOutputKey: 'out', expectedValue: '4000' },
      { inputs: { annual: '1200' }, expectedOutputKey: 'out', expectedValue: '100' },
      { inputs: { annual: '18000' }, expectedOutputKey: 'out', expectedValue: '1500' },
      { inputs: { annual: '9600' }, expectedOutputKey: 'out', expectedValue: '800' },
      { inputs: { annual: '36000' }, expectedOutputKey: 'out', expectedValue: '3000' },
    ],
  },

  // TC-06: Power operator ^{n}
  {
    id: 'TC-06',
    description: 'Power: x^{2}',
    latex: '\\mathrm{x}^{2}',
    expectedFormulaText: 'x ^ (2)',
    calcCases: [
      { inputs: { x: '5' }, expectedOutputKey: 'out', expectedValue: '25' },
      { inputs: { x: '10' }, expectedOutputKey: 'out', expectedValue: '100' },
      { inputs: { x: '0' }, expectedOutputKey: 'out', expectedValue: '0' },
      { inputs: { x: '1' }, expectedOutputKey: 'out', expectedValue: '1' },
      { inputs: { x: '2' }, expectedOutputKey: 'out', expectedValue: '4' },
      { inputs: { x: '3' }, expectedOutputKey: 'out', expectedValue: '9' },
      { inputs: { x: '4' }, expectedOutputKey: 'out', expectedValue: '16' },
      { inputs: { x: '7' }, expectedOutputKey: 'out', expectedValue: '49' },
      { inputs: { x: '8' }, expectedOutputKey: 'out', expectedValue: '64' },
      { inputs: { x: '12' }, expectedOutputKey: 'out', expectedValue: '144' },
    ],
  },

  // TC-07: Natural exponent e^{x}
  {
    id: 'TC-07',
    description: 'Natural exponent: e^{x} → exp(x)',
    latex: 'e^{\\mathrm{r}}',
    expectedFormulaText: 'exp(r)',
    calcCases: [
      { inputs: { r: '0' }, expectedOutputKey: 'out', expectedValue: '1' },
      { inputs: { r: '1' }, expectedOutputKey: 'out', expectedValue: '2.718281828459045235' },
      { inputs: { r: '2' }, expectedOutputKey: 'out', expectedValue: '7.389056098930650227' },
      { inputs: { r: '-1' }, expectedOutputKey: 'out', expectedValue: '0.367879441171442322' },
      { inputs: { r: '0.5' }, expectedOutputKey: 'out', expectedValue: '1.648721270700128147' },
      { inputs: { r: '3' }, expectedOutputKey: 'out', expectedValue: '20.085536923187667741' },
      { inputs: { r: '0.1' }, expectedOutputKey: 'out', expectedValue: '1.105170918075647625' },
      { inputs: { r: '0.693147' }, expectedOutputKey: 'out', expectedValue: '1.999999819512220285' },
      { inputs: { r: '-0.5' }, expectedOutputKey: 'out', expectedValue: '0.606530659712633424' },
      { inputs: { r: '1.5' }, expectedOutputKey: 'out', expectedValue: '4.481689070339594636' },
    ],
  },

  // TC-08: Square root
  {
    id: 'TC-08',
    description: 'Square root: \\sqrt{x}',
    latex: '\\sqrt{\\mathrm{x}}',
    expectedFormulaText: 'sqrt(x)',
    calcCases: [
      { inputs: { x: '4' }, expectedOutputKey: 'out', expectedValue: '2' },
      { inputs: { x: '9' }, expectedOutputKey: 'out', expectedValue: '3' },
      { inputs: { x: '16' }, expectedOutputKey: 'out', expectedValue: '4' },
      { inputs: { x: '25' }, expectedOutputKey: 'out', expectedValue: '5' },
      { inputs: { x: '0' }, expectedOutputKey: 'out', expectedValue: '0' },
      { inputs: { x: '1' }, expectedOutputKey: 'out', expectedValue: '1' },
      { inputs: { x: '100' }, expectedOutputKey: 'out', expectedValue: '10' },
      { inputs: { x: '2' }, expectedOutputKey: 'out', expectedValue: '1.414213562373095049' },
      { inputs: { x: '3' }, expectedOutputKey: 'out', expectedValue: '1.732050808056887729' },
      { inputs: { x: '144' }, expectedOutputKey: 'out', expectedValue: '12' },
    ],
  },

  // TC-09: Absolute value
  {
    id: 'TC-09',
    description: 'Absolute value: \\left|a - b\\right|',
    latex: '\\left|\\mathrm{a} - \\mathrm{b}\\right|',
    expectedFormulaText: 'abs(a - b)',
    calcCases: [
      { inputs: { a: '5', b: '8' }, expectedOutputKey: 'out', expectedValue: '3' },
      { inputs: { a: '10', b: '3' }, expectedOutputKey: 'out', expectedValue: '7' },
      { inputs: { a: '0', b: '0' }, expectedOutputKey: 'out', expectedValue: '0' },
      { inputs: { a: '-5', b: '5' }, expectedOutputKey: 'out', expectedValue: '10' },
      { inputs: { a: '100', b: '100' }, expectedOutputKey: 'out', expectedValue: '0' },
      { inputs: { a: '1', b: '2' }, expectedOutputKey: 'out', expectedValue: '1' },
      { inputs: { a: '50', b: '30' }, expectedOutputKey: 'out', expectedValue: '20' },
      { inputs: { a: '3', b: '7' }, expectedOutputKey: 'out', expectedValue: '4' },
      { inputs: { a: '200', b: '150' }, expectedOutputKey: 'out', expectedValue: '50' },
      { inputs: { a: '-10', b: '-20' }, expectedOutputKey: 'out', expectedValue: '10' },
    ],
  },

  // TC-10: Natural log
  {
    id: 'TC-10',
    description: 'Natural log: \\ln\\left(x\\right)',
    latex: '\\ln\\left(\\mathrm{x}\\right)',
    expectedFormulaText: 'ln(x)',
    calcCases: [
      { inputs: { x: '1' }, expectedOutputKey: 'out', expectedValue: '0' },
      { inputs: { x: '2.718281828' }, expectedOutputKey: 'out', expectedValue: '1.000000000055511151' },
      { inputs: { x: '10' }, expectedOutputKey: 'out', expectedValue: '2.302585092994045684' },
      { inputs: { x: '100' }, expectedOutputKey: 'out', expectedValue: '4.605170185988091368' },
      { inputs: { x: '0.5' }, expectedOutputKey: 'out', expectedValue: '-0.693147180559945310' },
      { inputs: { x: '2' }, expectedOutputKey: 'out', expectedValue: '0.693147180559945310' },
      { inputs: { x: '50' }, expectedOutputKey: 'out', expectedValue: '3.912023005428146059' },
      { inputs: { x: '7.389' }, expectedOutputKey: 'out', expectedValue: '1.9999924078065106' },
      { inputs: { x: '20' }, expectedOutputKey: 'out', expectedValue: '2.995732273553990993' },
      { inputs: { x: '1000' }, expectedOutputKey: 'out', expectedValue: '6.907755278982137052' },
    ],
  },

  // TC-11: Custom function via \operatorname{max}
  {
    id: 'TC-11',
    description: 'Custom function: \\operatorname{max}',
    latex: '\\operatorname{max}\\left(\\mathrm{a}, \\mathrm{b}\\right)',
    expectedFormulaText: 'max(a, b)',
    calcCases: [
      { inputs: { a: '10', b: '20' }, expectedOutputKey: 'out', expectedValue: '20' },
      { inputs: { a: '20', b: '10' }, expectedOutputKey: 'out', expectedValue: '20' },
      { inputs: { a: '5', b: '5' }, expectedOutputKey: 'out', expectedValue: '5' },
      { inputs: { a: '0', b: '1' }, expectedOutputKey: 'out', expectedValue: '1' },
      { inputs: { a: '-5', b: '0' }, expectedOutputKey: 'out', expectedValue: '0' },
      { inputs: { a: '100', b: '99' }, expectedOutputKey: 'out', expectedValue: '100' },
      { inputs: { a: '1000', b: '2000' }, expectedOutputKey: 'out', expectedValue: '2000' },
      { inputs: { a: '3.14', b: '2.71' }, expectedOutputKey: 'out', expectedValue: '3.14' },
      { inputs: { a: '0.1', b: '0.2' }, expectedOutputKey: 'out', expectedValue: '0.2' },
      { inputs: { a: '500', b: '500' }, expectedOutputKey: 'out', expectedValue: '500' },
    ],
  },

  // TC-12: min function via \operatorname{min}
  {
    id: 'TC-12',
    description: 'Min function: \\operatorname{min}',
    latex: '\\operatorname{min}\\left(\\mathrm{lo}, \\mathrm{hi}\\right)',
    expectedFormulaText: 'min(lo, hi)',
    calcCases: [
      { inputs: { lo: '5', hi: '10' }, expectedOutputKey: 'out', expectedValue: '5' },
      { inputs: { lo: '10', hi: '5' }, expectedOutputKey: 'out', expectedValue: '5' },
      { inputs: { lo: '3', hi: '3' }, expectedOutputKey: 'out', expectedValue: '3' },
      { inputs: { lo: '0', hi: '100' }, expectedOutputKey: 'out', expectedValue: '0' },
      { inputs: { lo: '-10', hi: '0' }, expectedOutputKey: 'out', expectedValue: '-10' },
      { inputs: { lo: '99', hi: '100' }, expectedOutputKey: 'out', expectedValue: '99' },
      { inputs: { lo: '1000', hi: '500' }, expectedOutputKey: 'out', expectedValue: '500' },
      { inputs: { lo: '0.05', hi: '0.1' }, expectedOutputKey: 'out', expectedValue: '0.05' },
      { inputs: { lo: '25', hi: '75' }, expectedOutputKey: 'out', expectedValue: '25' },
      { inputs: { lo: '200', hi: '200' }, expectedOutputKey: 'out', expectedValue: '200' },
    ],
  },

  // TC-13: Parentheses grouping
  {
    id: 'TC-13',
    description: 'Parentheses: (a + b) * c',
    latex: '\\left(\\mathrm{a} + \\mathrm{b}\\right) \\cdot \\mathrm{c}',
    expectedFormulaText: '(a + b) * c',
    calcCases: [
      { inputs: { a: '2', b: '3', c: '4' }, expectedOutputKey: 'out', expectedValue: '20' },
      { inputs: { a: '10', b: '5', c: '2' }, expectedOutputKey: 'out', expectedValue: '30' },
      { inputs: { a: '0', b: '0', c: '100' }, expectedOutputKey: 'out', expectedValue: '0' },
      { inputs: { a: '1', b: '1', c: '1' }, expectedOutputKey: 'out', expectedValue: '2' },
      { inputs: { a: '5', b: '5', c: '10' }, expectedOutputKey: 'out', expectedValue: '100' },
      { inputs: { a: '100', b: '50', c: '0' }, expectedOutputKey: 'out', expectedValue: '0' },
      { inputs: { a: '3', b: '7', c: '5' }, expectedOutputKey: 'out', expectedValue: '50' },
      { inputs: { a: '20', b: '30', c: '2' }, expectedOutputKey: 'out', expectedValue: '100' },
      { inputs: { a: '6', b: '4', c: '3' }, expectedOutputKey: 'out', expectedValue: '30' },
      { inputs: { a: '15', b: '5', c: '4' }, expectedOutputKey: 'out', expectedValue: '80' },
    ],
  },

  // TC-14: Comparison >= via \ge — embedded in conditional for calculability
  // Note: bare `age >= 18` produces an incomplete conditional DAG node (no then/else)
  // so we embed the \ge operator in a full if-then-else expression.
  {
    id: 'TC-14',
    description: 'Comparison: \\ge in conditional',
    latex: '\\begin{cases}\n\\mathrm{premium} \\cdot 1.5, & \\text{if } \\mathrm{age} \\ge 60 \\\\\n\\mathrm{premium}, & \\text{otherwise}\n\\end{cases}',
    expectedFormulaText: 'if age >= 60 then premium * 1.5 else premium',
    calcCases: [
      { inputs: { age: '65', premium: '1000' }, expectedOutputKey: 'out', expectedValue: '1500' },
      { inputs: { age: '60', premium: '1000' }, expectedOutputKey: 'out', expectedValue: '1500' },
      { inputs: { age: '59', premium: '1000' }, expectedOutputKey: 'out', expectedValue: '1000' },
      { inputs: { age: '30', premium: '500' }, expectedOutputKey: 'out', expectedValue: '500' },
      { inputs: { age: '70', premium: '2000' }, expectedOutputKey: 'out', expectedValue: '3000' },
      { inputs: { age: '45', premium: '800' }, expectedOutputKey: 'out', expectedValue: '800' },
      { inputs: { age: '60', premium: '600' }, expectedOutputKey: 'out', expectedValue: '900' },
      { inputs: { age: '0', premium: '1200' }, expectedOutputKey: 'out', expectedValue: '1200' },
      { inputs: { age: '80', premium: '400' }, expectedOutputKey: 'out', expectedValue: '600' },
      { inputs: { age: '59.9', premium: '1000' }, expectedOutputKey: 'out', expectedValue: '1000' },
    ],
  },

  // TC-15: Comparison <= via \le — embedded in conditional for calculability
  {
    id: 'TC-15',
    description: 'Comparison: \\le in conditional (discount for low risk)',
    latex: '\\begin{cases}\n\\mathrm{premium} \\cdot 0.9, & \\text{if } \\mathrm{risk} \\le 0.3 \\\\\n\\mathrm{premium}, & \\text{otherwise}\n\\end{cases}',
    expectedFormulaText: 'if risk <= 0.3 then premium * 0.9 else premium',
    calcCases: [
      { inputs: { risk: '0.1', premium: '1000' }, expectedOutputKey: 'out', expectedValue: '900' },
      { inputs: { risk: '0.3', premium: '1000' }, expectedOutputKey: 'out', expectedValue: '900' },
      { inputs: { risk: '0.31', premium: '1000' }, expectedOutputKey: 'out', expectedValue: '1000' },
      { inputs: { risk: '0.5', premium: '500' }, expectedOutputKey: 'out', expectedValue: '500' },
      { inputs: { risk: '0', premium: '2000' }, expectedOutputKey: 'out', expectedValue: '1800' },
      { inputs: { risk: '1', premium: '800' }, expectedOutputKey: 'out', expectedValue: '800' },
      { inputs: { risk: '0.2', premium: '600' }, expectedOutputKey: 'out', expectedValue: '540' },
      { inputs: { risk: '0.3', premium: '1200' }, expectedOutputKey: 'out', expectedValue: '1080' },
      { inputs: { risk: '0.8', premium: '400' }, expectedOutputKey: 'out', expectedValue: '400' },
      { inputs: { risk: '0.29', premium: '1000' }, expectedOutputKey: 'out', expectedValue: '900' },
    ],
  },

  // TC-16: Comparison != via \ne — embedded in conditional for calculability
  {
    id: 'TC-16',
    description: 'Comparison: \\ne in conditional (bonus flag)',
    latex: '\\begin{cases}\n\\mathrm{bonus}, & \\text{if } \\mathrm{flag} \\ne 0 \\\\\n0, & \\text{otherwise}\n\\end{cases}',
    expectedFormulaText: 'if flag != 0 then bonus else 0',
    calcCases: [
      { inputs: { flag: '1', bonus: '500' }, expectedOutputKey: 'out', expectedValue: '500' },
      { inputs: { flag: '0', bonus: '500' }, expectedOutputKey: 'out', expectedValue: '0' },
      { inputs: { flag: '2', bonus: '200' }, expectedOutputKey: 'out', expectedValue: '200' },
      { inputs: { flag: '-1', bonus: '300' }, expectedOutputKey: 'out', expectedValue: '300' },
      { inputs: { flag: '0', bonus: '100' }, expectedOutputKey: 'out', expectedValue: '0' },
      { inputs: { flag: '100', bonus: '1000' }, expectedOutputKey: 'out', expectedValue: '1000' },
      { inputs: { flag: '0', bonus: '750' }, expectedOutputKey: 'out', expectedValue: '0' },
      { inputs: { flag: '1', bonus: '0' }, expectedOutputKey: 'out', expectedValue: '0' },
      { inputs: { flag: '5', bonus: '600' }, expectedOutputKey: 'out', expectedValue: '600' },
      { inputs: { flag: '0', bonus: '999' }, expectedOutputKey: 'out', expectedValue: '0' },
    ],
  },

  // TC-17: Alternative comparison form \geq — embedded in conditional
  {
    id: 'TC-17',
    description: 'Comparison: \\geq (alternative form) in conditional',
    latex: '\\begin{cases}\n100, & \\text{if } \\mathrm{score} \\geq 90 \\\\\n\\mathrm{score}, & \\text{otherwise}\n\\end{cases}',
    expectedFormulaText: 'if score >= 90 then 100 else score',
    calcCases: [
      { inputs: { score: '95' }, expectedOutputKey: 'out', expectedValue: '100' },
      { inputs: { score: '90' }, expectedOutputKey: 'out', expectedValue: '100' },
      { inputs: { score: '89' }, expectedOutputKey: 'out', expectedValue: '89' },
      { inputs: { score: '100' }, expectedOutputKey: 'out', expectedValue: '100' },
      { inputs: { score: '0' }, expectedOutputKey: 'out', expectedValue: '0' },
      { inputs: { score: '50' }, expectedOutputKey: 'out', expectedValue: '50' },
      { inputs: { score: '89.9' }, expectedOutputKey: 'out', expectedValue: '89.9' },
      { inputs: { score: '90.1' }, expectedOutputKey: 'out', expectedValue: '100' },
      { inputs: { score: '75' }, expectedOutputKey: 'out', expectedValue: '75' },
      { inputs: { score: '91' }, expectedOutputKey: 'out', expectedValue: '100' },
    ],
  },

  // TC-18: Simple conditional via \begin{cases}
  {
    id: 'TC-18',
    description: 'Simple conditional: \\begin{cases}',
    latex: '\\begin{cases}\n1.5, & \\text{if } \\mathrm{age} \\ge 65 \\\\\n1, & \\text{otherwise}\n\\end{cases}',
    expectedFormulaText: 'if age >= 65 then 1.5 else 1',
    calcCases: [
      { inputs: { age: '70' }, expectedOutputKey: 'out', expectedValue: '1.5' },
      { inputs: { age: '65' }, expectedOutputKey: 'out', expectedValue: '1.5' },
      { inputs: { age: '64' }, expectedOutputKey: 'out', expectedValue: '1' },
      { inputs: { age: '30' }, expectedOutputKey: 'out', expectedValue: '1' },
      { inputs: { age: '0' }, expectedOutputKey: 'out', expectedValue: '1' },
      { inputs: { age: '100' }, expectedOutputKey: 'out', expectedValue: '1.5' },
      { inputs: { age: '66' }, expectedOutputKey: 'out', expectedValue: '1.5' },
      { inputs: { age: '64.9' }, expectedOutputKey: 'out', expectedValue: '1' },
      { inputs: { age: '65.1' }, expectedOutputKey: 'out', expectedValue: '1.5' },
      { inputs: { age: '18' }, expectedOutputKey: 'out', expectedValue: '1' },
    ],
  },

  // TC-19: Nested conditional
  {
    id: 'TC-19',
    description: 'Nested conditional: nested \\begin{cases}',
    latex: '\\begin{cases}\n2, & \\text{if } \\mathrm{age} \\ge 65 \\\\\n\\begin{cases}\n1.5, & \\text{if } \\mathrm{age} \\ge 45 \\\\\n1, & \\text{otherwise}\n\\end{cases}, & \\text{otherwise}\n\\end{cases}',
    expectedFormulaText: 'if age >= 65 then 2 else if age >= 45 then 1.5 else 1',
    calcCases: [
      { inputs: { age: '70' }, expectedOutputKey: 'out', expectedValue: '2' },
      { inputs: { age: '65' }, expectedOutputKey: 'out', expectedValue: '2' },
      { inputs: { age: '64' }, expectedOutputKey: 'out', expectedValue: '1.5' },
      { inputs: { age: '50' }, expectedOutputKey: 'out', expectedValue: '1.5' },
      { inputs: { age: '45' }, expectedOutputKey: 'out', expectedValue: '1.5' },
      { inputs: { age: '44' }, expectedOutputKey: 'out', expectedValue: '1' },
      { inputs: { age: '30' }, expectedOutputKey: 'out', expectedValue: '1' },
      { inputs: { age: '0' }, expectedOutputKey: 'out', expectedValue: '1' },
      { inputs: { age: '80' }, expectedOutputKey: 'out', expectedValue: '2' },
      { inputs: { age: '44.9' }, expectedOutputKey: 'out', expectedValue: '1' },
    ],
  },

  // TC-20: Complex compound formula
  {
    id: 'TC-20',
    description: 'Compound: \\frac{sum_insured * rate}{1000}',
    latex: '\\frac{\\mathrm{sum\\_insured} \\cdot \\mathrm{rate}}{1000}',
    expectedFormulaText: '(sum_insured * rate) / (1000)',
    calcCases: [
      { inputs: { sum_insured: '100000', rate: '5' }, expectedOutputKey: 'out', expectedValue: '500' },
      { inputs: { sum_insured: '200000', rate: '3' }, expectedOutputKey: 'out', expectedValue: '600' },
      { inputs: { sum_insured: '500000', rate: '2' }, expectedOutputKey: 'out', expectedValue: '1000' },
      { inputs: { sum_insured: '1000000', rate: '1' }, expectedOutputKey: 'out', expectedValue: '1000' },
      { inputs: { sum_insured: '50000', rate: '10' }, expectedOutputKey: 'out', expectedValue: '500' },
      { inputs: { sum_insured: '300000', rate: '4' }, expectedOutputKey: 'out', expectedValue: '1200' },
      { inputs: { sum_insured: '0', rate: '5' }, expectedOutputKey: 'out', expectedValue: '0' },
      { inputs: { sum_insured: '750000', rate: '2' }, expectedOutputKey: 'out', expectedValue: '1500' },
      { inputs: { sum_insured: '80000', rate: '7.5' }, expectedOutputKey: 'out', expectedValue: '600' },
      { inputs: { sum_insured: '400000', rate: '3.5' }, expectedOutputKey: 'out', expectedValue: '1400' },
    ],
  },
]

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

interface ReportResult {
  id: string
  description: string
  latex: string
  expectedFormulaText: string
  screenshotFile: string
  conversionOk: boolean
  apiFormulaId: string | null
  calcResults: Array<{
    inputs: Record<string, string>
    expected: string
    actual: string
    pass: boolean
  }>
  calcPassCount: number
  errors: string[]
}

const reportResults: ReportResult[] = []

async function getAuthToken(): Promise<string> {
  const res = await fetch(`${API_BASE}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: ADMIN_USER, password: ADMIN_PASS }),
  })
  if (!res.ok) {
    throw new Error(`Login failed: ${res.status} ${await res.text()}`)
  }
  const data = await res.json() as { token: string }
  return data.token
}

async function parseFormulaText(token: string, text: string): Promise<Record<string, unknown>> {
  const res = await fetch(`${API_BASE}/parse`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
    body: JSON.stringify({ text }),
  })
  if (!res.ok) {
    throw new Error(`Parse failed: ${res.status} ${await res.text()}`)
  }
  const data = await res.json() as { graph: Record<string, unknown> }
  return data.graph
}

async function createFormula(token: string, name: string): Promise<string> {
  const res = await fetch(`${API_BASE}/formulas`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
    body: JSON.stringify({ name, domain: 'life', description: `Test formula: ${name}` }),
  })
  if (!res.ok) {
    throw new Error(`Create formula failed: ${res.status} ${await res.text()}`)
  }
  const data = await res.json() as { id: string }
  return data.id
}

async function createVersion(
  token: string,
  formulaId: string,
  graph: Record<string, unknown>
): Promise<number> {
  const res = await fetch(`${API_BASE}/formulas/${formulaId}/versions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
    body: JSON.stringify({ graph, changeNote: 'Initial version' }),
  })
  if (!res.ok) {
    throw new Error(`Create version failed: ${res.status} ${await res.text()}`)
  }
  const data = await res.json() as { version: number }
  return data.version
}

async function publishVersion(token: string, formulaId: string, version: number): Promise<void> {
  const res = await fetch(`${API_BASE}/formulas/${formulaId}/versions/${version}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
    body: JSON.stringify({ state: 'published' }),
  })
  if (!res.ok) {
    throw new Error(`Publish version failed: ${res.status} ${await res.text()}`)
  }
}

async function calculate(
  token: string,
  formulaId: string,
  inputs: Record<string, string>
): Promise<Record<string, string>> {
  const res = await fetch(`${API_BASE}/calculate`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
    body: JSON.stringify({ formulaId, inputs }),
  })
  if (!res.ok) {
    throw new Error(`Calculate failed: ${res.status} ${await res.text()}`)
  }
  const data = await res.json() as { result: Record<string, string> }
  return data.result
}

// ---------------------------------------------------------------------------
// Playwright helpers
// ---------------------------------------------------------------------------

async function loginViaUI(page: Page): Promise<void> {
  await page.goto('/login')
  await page.locator('#username').fill(ADMIN_USER)
  await page.locator('#password').fill(ADMIN_PASS)
  await page.locator('button[type="submit"]').click()
  // Wait for navigation away from /login (might land on '/' or elsewhere)
  await page.waitForFunction(() => !window.location.pathname.includes('/login'), { timeout: 8_000 })
}

async function navigateToEditor(page: Page, formulaId: string): Promise<void> {
  // Navigate in text mode (?mode=text) to avoid React Flow rendering on initial load,
  // which can cause an infinite update loop when all nodes are at position {x:0,y:0}.
  await page.goto(`/formulas/${formulaId}?mode=text`)
  // Wait for the text editor toolbar to appear
  await page.locator('[data-testid="mode-text"]').waitFor({ state: 'visible', timeout: 10_000 })
  // Wait for the LaTeX tab to appear inside the text editor panel
  await page.locator('button:has-text("LaTeX")').waitFor({ state: 'visible', timeout: 5_000 })
}

async function inputLatexAndApply(page: Page, latex: string): Promise<void> {
  // Click the LaTeX tab
  await page.click('button:has-text("LaTeX")')
  await page.waitForTimeout(300)

  // Find the textarea in LaTeX mode
  const textarea = page.locator('textarea[placeholder*="frac"], textarea[placeholder*="mathrm"]').first()
  await textarea.waitFor({ state: 'visible', timeout: 5_000 })
  await textarea.fill(latex)

  // Wait for the KaTeX preview to render
  await page.waitForTimeout(500)
}

async function screenshotLatexPanel(page: Page, filename: string): Promise<void> {
  const screenshotPath = path.join(SCREENSHOTS_DIR, filename)
  await page.screenshot({ path: screenshotPath, fullPage: false })
}

async function applyAndScreenshotGraph(page: Page, filename: string): Promise<void> {
  // Click Apply to Graph button — converts LaTeX → formula text → parse → render graph
  const applyBtn = page.locator('button:has-text("Apply to Graph"), button:has-text("Apply")').first()
  if (await applyBtn.isEnabled()) {
    await applyBtn.click()
    // Switch back to visual mode to see the graph
    const visualBtn = page.locator('[data-testid="mode-visual"]')
    if (await visualBtn.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await visualBtn.click()
      await page.waitForTimeout(1_500)
    }
  }

  // Screenshot the graph area (React Flow canvas in visual mode)
  const graphArea = page.locator('.react-flow__pane, .react-flow__viewport').first()
  const box = await graphArea.boundingBox({ timeout: 5_000 })
  if (box) {
    await page.screenshot({
      path: path.join(SCREENSHOTS_DIR, filename),
      clip: { x: box.x, y: box.y, width: Math.min(box.width, 1000), height: Math.min(box.height, 700) },
    })
  } else {
    // Fallback: screenshot the whole page
    await page.screenshot({ path: path.join(SCREENSHOTS_DIR, filename) })
  }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

let authToken: string

test.beforeAll(async () => {
  fs.mkdirSync(SCREENSHOTS_DIR, { recursive: true })
  fs.mkdirSync(path.dirname(REPORT_DATA_FILE), { recursive: true })
  authToken = await getAuthToken()
})

test.afterAll(() => {
  // Write collected report data for report generation
  fs.writeFileSync(REPORT_DATA_FILE, JSON.stringify(reportResults, null, 2))
})

for (const tc of TEST_CASES) {
  test(`${tc.id}: ${tc.description}`, async ({ page }) => {
    // Cap individual page actions so we don't blow the 60s test timeout
    page.setDefaultTimeout(8_000)
    page.setDefaultNavigationTimeout(10_000)

    const result: ReportResult = {
      id: tc.id,
      description: tc.description,
      latex: tc.latex,
      expectedFormulaText: tc.expectedFormulaText,
      screenshotFile: '',
      conversionOk: false,
      apiFormulaId: null,
      calcResults: [],
      calcPassCount: 0,
      errors: [],
    }

    // ── Step 1: Create formula via API ───────────────────────────────────────

    let formulaId: string
    try {
      const graph = await parseFormulaText(authToken, tc.expectedFormulaText)
      formulaId = await createFormula(authToken, `[018] ${tc.id}`)
      result.apiFormulaId = formulaId
      const version = await createVersion(authToken, formulaId, graph)
      await publishVersion(authToken, formulaId, version)
    } catch (err) {
      result.errors.push(`Formula setup failed: ${(err as Error).message}`)
      reportResults.push(result)
      throw err
    }

    // ── Step 2: Navigate to editor, input LaTeX, screenshot ──────────────────
    // Best-effort: failures here don't abort the calc tests below.

    try {
      // Each test gets a fresh page/context, so always log in
      await loginViaUI(page)
      await navigateToEditor(page, formulaId)
      await inputLatexAndApply(page, tc.latex)

      const latexScreenshotFile = `${tc.id.toLowerCase()}-latex-panel.png`
      result.screenshotFile = latexScreenshotFile
      await screenshotLatexPanel(page, latexScreenshotFile)

      const convertedPre = page.locator('pre').filter({ hasText: tc.expectedFormulaText })
      result.conversionOk = await convertedPre.isVisible({ timeout: 3_000 }).catch(() => false)

      const graphScreenshotFile = `${tc.id.toLowerCase()}-graph.png`
      await applyAndScreenshotGraph(page, graphScreenshotFile)
    } catch (err) {
      result.errors.push(`Screenshot step: ${(err as Error).message}`)
    }

    // ── Step 3: API calculation tests (10 per formula) ───────────────────────

    let passCount = 0
    for (const cc of tc.calcCases) {
      try {
        const resultMap = await calculate(authToken, formulaId, cc.inputs)
        const actualValue = Object.values(resultMap)[0] ?? ''
        // Compare with 6 decimal places tolerance for transcendental functions
        const pass = actualValue === cc.expectedValue
          || Math.abs(parseFloat(actualValue) - parseFloat(cc.expectedValue)) < 0.000001
        if (pass) passCount++
        result.calcResults.push({ inputs: cc.inputs, expected: cc.expectedValue, actual: actualValue, pass })
      } catch (err) {
        result.calcResults.push({ inputs: cc.inputs, expected: cc.expectedValue, actual: 'ERROR', pass: false })
        result.errors.push(`Calc: inputs=${JSON.stringify(cc.inputs)} err=${(err as Error).message}`)
      }
    }
    result.calcPassCount = passCount
    reportResults.push(result)

    // Assert calculation correctness
    expect(passCount, `${tc.id} calc pass count`).toBe(tc.calcCases.length)
  })
}
