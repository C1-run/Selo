# Selo — brand

## Name

**Selo** — Portuguese for *stamp* / *seal*: the mark that proves something is
authentic. The whole product is that idea applied to AI agent runs — a signed
receipt that anyone can verify.

## Slogan

**Primary — “Don't trust the agent. Verify the receipt.”**

States the wedge in one line: Selo proves what happened by *enforcement and a
signature*, not by the agent's own word.

Alternatives:

| Slogan | Note |
|---|---|
| Seal every agent run. | Ties to the name (*selo* = seal). Short, a touch cryptic alone. |
| Signed proof of what your agent actually did. | Descriptive and clear; longer. |
| Every agent run, on the record. | Evocative; doesn't mention verification. |
| Proof, not promises. | Punchy; generic without context. |

## Marks

| File | Mark | When to use |
|---|---|---|
| `logo.svg` | **Receipt seal** — a receipt with a torn bottom edge and a verification stamp. | **Default.** On-message (receipts), recognizable. Used by the README. |
| `logo-stamp.svg` | **Stamp** — a perforated postage-stamp square with a check. | Alternate. Ties to the name; simplest shape, reads best at favicon size. |
| `logo-seal.svg` | **Seal ring** — a notary-style ring seal with a check. | Alternate. Elegant, but closest to the generic "check in a circle". |

Every mark uses only the brand cyan and accent pink, so it works unchanged on
light and dark backgrounds.

## Wordmark

| File | Background |
|---|---|
| `logo-word-light.svg` | Light (dark “Se”, teal “lo”) |
| `logo-word-dark.svg` | Dark (light “Se”, cyan “lo”) |

Set in a monospace stack (`ui-monospace, SF Mono, Menlo, Consolas`), bold —
it echoes the terminal/receipt subject. Do not place the light variant on a dark
background (the “Se” disappears).

## Palette

| Token | Hex | Use |
|---|---|---|
| Cyan | `#00d4ff` | Primary mark, accents |
| Pink | `#ff007b` | Verification stamp / check accent |
| Ink | `#0a0a14` | Dark text, dark surfaces |
| Paper | `#f4f6fb` | Light text on dark |
| Terminal ink | `#020204` | Detail knocked out of the mark |

## Usage

**README hero** — the wordmark, theme-switched so it stays legible on GitHub light
*and* dark:

```html
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/brand/logo-word-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="docs/brand/logo-word-light.svg">
  <img src="docs/brand/logo-word-light.svg" width="300" alt="Selo">
</picture>
```

**Mark alone** (favicon, avatar, tight spaces):
`<img src="docs/brand/logo.svg" width="110" alt="Selo">`

**GitHub social preview:** `docs/social-banner.svg` / `docs/social-preview.png`

Keep the mark clear-space of at least 25% of its width on all sides.

## Concepts

`../../.internal-docs/brand/preview.html` renders all three marks (light + dark,
large and small) alongside the wordmark and slogan options — kept out of the
public tree as design exploration.
