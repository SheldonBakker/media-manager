// Renders the Media Manager icon set and share images into rebrand/assets.
// Run from the repo root, pointing RESVG_DIR at a folder where `npm i @resvg/resvg-js` was run:
//   RESVG_DIR=/path/to/dir node rebrand/make-assets.mjs
// The output is committed, so this only needs re-running when the mark changes.
import { createRequire } from 'node:module'
import { mkdirSync, writeFileSync } from 'node:fs'

const { Resvg } = createRequire(`${process.env.RESVG_DIR ?? process.cwd()}/`)('@resvg/resvg-js')

const OUT = 'rebrand/assets'
const FONT = 'web/public/fonts/WixMadeforDisplay-Variable.ttf'
const BG = '#0b0d18'
const ACCENT = '#2dd4bf'

// The mark: a screen with a play triangle cut out (even-odd fill), 32x32 grid.
export const MARK = 'M7 5h18a4 4 0 0 1 4 4v14a4 4 0 0 1-4 4H7a4 4 0 0 1-4-4V9a4 4 0 0 1 4-4zM13 10.5v11l9-5.5z'

mkdirSync(OUT, { recursive: true })

const tile = (pad) => `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="32" height="32"><rect x=".5" y=".5" width="31" height="31" rx="7" fill="#161A21" stroke="#2A303B"/><g transform="translate(16 16) scale(${pad}) translate(-16 -16)"><path d="${MARK}" fill="#34D1BF" fill-rule="evenodd"/></g></svg>`
const maskable = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="32" height="32"><rect width="32" height="32" fill="${BG}"/><g transform="translate(16 16) scale(.6) translate(-16 -16)"><path d="${MARK}" fill="#34D1BF" fill-rule="evenodd"/></g></svg>`

const png = (svg, w, h = w) =>
  new Resvg(svg, { fitTo: { mode: 'width', value: w }, font: { fontFiles: [FONT], loadSystemFonts: false, defaultFontFamily: 'Wix Madefor Display' } }).render().asPng()

writeFileSync(`${OUT}/favicon.svg`, tile(0.84))
writeFileSync(`${OUT}/mark.svg`, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><path d="${MARK}" fill="currentColor" fill-rule="evenodd"/></svg>\n`)
for (const s of [16, 32, 48, 180, 192, 512]) {
  const name = s === 180 ? 'apple-touch-icon-180.png' : s <= 48 ? `favicon-${s}.png` : `icon-${s}.png`
  writeFileSync(`${OUT}/${name}`, png(tile(0.84), s))
}
writeFileSync(`${OUT}/icon-maskable-512.png`, png(maskable, 512))

// favicon.ico holding the 16/32/48 PNGs (ICO allows embedded PNG).
const sizes = [16, 32, 48]
const imgs = sizes.map((s) => png(tile(0.84), s))
const header = Buffer.alloc(6 + 16 * imgs.length)
header.writeUInt16LE(0, 0); header.writeUInt16LE(1, 2); header.writeUInt16LE(imgs.length, 4)
let offset = header.length
imgs.forEach((img, i) => {
  const e = 6 + 16 * i
  header.writeUInt8(sizes[i], e); header.writeUInt8(sizes[i], e + 1)
  header.writeUInt16LE(1, e + 4); header.writeUInt16LE(32, e + 6)
  header.writeUInt32LE(img.length, e + 8); header.writeUInt32LE(offset, e + 12)
  offset += img.length
})
writeFileSync(`${OUT}/favicon.ico`, Buffer.concat([header, ...imgs]))

// Share images: mark + name on the app background.
const share = (w, h) => {
  const m = Math.min(w, h) * 0.28
  const fs = Math.min(w, h) * 0.11
  const cy = h / 2 - fs * 0.6
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}">
  <defs><radialGradient id="g" cx="50%" cy="40%" r="60%"><stop offset="0" stop-color="${ACCENT}" stop-opacity=".14"/><stop offset="1" stop-color="${ACCENT}" stop-opacity="0"/></radialGradient></defs>
  <rect width="${w}" height="${h}" fill="${BG}"/><rect width="${w}" height="${h}" fill="url(#g)"/>
  <g transform="translate(${w / 2 - m / 2} ${cy - m}) scale(${m / 32})"><path d="${MARK}" fill="${ACCENT}" fill-rule="evenodd"/></g>
  <text x="${w / 2}" y="${cy + fs * 1.3}" text-anchor="middle" font-family="Wix Madefor Display" font-weight="800" font-size="${fs}" fill="#eef0fa">Media Manager</text>
  <text x="${w / 2}" y="${cy + fs * 2.3}" text-anchor="middle" font-family="Wix Madefor Display" font-size="${fs * 0.38}" fill="#979db8">Movies, TV, music and books</text>
</svg>`
}
for (const [id, w, h] of [['wide', 1200, 630], ['square', 1080, 1080], ['story', 1080, 1920]]) {
  writeFileSync(`${OUT}/share-${id}.png`, png(share(w, h), w, h))
}
console.log('assets written to', OUT)
