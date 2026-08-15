// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import BatchTestPage from './BatchTestPage'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

vi.mock('../../api/formulas', () => ({
  listFormulas: vi.fn().mockResolvedValue({ formulas: [], total: 0 }),
}))

vi.mock('../../api/client', () => ({
  api: { get: vi.fn().mockResolvedValue({ versions: [] }) },
}))

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <BatchTestPage />
    </QueryClientProvider>,
  )
}

function uploadCsv(csv: string) {
  const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement
  const file = new File([csv], 'batch.csv', { type: 'text/csv' })
  fireEvent.change(fileInput, { target: { files: [file] } })
}

afterEach(() => {
  cleanup()
})

describe('BatchTestPage CSV import', () => {
  it('parses RFC 4180 quoting, BOM, CRLF, escaped quotes, and embedded newlines without changing values', async () => {
    renderPage()
    uploadCsv(
      '\ufefflabel,age,note,expected_premium\r\n"case, ""quoted""",35,"first line\r\nsecond line",42.50\r\n',
    )

    const textarea = screen.getByRole('textbox')
    await waitFor(() => expect((textarea as HTMLTextAreaElement).value).not.toBe(''))

    expect(JSON.parse((textarea as HTMLTextAreaElement).value)).toEqual([
      {
        label: 'case, "quoted"',
        inputs: { age: '35', note: 'first line\r\nsecond line' },
        expected: { premium: '42.50' },
      },
    ])
  })

  it('shows an import error for an unterminated quoted field instead of silently constructing corrupted cases', async () => {
    renderPage()
    uploadCsv('label,age,expected_premium\r\n"unterminated,35,42.50')

    await waitFor(() => {
      expect(screen.getByText(/unterminated quoted CSV field/i)).toBeTruthy()
    })
  })
})
