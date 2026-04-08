/**
 * Task #018 — Generate Markdown Test Report
 * Run after the E2E tests: node generate-report.mjs
 */

import * as fs from 'fs'
import * as path from 'path'
import { fileURLToPath } from 'url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const DATA_FILE = path.join(__dirname, '../reports/018-data.json')
const REPORT_FILE = path.join(__dirname, '../reports/018-latex-test-report.md')

if (!fs.existsSync(DATA_FILE)) {
  console.error('No test data found. Run E2E tests first: npx playwright test')
  process.exit(1)
}

const results = JSON.parse(fs.readFileSync(DATA_FILE, 'utf-8'))
const now = new Date().toISOString().slice(0, 19).replace('T', ' ')

let totalCalcTests = 0
let passedCalcTests = 0
let conversionPassed = 0
let casesPassed = 0

for (const r of results) {
  totalCalcTests += r.calcResults.length
  passedCalcTests += r.calcPassCount
  if (r.conversionOk) conversionPassed++
  if (r.calcPassCount === r.calcResults.length && r.errors.length === 0) casesPassed++
}

const calcPassRate = totalCalcTests > 0 ? (passedCalcTests / totalCalcTests * 100).toFixed(1) : '0.0'

let md = `# Task #018 LaTeX Formula Input Test Report

**Generated**: ${now}
**Test Cases**: ${results.length}
**Calculation Tests**: ${passedCalcTests}/${totalCalcTests} passed (${calcPassRate}%)
**Conversion Preview Verified**: ${conversionPassed}/${results.length}
**Fully Passed Cases**: ${casesPassed}/${results.length}

---

## Summary Table

| ID | Description | Conversion | Calc Pass | Errors |
|----|-------------|:----------:|:---------:|--------|
`

for (const r of results) {
  const convIcon = r.conversionOk ? '✅' : '⚠️'
  const calcStr = `${r.calcPassCount}/${r.calcResults.length}`
  const calcIcon = r.calcPassCount === r.calcResults.length ? '✅' : '❌'
  const errStr = r.errors.length > 0 ? r.errors[0].slice(0, 60) : '—'
  md += `| ${r.id} | ${r.description} | ${convIcon} | ${calcIcon} ${calcStr} | ${errStr} |\n`
}

md += `\n---\n\n## Detailed Results\n\n`

for (const r of results) {
  const allCalcPass = r.calcPassCount === r.calcResults.length
  const status = allCalcPass && r.errors.length === 0 ? '✅ PASS' : '❌ FAIL'

  md += `### ${r.id}: ${r.description} — ${status}\n\n`
  md += `**LaTeX Input:**\n\`\`\`latex\n${r.latex}\n\`\`\`\n\n`
  md += `**Expected Formula Text:** \`${r.expectedFormulaText}\`  \n`
  md += `**Conversion Preview:** ${r.conversionOk ? '✅ Verified' : '⚠️ Not verified (UI unavailable)'}  \n`
  md += `**Formula ID:** ${r.apiFormulaId ?? 'N/A'}  \n`

  if (r.screenshotFile) {
    md += `**Screenshot:** \`tests/screenshots/018/${r.screenshotFile}\`  \n`
  }
  md += `\n`

  md += `**Calculation Tests** (${r.calcPassCount}/${r.calcResults.length} passed):\n\n`
  md += `| # | Inputs | Expected | Actual | Pass |\n`
  md += `|---|--------|----------|--------|:----:|\n`

  r.calcResults.forEach((c, i) => {
    const inputStr = Object.entries(c.inputs).map(([k, v]) => `${k}=${v}`).join(', ')
    const passIcon = c.pass ? '✅' : '❌'
    md += `| ${i + 1} | \`${inputStr}\` | \`${c.expected}\` | \`${c.actual}\` | ${passIcon} |\n`
  })

  if (r.errors.length > 0) {
    md += `\n**Errors:**\n`
    for (const e of r.errors) {
      md += `- ${e}\n`
    }
  }
  md += `\n`
}

md += `---\n\n## Screenshots\n\nScreenshots are saved in \`tests/screenshots/018/\`:\n\n`
for (const r of results) {
  if (r.screenshotFile) {
    md += `- \`${r.id.toLowerCase()}-latex-panel.png\` — LaTeX input panel for ${r.id}\n`
    md += `- \`${r.id.toLowerCase()}-graph.png\` — Visual formula graph for ${r.id}\n`
  }
}

fs.mkdirSync(path.dirname(REPORT_FILE), { recursive: true })
fs.writeFileSync(REPORT_FILE, md)
console.log(`Report written to: ${REPORT_FILE}`)
console.log(`\nSummary: ${passedCalcTests}/${totalCalcTests} calc tests passed (${calcPassRate}%)`)
