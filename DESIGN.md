# Agent VM Design System

> Cursor-like developer-tool polish for Agent VM: dark editor surfaces, precise
> typography, cool gradients, honest CLI state, and portable-agent diagrams.

This file is the visual source of truth for README assets, future docs pages,
social preview images, and any UI work around Agent VM. It is inspired by the
Cursor-like direction from getdesign.md, but adapted for AVM's own product:
local-first AI agent profile virtualization.

## 1. Brand Position

Agent VM should feel like infrastructure for serious AI coding users:

- technical, not playful
- premium, not corporate
- local-first and safe, not cloud-platform vague
- sharp enough to be memorable on GitHub
- honest about implemented vs planned behavior

Core phrase:

```text
nvm for AI coding agents.
```

Supporting phrase:

```text
One portable profile for tools, permissions, model settings, and runtime choice.
```

Do not over-explain the product in the hero. The first screen should make one
thing clear: AVM turns scattered agent setup into a named profile.

## 2. Visual Theme

AVM uses a dark developer-tool atmosphere with cool cyan, blue, and amber
accents. The visual language should resemble a high-end code editor or terminal
control plane.

Key characteristics:

- dark navy / ink backgrounds
- subtle grid texture for technical depth
- cyan and blue gradients for "routing" and "activation"
- amber accents for warnings and highlight state
- dark terminal cards with mono text
- clean runtime pills for Codex, Claude Code, OpenClaw, Hermes Agent
- precise alignment; no text should touch or overflow a card edge

Avoid:

- generic SaaS purple gradients
- decorative blob backgrounds
- cartoon agent imagery
- mascot art
- vague AI sparkle overload
- large marketing cards nested inside other cards

## 3. Color Palette

### Core

| Token | Hex | Use |
| --- | --- | --- |
| `avm-ink` | `#09111F` | page and hero background |
| `avm-navy` | `#111827` | panels, editor chrome |
| `avm-panel` | `#0D1726` | inner cards and code surfaces |
| `avm-terminal` | `#07111F` | terminal blocks |
| `avm-text` | `#F8FAFC` | primary text |
| `avm-muted` | `#B7C7D8` | supporting copy |
| `avm-border` | `#344254` | panel borders |

### Accents

| Token | Hex | Use |
| --- | --- | --- |
| `avm-cyan` | `#5EEAD4` | activation, command prompts, primary accent |
| `avm-blue` | `#93C5FD` | Codex, links, cool gradient midpoint |
| `avm-amber` | `#FDE68A` | warnings, highlight state |
| `avm-green` | `#10B981` | OpenClaw, success, available |
| `avm-violet` | `#A78BFA` | Hermes Agent, secondary runtime |
| `avm-orange` | `#F59E0B` | Claude Code, caution, warm contrast |

### Light Surfaces

Use light surfaces only for comparison diagrams and docs pages.

| Token | Hex | Use |
| --- | --- | --- |
| `avm-paper` | `#F8FAFC` | light background |
| `avm-mint-paper` | `#E7F7F4` | light gradient endpoint |
| `avm-paper-text` | `#0F172A` | light-mode headline |
| `avm-paper-muted` | `#475569` | light-mode body |

## 4. Typography

Use system fonts so GitHub SVG rendering stays reliable.

### Families

```css
--font-sans: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont,
  "Segoe UI", sans-serif;
--font-mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
```

### Rules

- Display headings: 48-58px in SVG assets, weight 800.
- README HTML headings: use native Markdown/HTML, do not over-style.
- Body copy: 18-24px in graphics, 16px in docs.
- Code/terminal: mono, 16-22px depending on canvas size.
- Letter spacing: `0`; do not use negative tracking in SVG assets.
- Prefer short line breaks over relying on automatic SVG wrapping.

SVG text must be manually line-broken. Never place long copy in one `<text>`
element unless there is at least 80px of right-side padding.

## 5. Layout System

### Canvas Sizes

| Asset | Size | Notes |
| --- | --- | --- |
| README hero | `1200 x 630` | also suitable as social preview source |
| Before/after | `1200 x 520` | README problem-solution visual |
| Social preview export | `1200 x 630` PNG | generated from hero SVG when needed |
| Docs screenshot | `1440 x 900` | future browser screenshots |

### Spacing

- Outer margin in 1200px graphics: 60-80px.
- Card padding: 32-44px.
- Terminal padding: 22-30px.
- Runtime pill minimum width: 150px.
- Leave at least 24px between text and card borders.
- Leave at least 48px between large visual groups.

### Composition

Use two strong compositions:

1. Hero: left-side positioning statement, right-side profile-to-runtimes diagram.
2. Before/After: left scattered files, right single activation command.

These are the core mental models. Reuse them before inventing new metaphors.

## 6. Components

### Terminal Card

Use for CLI examples and activation state.

- Background: `#07111F`
- Header: `#101A2A`
- Border: `#314158`
- Radius: 18px
- Prompt text: `#5EEAD4`
- State text: `#E2E8F0`
- Warning/state highlight: `#FDE68A`

Keep terminal commands short:

```text
$ avm use backend-coder
profile  backend-coder
runtime  codex
```

Avoid long file paths in hero graphics. Put long paths in README code blocks.

### Runtime Pill

Use for target runtime labels.

- Width: at least 150px.
- Height: 56px.
- Radius: 14px.
- Text: centered, font weight 700.

Runtime colors:

| Runtime | Fill | Stroke | Text |
| --- | --- | --- | --- |
| Codex | `#102033` | `#3B82F6` | `#DBEAFE` |
| Claude Code | `#211A10` | `#F59E0B` | `#FEF3C7` |
| OpenClaw | `#0D241D` | `#10B981` | `#D1FAE5` |
| Hermes Agent | `#1F1B2E` | `#A78BFA` | `#EDE9FE` |

### Profile Card

Use for showing the portable object.

- Background: `#0D1726`
- Border: `#3B4D63`
- Radius: 16px.
- Title: profile filename or profile name.
- Secondary line: short capability summary.

Good:

```text
backend-coder.yaml
skills: git, test
```

Bad:

```text
backend-coder-with-many-capabilities-and-runtime-overrides.yaml
```

### Before/After Panel

Use a light background to increase contrast from the dark hero.

- Background gradient: `#F8FAFC` to `#E7F7F4`
- Left card: white with slate text.
- Right card: `#0F172A` with cyan border.
- Arrow: teal stroke.

Long descriptions must be split into 2-3 lines.

## 7. README Rules

The README is a conversion surface, not only documentation.

Order:

1. Hero image
2. Project name
3. one-line positioning
4. badges
5. language switcher
6. short product thesis
7. before/after visual
8. core command
9. differentiation table
10. recipes
11. status and quickstart

Tone:

- Lead with the concept, not implementation details.
- Keep "working today" and "in progress" separate.
- Do not imply installability until release packaging exists.
- Avoid claiming runtime writes work before adapters land.

## 8. Multilingual Docs

Maintain:

- `README.md` as the primary English entry.
- `README.zh-CN.md` as the Simplified Chinese entry.
- `README.ja.md` as the Japanese entry.
- `README.ko.md` as the Korean entry.
- `README.es.md` as the Spanish entry.
- `README.pt-BR.md` as the Brazilian Portuguese entry.
- `README.fr.md` as the French entry.

Both files must stay aligned on:

- implemented commands
- planned commands
- install status
- safety model
- roadmap phase names

Localized copy may be more direct and product-oriented. It does not need to be
a literal translation, but it must not promise more than the English README.

## 9. SVG Quality Checklist

Before committing SVG assets:

- Run `xmllint --noout assets/*.svg`.
- Render with Chrome or another browser, not only XML validation.
- Check that no text overflows its card.
- Check that no text is hidden under another panel.
- Check at README width and social-preview width.
- Keep all text selectable and editable in SVG unless a raster export is
  intentionally required.

Chrome preview command:

```bash
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
  --headless --disable-gpu --no-sandbox \
  --window-size=1200,630 \
  --screenshot=/tmp/avm-hero.png \
  file://$PWD/assets/avm-hero.svg
```

## 10. Future Landing Page Direction

If AVM gets a website, the first page should be a usable product landing page,
not a generic docs splash.

First viewport:

- full-bleed dark hero
- terminal activation command
- profile-to-runtimes diagram
- CTA: "Star on GitHub" and "Read the quickstart"

Second viewport:

- before/after scattered config visual
- runtime support matrix
- recipes
- safety model

Do not use floating nested cards for every section. Use full-width bands with
constrained inner content and a small number of strong visual anchors.
