import type { ReactNode } from 'react'
import InfoTip from './InfoTip'
import './InfoTip.css'

// A label with a tip after it:
//
//   <TipLabel text={t('What it does and what changes when you set it.')}>{t('Label')}</TipLabel>
//
// The last word and the button are kept together, so the button never wraps
// onto a line of its own, in a narrow column or on a phone. Give it the label
// as plain text where you can; any other content is kept on one line.
// `text` is the sentence of the InfoTip.
export default function TipLabel({ children, text, label }: { children: ReactNode; text: string; label?: string }) {
  const button = <InfoTip text={text} label={label} />
  if (typeof children === 'string') {
    const m = /^([\s\S]*?)(\S+)\s*$/.exec(children)
    if (m && m[1]) {
      return (
        <span className="has-tip">
          {m[1]}
          <span className="tip-last">
            {m[2]}
            {button}
          </span>
        </span>
      )
    }
  }
  return (
    <span className="has-tip">
      <span className="tip-last">
        {children}
        {button}
      </span>
    </span>
  )
}
