import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import katex from 'katex'
import 'katex/dist/katex.min.css'
import { formulaTextToLatex } from '../../utils/formulaLatex'

interface Props {
  value: string
  onChange: (value: string) => void
}

export default function TextEditor({ value, onChange }: Props) {
  const { t } = useTranslation()
  const [localValue, setLocalValue] = useState(value)

  useEffect(() => {
    setLocalValue(value)
  }, [value])

  const handleApply = () => {
    onChange(localValue)
  }

  let previewHtml = ''
  let previewError: string | null = null

  try {
    const latex = formulaTextToLatex(localValue)
    previewHtml = latex
      ? katex.renderToString(latex, {
          displayMode: true,
          throwOnError: true,
          output: 'html',
          strict: 'ignore',
        })
      : ''
  } catch (err) {
    previewError = (err as Error).message
  }

  return (
    <div className="flex-1 flex flex-col">
      <div className="flex items-center gap-2 p-2 border-b border-gray-200 bg-gray-50">
        <span className="text-sm font-medium text-gray-600">{t('editor.text')}</span>
        <button
          onClick={handleApply}
          className="ml-auto bg-blue-600 text-white px-3 py-1 rounded text-xs hover:bg-blue-700"
        >
          {t('editor.validate')}
        </button>
      </div>
      <div className="grid min-h-0 flex-1 gap-0 lg:grid-cols-[minmax(0,1.1fr)_minmax(320px,0.9fr)]">
        <textarea
          value={localValue}
          onChange={(e) => setLocalValue(e.target.value)}
          className="min-h-[280px] w-full border-r border-gray-200 p-4 font-mono text-sm resize-none focus:outline-none"
          placeholder="round(lookup(mortalityTable, age) * sumAssured, 18)"
          spellCheck={false}
        />
        <div className="flex min-h-[280px] flex-col bg-white">
          <div className="border-b border-gray-200 px-4 py-3">
            <div className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-400">
              {t('editor.latexPreview')}
            </div>
          </div>
          <div className="flex-1 overflow-auto px-4 py-5">
            {previewError ? (
              <div className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-700">
                {t('editor.latexPreviewError')}: {previewError}
              </div>
            ) : previewHtml ? (
              <div
                className="min-w-max rounded-xl border border-slate-200 bg-slate-50 px-6 py-5 text-slate-900 shadow-sm"
                dangerouslySetInnerHTML={{ __html: previewHtml }}
              />
            ) : (
              <div className="rounded-lg border border-dashed border-slate-200 px-3 py-6 text-sm text-slate-400">
                {t('editor.latexPreviewEmpty')}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
