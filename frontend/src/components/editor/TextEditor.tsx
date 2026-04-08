import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import katex from 'katex'
import 'katex/dist/katex.min.css'
import { formulaTextToLatex } from '../../utils/formulaLatex'
import { latexToFormulaText } from '../../utils/latexToFormula'

interface Props {
  value: string
  onChange: (value: string) => void
  isParsing?: boolean
}

type Mode = 'text' | 'latex'

export default function TextEditor({ value, onChange, isParsing }: Props) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<Mode>('text')
  const [localValue, setLocalValue] = useState(value)
  const [latexInput, setLatexInput] = useState('')

  useEffect(() => {
    setLocalValue(value)
  }, [value])

  // ── Text mode: formula text → LaTeX preview ────────────────────────────────

  let textPreviewHtml = ''
  let textPreviewError: string | null = null

  try {
    const latex = formulaTextToLatex(localValue)
    textPreviewHtml = latex
      ? katex.renderToString(latex, {
          displayMode: true,
          throwOnError: true,
          output: 'html',
          strict: 'ignore',
        })
      : ''
  } catch (err) {
    textPreviewError = (err as Error).message
  }

  // ── LaTeX mode: LaTeX → KaTeX preview + converted formula text ─────────────

  let latexPreviewHtml = ''
  let latexPreviewError: string | null = null

  try {
    latexPreviewHtml = latexInput.trim()
      ? katex.renderToString(latexInput, {
          displayMode: true,
          throwOnError: true,
          output: 'html',
          strict: 'ignore',
        })
      : ''
  } catch (err) {
    latexPreviewError = (err as Error).message
  }

  let convertedText = ''
  let convertError: string | null = null

  try {
    convertedText = latexToFormulaText(latexInput)
  } catch (err) {
    convertError = (err as Error).message
  }

  // ── Handlers ────────────────────────────────────────────────────────────────

  const handleApply = () => {
    if (mode === 'latex') {
      if (convertedText) {
        onChange(convertedText)
      }
    } else {
      onChange(localValue)
    }
  }

  // ── Render ──────────────────────────────────────────────────────────────────

  return (
    <div className="flex-1 flex flex-col">
      {/* Header bar */}
      <div className="flex items-center gap-2 p-2 border-b border-gray-200 bg-gray-50">
        {/* Mode tabs */}
        <div className="flex rounded border border-gray-300 overflow-hidden text-xs">
          <button
            onClick={() => setMode('text')}
            className={`px-2.5 py-1 ${mode === 'text' ? 'bg-blue-600 text-white' : 'bg-white text-gray-600 hover:bg-gray-100'}`}
          >
            {t('editor.text')}
          </button>
          <button
            onClick={() => setMode('latex')}
            className={`px-2.5 py-1 border-l border-gray-300 ${mode === 'latex' ? 'bg-blue-600 text-white' : 'bg-white text-gray-600 hover:bg-gray-100'}`}
          >
            LaTeX
          </button>
        </div>
        <button
          onClick={handleApply}
          disabled={isParsing || (mode === 'latex' && !convertedText)}
          className="ml-auto bg-blue-600 text-white px-3 py-1 rounded text-xs hover:bg-blue-700 disabled:opacity-50"
        >
          {isParsing ? t('common.loading') : t('editor.applyToGraph')}
        </button>
      </div>

      {/* Text mode */}
      {mode === 'text' && (
        <div className="flex min-h-0 flex-1 flex-col">
          <div className="flex min-h-0 flex-[1.15] flex-col border-b border-gray-200 bg-white">
            <div className="border-b border-gray-200 px-4 py-3">
              <div className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-400">
                {t('editor.text')}
              </div>
            </div>
            <textarea
              value={localValue}
              onChange={(e) => setLocalValue(e.target.value)}
              className="min-h-[280px] flex-1 w-full overflow-auto p-4 font-mono text-sm resize-none focus:outline-none"
              placeholder="round(lookup(mortalityTable, age) * sumAssured, 18)"
              spellCheck={false}
            />
          </div>
          <div className="flex min-h-0 flex-1 flex-col bg-white">
            <div className="border-b border-gray-200 px-4 py-3">
              <div className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-400">
                {t('editor.latexPreview')}
              </div>
            </div>
            <div className="flex-1 overflow-auto px-4 py-5">
              {textPreviewError ? (
                <div className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-700">
                  {t('editor.latexPreviewError')}: {textPreviewError}
                </div>
              ) : textPreviewHtml ? (
                <div
                  className="min-w-max rounded-xl border border-slate-200 bg-slate-50 px-6 py-5 text-slate-900 shadow-sm"
                  dangerouslySetInnerHTML={{ __html: textPreviewHtml }}
                />
              ) : (
                <div className="rounded-lg border border-dashed border-slate-200 px-3 py-6 text-sm text-slate-400">
                  {t('editor.latexPreviewEmpty')}
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {/* LaTeX mode */}
      {mode === 'latex' && (
        <div className="flex min-h-0 flex-1 flex-col">
          {/* LaTeX input + KaTeX preview */}
          <div className="flex min-h-0 flex-[1.15] flex-col border-b border-gray-200 bg-white">
            <div className="border-b border-gray-200 px-4 py-3">
              <div className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-400">
                {t('editor.latexInput')}
              </div>
            </div>
            <textarea
              value={latexInput}
              onChange={(e) => setLatexInput(e.target.value)}
              className="min-h-[180px] flex-1 w-full overflow-auto p-4 font-mono text-sm resize-none focus:outline-none"
              placeholder="\frac{\mathrm{a} + \mathrm{b}}{2}"
              spellCheck={false}
            />
            <div className="border-t border-gray-100 px-4 py-3">
              {latexPreviewError ? (
                <div className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-700">
                  {t('editor.latexPreviewError')}: {latexPreviewError}
                </div>
              ) : latexPreviewHtml ? (
                <div
                  className="min-w-max rounded-xl border border-slate-200 bg-slate-50 px-6 py-5 text-slate-900 shadow-sm"
                  dangerouslySetInnerHTML={{ __html: latexPreviewHtml }}
                />
              ) : (
                <div className="rounded-lg border border-dashed border-slate-200 px-3 py-6 text-sm text-slate-400 text-center">
                  {t('editor.latexPreviewEmpty')}
                </div>
              )}
            </div>
          </div>

          {/* Converted formula text preview */}
          <div className="flex min-h-0 flex-1 flex-col bg-white">
            <div className="border-b border-gray-200 px-4 py-3">
              <div className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-400">
                {t('editor.convertedText')}
              </div>
            </div>
            <div className="flex-1 overflow-auto px-4 py-4">
              {convertError ? (
                <div className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
                  {t('editor.latexConvertError')}: {convertError}
                </div>
              ) : convertedText ? (
                <pre className="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 font-mono text-sm text-emerald-900 whitespace-pre-wrap">
                  {convertedText}
                </pre>
              ) : (
                <div className="rounded-lg border border-dashed border-slate-200 px-3 py-6 text-sm text-slate-400 text-center">
                  {t('editor.latexPreviewEmpty')}
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
