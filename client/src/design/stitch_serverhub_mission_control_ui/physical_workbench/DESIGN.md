---
name: Physical Workbench
colors:
  surface: '#fff8f5'
  surface-dim: '#e2d8d2'
  surface-bright: '#fff8f5'
  surface-container-lowest: '#ffffff'
  surface-container-low: '#fcf2eb'
  surface-container: '#f6ece6'
  surface-container-high: '#f0e6e0'
  surface-container-highest: '#eae1da'
  on-surface: '#1f1b17'
  on-surface-variant: '#5a4138'
  inverse-surface: '#342f2b'
  inverse-on-surface: '#f9efe8'
  outline: '#8e7166'
  outline-variant: '#e2bfb2'
  surface-tint: '#a73a00'
  primary: '#a33900'
  on-primary: '#ffffff'
  primary-container: '#cc4900'
  on-primary-container: '#fffbff'
  inverse-primary: '#ffb599'
  secondary: '#625d5b'
  on-secondary: '#ffffff'
  secondary-container: '#e9e1dd'
  on-secondary-container: '#686361'
  tertiary: '#665f3d'
  on-tertiary: '#ffffff'
  tertiary-container: '#b5ac84'
  on-tertiary-container: '#464021'
  error: '#ba1a1a'
  on-error: '#ffffff'
  error-container: '#ffdad6'
  on-error-container: '#93000a'
  primary-fixed: '#ffdbce'
  primary-fixed-dim: '#ffb599'
  on-primary-fixed: '#370e00'
  on-primary-fixed-variant: '#7f2b00'
  secondary-fixed: '#e9e1dd'
  secondary-fixed-dim: '#ccc5c2'
  on-secondary-fixed: '#1e1b19'
  on-secondary-fixed-variant: '#4a4643'
  tertiary-fixed: '#ede3b8'
  tertiary-fixed-dim: '#d1c79d'
  on-tertiary-fixed: '#201c02'
  on-tertiary-fixed-variant: '#4d4727'
  background: '#fff8f5'
  on-background: '#1f1b17'
  surface-variant: '#eae1da'
typography:
  headline-lg:
    fontFamily: Space Grotesk
    fontSize: 32px
    fontWeight: '700'
    lineHeight: 38px
    letterSpacing: -0.03em
  headline-lg-mobile:
    fontFamily: Space Grotesk
    fontSize: 26px
    fontWeight: '700'
    lineHeight: 32px
    letterSpacing: -0.02em
  headline-md:
    fontFamily: Space Grotesk
    fontSize: 22px
    fontWeight: '600'
    lineHeight: 28px
    letterSpacing: -0.02em
  headline-sm:
    fontFamily: Space Grotesk
    fontSize: 18px
    fontWeight: '600'
    lineHeight: 24px
    letterSpacing: -0.01em
  body-lg:
    fontFamily: Inter
    fontSize: 15px
    fontWeight: '400'
    lineHeight: 22px
  body-md:
    fontFamily: Inter
    fontSize: 13px
    fontWeight: '400'
    lineHeight: 18px
  body-sm:
    fontFamily: Inter
    fontSize: 12px
    fontWeight: '400'
    lineHeight: 16px
  body-medium:
    fontFamily: Inter
    fontSize: 13px
    fontWeight: '500'
    lineHeight: 18px
  label-kicker:
    fontFamily: Inter
    fontSize: 11px
    fontWeight: '600'
    lineHeight: 14px
    letterSpacing: 0.08em
  code-lg:
    fontFamily: JetBrains Mono
    fontSize: 14px
    fontWeight: '500'
    lineHeight: 20px
  code-md:
    fontFamily: JetBrains Mono
    fontSize: 12px
    fontWeight: '500'
    lineHeight: 16px
  code-sm:
    fontFamily: JetBrains Mono
    fontSize: 11px
    fontWeight: '400'
    lineHeight: 14px
rounded:
  sm: 0.25rem
  DEFAULT: 0.5rem
  md: 0.75rem
  lg: 1rem
  xl: 1.5rem
  full: 9999px
spacing:
  gutter: 1rem
  gutter-desktop: 1.25rem
  margin: 1rem
  margin-desktop: 2.5rem
  space-xs: 0.25rem
  space-sm: 0.5rem
  space-md: 0.75rem
  space-lg: 1.25rem
  space-xl: 2rem
---

## Brand & Style

This design system reimagines the infrastructure management experience as a tangible, high-precision physical workbench. Departing entirely from sterile enterprise dashboards, it embraces the tactile utility of industrial mission control desks, drafting tables, and high-fidelity studio gear. 

The aesthetic is crisp, dense, and mechanical yet warm and inviting. Interfaces are structured like composed instruments laid out on thick paper stock: flat, deliberate, grounded by precise 1px hairline rules, and free of artificial atmospheric blurs or soft drop shadows. Interaction models prioritize fluid spatial freedom through decoupled, floating control instruments rather than rigid boundary-locking frames. The feeling evoked is one of absolute mastery, situational clarity, and creative engineering craft.

## Colors

The palette draws from natural drafting paper, technical drawing ink, and industrial instrument panels.

- **Canvas Foundation**: The root application background is set strictly to `#FAF7F0` (warm paper cream).
- **Surfaces**: Functional panels, cards, and operational modules sit on crisp `#FFFFFF` (flat white), defined strictly by structural borders without drop shadows.
- **Ink & Typography**: Primary headers and critical readouts use `#1C1917` (warm deep ink). Metadata, structural labels, and inactive text render in `#78716C` (muted stone).
- **Structural Lines**: All bounding boxes, cross-sectional dividers, and module segmentations use 1px solid `#E7E0D3` (crisp warm outline).
- **Primary Accent**: `#EA580C` (persimmon orange) drives key focus points, operational switches, active navigation indicators, and primary command triggers.
- **Instrument Surface Washes**:
  - **Metric Surface**: `#E0F2F1` (subtle sky wash) for metric cards, resource utilization badges, and telemetry fields.
  - **Callout Surface**: `#FEF3C7` (butter yellow) for active warnings, maintenance notices, and high-priority notices.
- **Telemetry & Status States**:
  - **Healthy / Running**: `#16A34A` (moss green)
  - **Degraded / Attention**: `#D97706` (amber)
  - **Down / Destructive**: `#DC2626` (brick red)
  - **Stopped / Idle**: `#A8A29E` (stone gray)

## Typography

The typography implements a tripartite hierarchy reflecting architectural blueprints and instrumentation readouts:

1. **Space Grotesk** commands section headers, node clusters, and workbench areas. Its technical geometric construction provides punchy authority without tech-bro tropes.
2. **Inter** handles high-density text, operational descriptors, table rows, and secondary labels.
3. **JetBrains Mono** is mandatory for all technical elements: Git SHAs, IP addresses, ports, uptimes, resource rates, CLI parameters, and timestamps.
4. **Kickers and Section Overlines**: Rendered in `11px`, `uppercase`, `tracking-wider` (`0.08em`), weighted semi-bold in `muted stone` (`#78716C`) to catalog workbench sections like hardware schematics.

## Layout & Spacing

The layout is an open drafting desk. It rejects anchored, non-standard frame panels (no pinned side rails, no pinned horizontal mastheads). Content occupies an expansive work canvas that breathes naturally while maintaining dense, compact information clusters inside cards.

- **Grid Framework**: A fluid 12-column dynamic workbench grid. Cards organize into modular bento tiles spanning 3, 4, 6, or 12 columns.
- **Margins & Safe Zones**: Canvas padding scales from `1rem` on mobile up to `2.5rem` on desktop displays.
- **Vertical Spacing Rhythm**: Gaps between instrument tiles stay locked to compact increments (`1rem` to `1.25rem`) to maintain visual tension and instrument cohesiveness.
- **Viewport Clearances**: The top and bottom bounds enforce 80px buffer clear zones to ensure unobstructed sightlines beneath floating control pills and the suspended bottom dock.

## Elevation & Depth

This design system is strictly non-skeuomorphic and flat in elevation: **zero blur dropshadows** are used on content surfaces. Depth is established through line work, distinct tonal groundings, and floating hardware-inspired chrome.

- **Ground Level**: The base floor is `#FAF7F0`.
- **Level 1 (Workbench Surfaces)**: Pure `#FFFFFF` panels cut into the ground plane with crisp `1px solid #E7E0D3` boundaries.
- **Level 2 (Inlaid Instruments & Troughs)**: Telemetry blocks use recessed surface fills (`#E0F2F1`, `#FEF3C7`, or `#FAF7F0`) with interior hairlines, signaling functional containment.
- **Floating Hardware Decoupled Chrome**: Navigation and global telemetry exist exclusively as floating, decoupled pills. These items feature a paper-white face (`#FFFFFF`), a hairline outline (`1px solid #E7E0D3`), and a sharp, tactile micro-mechanical contact shadow: `0 4px 12px rgba(28, 25, 23, 0.06), 0 1px 2px rgba(28, 25, 23, 0.04)`.

## Shapes

The geometry balances soft, human industrial corners with sharp mechanical fittings:

- **Workbench Cards & Panels**: Fixed at `14px` to `16px` radius (`rounded-2xl`). This produces a tailored sheet-like profile against the warm paper backdrop.
- **Buttons, Field Inputs, & Selectors**: Standardized at `8px` to `10px` radius.
- **Floating Chrome & Badges**: Fully pill-shaped (`9999px` / `rounded-full`). This includes status pills, the central search instrument, active tags, and the decoupled dock.

## Components

### 1. Floating Chrome Instruments (Decoupled Navigation)
- **Top-Center Command Pill**: Floating search apparatus (`⌘K Search`). Height `40px`, background `#FFFFFF`, border `1px solid #E7E0D3`, padding `0 14px`, rounded full. Contains an input prompt in `#78716C` alongside a tactile keyboard badge in `#FAF7F0` bordered with `#E7E0D3`.
- **Top-Right Status Pill**: Floating operational heartbeat. Rounded full, `#FFFFFF` body, border `1px solid #E7E0D3`, containing a ping dot (`#16A34A`), "All Nodes Active", and JetBrains Mono latency metric (`24ms`).
- **Bottom-Center Floating Dock**: The system's primary navigation module. Floats 24px above the viewport base. Composed of 5 monochrome icon triggers housing a 4px persimmon orange (`#EA580C`) active locator dot underneath the active module. Background `#FFFFFF`, bordered in `#E7E0D3`, rounded full.

### 2. Buttons
- **Primary**: Solid `#EA580C` background, white high-contrast text, `rounded-[10px]`, font-medium Inter. Hover shifts to `#C2410C`.
- **Secondary / Ghost**: `#FFFFFF` background, `1px solid #E7E0D3` border, text `#1C1917`. Hover applies a `#FAF7F0` surface tint.
- **Technical Action Buttons**: Monospaced terminal actions with inline keyboard hints, styled with hairline outlines and `#78716C` text.

### 3. Cards & Workbench Tiles
- **Base Card**: Solid `#FFFFFF`, bordered with `1px solid #E7E0D3`, `16px` border radius. Internal spacing defaults to `1.25rem`. No box shadows.
- **Header Structure**: Contains an uppercase 11px kicker label, primary Space Grotesk title, and right-aligned JetBrains Mono telemetry readouts.
- **Inlaid Metrics Tray**: A nested sub-panel with `#E0F2F1` wash, housing CPU/Memory gauges with sharp technical readouts.

### 4. Status Badges & Chips
- Fully rounded pills (`rounded-full`), height `24px`, horizontal padding `10px`.
- Structure: 6px solid circular pip indicator on the left followed by a JetBrains Mono micro-label (`11px`).
- Tone pairings:
  - Running: `#16A34A` pip with green-tinted or neutral white ground.
  - Degraded: `#D97706` pip.
  - Down: `#DC2626` pip.
  - Idle: `#A8A29E` pip.

### 5. Input Fields & Selectors
- Background `#FFFFFF`, border `1px solid #E7E0D3`, rounded `8px` to `10px`. Text rendered in Inter `13px`. Focused states ditch standard halo rings for a crisp `1px solid #EA580C` bounding border.

### 6. Lists & Tables
- Borderless rows separated by flat `1px solid #E7E0D3` dividers. Row hover yields `#FAF7F0`. Column headers use the 11px uppercase kicker style. Ports, IPs, SHAs, and durations stay strictly locked to JetBrains Mono.

### 7. Physical Switches (Toggles)
- Track: 44px length by 24px height, rounded-full. Inactive track is `#E7E0D3`; active track is `#EA580C`. Knob: Pure white `#FFFFFF` circle, 20px diameter, with a micro-mechanical snap transition.