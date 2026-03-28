import { useState } from 'react'
import { useTranslation } from 'react-i18next'

interface Props {
  value: string
  onChange: (value: string) => void
}

export default function TextEditor({ value, onChange }: Props) {
  const { t } = useTranslation()
  const [localValue, setLocalValue] = useState(value)

  const handleApply = () => {
    onChange(localValue)
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
      <textarea
        value={localValue}
        onChange={(e) => setLocalValue(e.target.value)}
        className="flex-1 p-4 font-mono text-sm resize-none focus:outline-none"
        placeholder="round(lookup(mortalityTable, age) * sumAssured, 18)"
        spellCheck={false}
      />
    </div>
  )
}
