import { useState, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation } from '@tanstack/react-query'
import { listFormulas } from '../../api/formulas'
import { api } from '../../api/client'
import { batchTest } from '../../api/calculations'
import type { BatchTestCase, BatchTestResponse, FormulaVersion } from '../../types/formula'

// ── CSV parser ────────────────────────────────────────────────────────────────

function parseCsv(text: string): BatchTestCase[] {
  const lines = text.trim().split('\n').filter((l) => l.trim())
  if (lines.length < 2) throw new Error('CSV must have a header row and at least one data row')

  const headers = lines[0].split(',').map((h) => h.trim())
  const expectedKeys = headers.filter((h) => h.startsWith('expected_'))
  const inputKeys = headers.filter((h) => h !== 'label' && !h.startsWith('expected_'))

  return lines.slice(1).map((line, i) => {
    const values = line.split(',').map((v) => v.trim())
    const row: Record<string, string> = {}
    headers.forEach((h, idx) => { row[h] = values[idx] ?? '' })

    const inputs: Record<string, string> = {}
    inputKeys.forEach((k) => { inputs[k] = row[k] })

    const expected: Record<string, string> = {}
    expectedKeys.forEach((k) => { expected[k.replace(/^expected_/, '')] = row[k] })

    return { label: row['label'] || `Row ${i + 2}`, inputs, expected }
  })
}

// ── component ─────────────────────────────────────────────────────────────────

export default function BatchTestPage() {
  const { t } = useTranslation()
  const fileRef = useRef<HTMLInputElement>(null)

  const [formulaId, setFormulaId] = useState('')
  const [versionNum, setVersionNum] = useState<number | undefined>(undefined)
  const [tolerance, setTolerance] = useState('0')
  const [jsonText, setJsonText] = useState('')
  const [parseError, setParseError] = useState('')
  const [result, setResult] = useState<BatchTestResponse | null>(null)

  // All formulas (for selector)
  const { data: formulasData } = useQuery({
    queryKey: ['formulas-all'],
    queryFn: () => listFormulas({ pageSize: 200 }),
  })

  // Versions for selected formula
  const { data: versionsData } = useQuery({
    queryKey: ['versions', formulaId],
    queryFn: () =>
      api.get<{ versions: FormulaVersion[] }>(`/formulas/${formulaId}/versions`),
    enabled: !!formulaId,
  })
  const versions = versionsData?.versions ?? []
  const publishedVersions = versions.filter((v) => v.state === 'published')

  const runMutation = useMutation({
    mutationFn: batchTest,
    onSuccess: (data) => setResult(data),
  })

  function handleFormulaChange(id: string) {
    setFormulaId(id)
    setVersionNum(undefined)
    setResult(null)
  }

  function handleFileUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = (ev) => {
      const text = ev.target?.result as string
      if (file.name.endsWith('.csv')) {
        try {
          const cases = parseCsv(text)
          setJsonText(JSON.stringify(cases, null, 2))
          setParseError('')
        } catch (err) {
          setParseError(String(err))
        }
      } else {
        setJsonText(text)
        setParseError('')
      }
    }
    reader.readAsText(file)
    // reset so same file can be re-uploaded
    e.target.value = ''
  }

  function handleRun() {
    setParseError('')
    setResult(null)
    let cases: BatchTestCase[]
    try {
      cases = JSON.parse(jsonText)
      if (!Array.isArray(cases)) throw new Error('JSON must be an array')
    } catch (err) {
      setParseError(t('batchTest.parseError') + ': ' + String(err))
      return
    }
    runMutation.mutate({ formulaId, version: versionNum, tolerance, cases })
  }

  const summary = result?.summary
  const passRateColor =
    !summary ? '' :
    summary.passRate === 100 ? 'text-green-600' :
    summary.passRate >= 80   ? 'text-yellow-600' : 'text-red-600'

  return (
    <div className="mx-auto max-w-5xl px-6 py-8">
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-900">{t('batchTest.title')}</h1>
        <p className="mt-2 text-sm text-gray-500">{t('batchTest.subtitle')}</p>
      </div>

      {/* Config panel */}
      <div className="mb-6 rounded-2xl border border-gray-200 bg-white p-6 shadow-sm">
        <h2 className="mb-4 text-base font-semibold text-gray-900">{t('batchTest.config')}</h2>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          {/* Formula */}
          <div>
            <label className="mb-1 block text-xs font-medium text-gray-600">{t('batchTest.formula')}</label>
            <select
              value={formulaId}
              onChange={(e) => handleFormulaChange(e.target.value)}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
            >
              <option value="">{t('batchTest.selectFormula')}</option>
              {formulasData?.formulas.map((f) => (
                <option key={f.id} value={f.id}>{f.name}</option>
              ))}
            </select>
          </div>

          {/* Version */}
          <div>
            <label className="mb-1 block text-xs font-medium text-gray-600">{t('batchTest.version')}</label>
            <select
              value={versionNum ?? ''}
              onChange={(e) => setVersionNum(e.target.value ? Number(e.target.value) : undefined)}
              disabled={!formulaId}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500 disabled:opacity-50"
            >
              <option value="">{t('batchTest.latestPublished')}</option>
              {publishedVersions.map((v) => (
                <option key={v.version} value={v.version}>v{v.version}</option>
              ))}
            </select>
          </div>

          {/* Tolerance */}
          <div>
            <label className="mb-1 block text-xs font-medium text-gray-600">
              {t('batchTest.tolerance')}
            </label>
            <div className="flex items-center gap-1">
              <input
                type="number"
                min="0"
                step="0.001"
                value={tolerance}
                onChange={(e) => setTolerance(e.target.value)}
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
              />
              <span className="text-sm text-gray-400">{t('batchTest.toleranceHint')}</span>
            </div>
          </div>
        </div>
      </div>

      {/* Data input panel */}
      <div className="mb-6 rounded-2xl border border-gray-200 bg-white p-6 shadow-sm">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-base font-semibold text-gray-900">{t('batchTest.testData')}</h2>
          <div className="flex gap-2">
            <button
              onClick={() => fileRef.current?.click()}
              className="rounded-lg border border-gray-300 px-3 py-1.5 text-xs font-medium text-gray-600 hover:bg-gray-50"
            >
              {t('batchTest.uploadFile')}
            </button>
            <input ref={fileRef} type="file" accept=".json,.csv" className="hidden" onChange={handleFileUpload} />
          </div>
        </div>

        <p className="mb-2 text-xs text-gray-400">{t('batchTest.dataHint')}</p>
        <textarea
          value={jsonText}
          onChange={(e) => { setJsonText(e.target.value); setParseError('') }}
          rows={10}
          placeholder={t('batchTest.placeholder')}
          className="w-full rounded-lg border border-gray-300 px-3 py-2 font-mono text-xs focus:outline-none focus:ring-2 focus:ring-indigo-500"
        />
        {parseError && (
          <p className="mt-1 text-xs text-red-500">{parseError}</p>
        )}

        <div className="mt-4 flex justify-end">
          <button
            onClick={handleRun}
            disabled={!formulaId || !jsonText.trim() || runMutation.isPending}
            className="rounded-lg bg-indigo-600 px-5 py-2 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-50"
          >
            {runMutation.isPending ? t('common.loading') : t('batchTest.run')}
          </button>
        </div>

        {runMutation.isError && (
          <p className="mt-2 text-sm text-red-500">{String(runMutation.error)}</p>
        )}
      </div>

      {/* Results */}
      {result && (
        <>
          {/* Summary cards */}
          <div className="mb-4 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
            {[
              { label: t('batchTest.total'),     value: summary!.total,  color: 'text-gray-900' },
              { label: t('batchTest.passed'),    value: summary!.passed, color: 'text-green-600' },
              { label: t('batchTest.failed'),    value: summary!.failed, color: 'text-red-600' },
              { label: t('batchTest.passRate'),  value: `${summary!.passRate.toFixed(1)}%`, color: passRateColor },
              { label: t('batchTest.totalTime'), value: `${summary!.totalExecutionTimeMs.toFixed(0)} ms`, color: 'text-gray-700' },
            ].map(({ label, value, color }) => (
              <div key={label} className="rounded-xl border border-gray-200 bg-white p-4 shadow-sm">
                <p className="text-xs text-gray-500">{label}</p>
                <p className={`mt-1 text-2xl font-bold ${color}`}>{value}</p>
              </div>
            ))}
          </div>

          {/* Detail table */}
          <div className="overflow-x-auto rounded-2xl border border-gray-200 bg-white shadow-sm">
            <table className="min-w-full text-sm">
              <thead className="bg-gray-50 text-xs text-gray-500">
                <tr>
                  <th className="px-4 py-3 text-left">#</th>
                  <th className="px-4 py-3 text-left">{t('batchTest.label')}</th>
                  <th className="px-4 py-3 text-left">{t('batchTest.status')}</th>
                  <th className="px-4 py-3 text-left">{t('batchTest.expected')}</th>
                  <th className="px-4 py-3 text-left">{t('batchTest.actual')}</th>
                  <th className="px-4 py-3 text-left">{t('batchTest.diff')}</th>
                  <th className="px-4 py-3 text-right">{t('batchTest.timeMs')}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {result.results.map((row) => (
                  <tr key={row.index} className={row.pass ? '' : 'bg-red-50'}>
                    <td className="px-4 py-2 text-gray-400">{row.index}</td>
                    <td className="px-4 py-2 text-gray-700">{row.label || '—'}</td>
                    <td className="px-4 py-2">
                      {row.error ? (
                        <span className="rounded-full bg-red-100 px-2 py-0.5 text-xs font-medium text-red-700">ERROR</span>
                      ) : row.pass ? (
                        <span className="rounded-full bg-green-100 px-2 py-0.5 text-xs font-medium text-green-700">PASS</span>
                      ) : (
                        <span className="rounded-full bg-red-100 px-2 py-0.5 text-xs font-medium text-red-700">FAIL</span>
                      )}
                    </td>
                    <td className="px-4 py-2 font-mono text-xs text-gray-600">
                      {Object.entries(row.expected).map(([k, v]) => (
                        <div key={k}><span className="text-gray-400">{k}=</span>{v}</div>
                      ))}
                    </td>
                    <td className="px-4 py-2 font-mono text-xs">
                      {row.error ? (
                        <span className="text-red-500">{row.error}</span>
                      ) : (
                        Object.entries(row.expected).map(([k]) => {
                          const val = row.actual[k]
                          const fail = row.diff?.[k]
                          return (
                            <div key={k} className={fail ? 'text-red-600' : 'text-gray-600'}>
                              <span className="text-gray-400">{k}=</span>{val ?? '—'}
                            </div>
                          )
                        })
                      )}
                    </td>
                    <td className="px-4 py-2 font-mono text-xs text-red-500">
                      {row.diff
                        ? Object.entries(row.diff).map(([k, d]) => (
                            <div key={k}>{k}: {d}</div>
                          ))
                        : '—'}
                    </td>
                    <td className="px-4 py-2 text-right text-xs text-gray-500">
                      {row.executionTimeMs.toFixed(3)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  )
}
