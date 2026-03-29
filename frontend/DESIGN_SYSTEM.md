# KANTOR Design System

## Brand Identity
- **Nama produk**: KANTOR
- **Tagline**: KanA Intelligence Operational dashboaRd
- **Personality**: Confident, energetic, organized — sebuah workspace yang bikin kerja terasa terstruktur tapi tidak membosankan.
- **Logo text**: "KANTOR" — huruf K dan uppercase semua, bold, dengan titik warna primary di atas huruf "A" sebagai identitas visual.

## Design Philosophy
**Colorful corporate** — terang, bersih, tapi berani pakai warna. Setiap modul punya identitas warna sendiri yang kuat dan langsung bisa dibedakan. Background putih/sangat terang supaya warna-warna aksen "pop". Tidak ada elemen yang terasa wireframe atau placeholder.

## Color Palette

### Base Colors
- Background: `#FAFBFC`
- Surface: `#FFFFFF` 
- Surface Elevated: `#FFFFFF` (with shadow)
- Surface Muted: `#F4F5F7`
- Text Primary: `#172B4D`
- Text Secondary: `#5E6C84`
- Text Tertiary: `#97A0AF`
- Border: `#DFE1E6`
- Border Focused: `#4C9AFF`

### Module Identity Colors
**Operasional — Electric Blue**
- Primary: `#0065FF`
- Light: `#DEEBFF`
- Dark: `#0747A6`

**HRIS — Royal Purple**
- Primary: `#6554C0`
- Light: `#EAE6FF`
- Dark: `#403294`

**Marketing — Sunrise Orange**
- Primary: `#FF5630`
- Light: `#FFEBE6`
- Dark: `#BF2600`

### Semantic Colors
- Success: `#36B37E` | Light: `#E3FCEF`
- Warning: `#FFAB00` | Light: `#FFFAE6`
- Error: `#FF5630` | Light: `#FFEBE6`
- Info: `#00B8D9` | Light: `#E6FCFF`

### Priority Colors
- Critical: `#FF5630`
- High: `#FF8B00`
- Medium: `#FFAB00`
- Low: `#36B37E`

### Pipeline Status Colors (Leads)
- New: `#00B8D9`
- Contacted: `#4C9AFF`
- Qualified: `#6554C0`
- Proposal: `#FF8B00`
- Negotiation: `#FFAB00`
- Won: `#36B37E`
- Lost: `#97A0AF`

## Typography
- **Display Font**: Sora (Google Fonts) — headings, stats, branding.
- **Body Font**: Inter (Google Fonts) — body, labels, tables, inputs.
- **Monospace**: JetBrains Mono — numbers, currency, code.

**Scale:**
- Display: 40px/48px, weight 800
- H1: 28px/36px, weight 700
- H2: 22px/28px, weight 700
- H3: 18px/24px, weight 600
- Body LG: 16px/24px, weight 400
- Body: 14px/20px, weight 400
- Body SM: 13px/18px, weight 400
- Caption: 12px/16px, weight 500
- Overline: 11px/16px, weight 700, uppercase, tracking wide

## Spacing & Radius
**Spacing (Base 4px)**: 2, 3, 4, 5, 6, 8, 10, 12, 16
**Radius**: XS (4px), SM (6px), MD (8px), LG (12px), XL (16px), Full (9999px)

## Shadows
- XS: `0 1px 2px rgba(23,43,77,0.04)`
- SM: `0 1px 3px rgba(23,43,77,0.06), 0 1px 2px rgba(23,43,77,0.04)`
- MD: `0 4px 8px -2px rgba(23,43,77,0.08), 0 2px 4px -2px rgba(23,43,77,0.06)`
- LG: `0 8px 16px -4px rgba(23,43,77,0.08), 0 4px 8px -4px rgba(23,43,77,0.06)`
- XL: `0 20px 32px -8px rgba(23,43,77,0.12)`
- Focus Ring: `0 0 0 2px #FFFFFF, 0 0 0 4px [module primary color]`

## Important Rules
1. Colors only from palette.
2. Spacing only from scale.
3. NO placeholder/lorem ipsum text — use real data or realistic mock data.
4. Button colors are contextual to module.
5. All statuses must be badges with dot indicators + text.
6. All currency in Rupiah, JetBrains Mono font.
7. Empty states must have illustration, heading, description, button.
8. Active state in Sidebar uses module color.
