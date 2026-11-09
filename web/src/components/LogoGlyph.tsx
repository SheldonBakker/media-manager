import { useId } from 'react'

// The Mediarium arch as a solid object: lit from the top, a darker edge, and a
// slab of depth behind it, so it sits with the media shapes (KindGlyph's
// "solid" variant) instead of looking like a flat sticker. Takes its colour
// from the current text colour; the .g-logo class sets it to the accent.
export default function LogoGlyph({ size = 64, className = '' }: { size?: number; className?: string }) {
  const id = useId().replace(/:/g, '')
  const arch = 'M7 5h18a4 4 0 0 1 4 4v14a4 4 0 0 1-4 4H7a4 4 0 0 1-4-4V9a4 4 0 0 1 4-4zM13 10.5v11l9-5.5z'
  return (
    <svg className={`kind-glyph solid-glyph g-logo ${className}`.trim()} width={size} height={size} viewBox="0 0 32 32" fill="none" strokeWidth={1.2} strokeLinejoin="round" aria-hidden="true">
      <defs>
        <linearGradient id={`l${id}`} x1="0" y1="0" x2="0.35" y2="1">
          <stop offset="0" className="g-top" />
          <stop offset="0.55" className="g-mid" />
          <stop offset="1" className="g-low" />
        </linearGradient>
      </defs>
      <path className="logo-depth" fillRule="evenodd" d={arch} transform="translate(1.1 1.5)" />
      <path className="logo-face" fillRule="evenodd" d={arch} fill={`url(#l${id})`} />
      <path className="shine" d="M5.6 10a3.4 3.4 0 0 1 2.9-2.9" />
    </svg>
  )
}
