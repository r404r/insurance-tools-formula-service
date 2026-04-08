import { test } from '@playwright/test'
import * as path from 'path'
import * as fs from 'fs'

test('debug login and navigation', async ({ page }) => {
  page.setDefaultTimeout(15_000)
  page.setDefaultNavigationTimeout(15_000)

  // Capture browser console + errors
  page.on('console', msg => console.log('BROWSER LOG:', msg.type(), msg.text()))
  page.on('pageerror', err => console.log('PAGE ERROR:', err.message))

  // Login
  await page.goto('/login')
  await page.screenshot({ path: '/tmp/debug-01-login.png' })
  await page.locator('#username').fill('admin')
  await page.locator('#password').fill('admin99999')
  await page.locator('button[type="submit"]').click()
  await page.waitForFunction(() => !window.location.pathname.includes('/login'), { timeout: 10_000 })
  await page.screenshot({ path: '/tmp/debug-02-after-login.png' })
  console.log('URL after login:', page.url())

  // Navigate to a formula
  const TOKEN = 'test'
  const res = await fetch('http://localhost:8080/api/v1/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: 'admin', password: 'admin99999' })
  })
  const { token } = await res.json() as { token: string }

  const fres = await fetch('http://localhost:8080/api/v1/formulas', {
    headers: { Authorization: `Bearer ${token}` }
  })
  const fdata = await fres.json() as { formulas?: Array<{id: string, name: string}> }
  const formulas = fdata.formulas || []
  const match = formulas.find((f: any) => f.name.includes('018') && f.name.includes('TC-01'))
  const formulaId = match?.id || formulas[0]?.id
  console.log('Formula ID:', formulaId)

  await page.goto(`/formulas/${formulaId}?mode=text`)
  await page.waitForTimeout(3000)
  await page.screenshot({ path: '/tmp/debug-03-editor.png' })
  console.log('URL after navigate to editor:', page.url())

  // Check localStorage for auth token
  const authStore = await page.evaluate(() => {
    const keys = Object.keys(localStorage)
    const result: Record<string, string> = {}
    for (const k of keys) result[k] = localStorage.getItem(k) ?? ''
    return result
  })
  console.log('localStorage keys:', Object.keys(authStore))

  // Check document body
  const bodyText = await page.locator('body').innerText().catch(() => 'ERROR reading body')
  console.log('Body text (first 200):', bodyText.slice(0, 200))

  // Try to find mode-text button
  const allButtons = await page.locator('button').allTextContents()
  console.log('All buttons:', allButtons.slice(0, 20))
})
