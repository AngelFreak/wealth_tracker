# Wealth Tracker Design System

## Overview
This document defines the visual design standards for the Wealth Tracker application. All UI components must adhere to these specifications.

---

## Color Palette

### Light Mode

| Usage | Color | Hex | Tailwind |
|-------|-------|-----|----------|
| Background | White | `#ffffff` | `bg-white` |
| Surface | Light Gray | `#f9fafb` | `bg-gray-50` |
| Border | Gray | `#e5e7eb` | `border-gray-200` |
| Text Primary | Dark Gray | `#111827` | `text-gray-900` |
| Text Secondary | Medium Gray | `#6b7280` | `text-gray-500` |

### Dark Mode (Default)

| Usage | Color | Hex | Tailwind |
|-------|-------|-----|----------|
| Background | Near Black | `#0a0a0a` | `bg-dark-bg` |
| Surface | Dark Gray | `#171717` | `bg-dark-surface` |
| Border | Charcoal | `#262626` | `border-dark-border` |
| Text Primary | Off White | `#fafafa` | `text-gray-100` |
| Text Secondary | Light Gray | `#a1a1aa` | `text-gray-400` |

### Accent Colors

| Usage | Color | Hex | Tailwind |
|-------|-------|-----|----------|
| Primary | Indigo | `#6366f1` | `indigo-500` |
| Primary Hover | Dark Indigo | `#4f46e5` | `indigo-600` |
| Success | Green | `#22c55e` | `green-500` |
| Warning | Amber | `#f59e0b` | `amber-500` |
| Error | Red | `#ef4444` | `red-500` |

### Category Colors (Charts)

| Category | Color | Hex | Tailwind Class |
|----------|-------|-----|----------------|
| Likvider | Blue | `#3b82f6` | `category-likvider` |
| Aktier | Green | `#22c55e` | `category-aktier` |
| Krypto | Orange | `#f97316` | `category-krypto` |
| Udlån | Purple | `#a855f7` | `category-udlaan` |
| Ejendom | Cyan | `#06b6d4` | `category-ejendom` |
| Bil | Pink | `#ec4899` | `category-bil` |
| Pension | Indigo | `#6366f1` | `category-pension` |

---

## Typography

### Font Stack

```css
font-family: 'Inter', system-ui, -apple-system, sans-serif;
font-family-mono: 'JetBrains Mono', 'Fira Code', monospace;
```

### Scale

| Element | Size | Weight | Class |
|---------|------|--------|-------|
| Net Worth Display | 36px | Bold (700) | `text-4xl font-bold` |
| Page Title | 24px | Semibold (600) | `text-2xl font-semibold` |
| Section Header | 18px | Semibold (600) | `text-lg font-semibold` |
| Card Title | 14px | Medium (500) | `text-sm font-medium` |
| Body Text | 14px | Regular (400) | `text-sm` |
| Small/Labels | 12px | Regular (400) | `text-xs` |
| Micro | 10px | Medium (500) | `text-[10px] font-medium` |

### Currency Display

- Use `font-mono tabular-nums` for alignment
- Format: `1,234,567.00 kr.`
- Positive change: Green with `+` prefix
- Negative change: Red with `-` prefix

---

## Spacing

### Base Unit
4px (0.25rem)

### Scale

| Name | Size | Tailwind |
|------|------|----------|
| xs | 4px | `p-1`, `m-1` |
| sm | 8px | `p-2`, `m-2` |
| md | 12px | `p-3`, `m-3` |
| lg | 16px | `p-4`, `m-4` |
| xl | 20px | `p-5`, `m-5` |
| 2xl | 24px | `p-6`, `m-6` |

### Component Spacing

| Component | Padding | Gap |
|-----------|---------|-----|
| Card | `p-5` (20px) | - |
| Card Header | `px-5 py-4` | - |
| Button | `px-4 py-2` | `gap-2` |
| Input | `px-3 py-2` | - |
| Table Cell | `px-4 py-3` | - |
| Section Gap | - | `gap-6` |
| Card Grid Gap | - | `gap-4` |

---

## Components

### Cards

```html
<div class="card">
  <div class="card-header">
    <h3 class="text-sm font-medium">Card Title</h3>
  </div>
  <div class="card-body">
    <!-- Content -->
  </div>
</div>
```

### Buttons

| Variant | Class | Usage |
|---------|-------|-------|
| Primary | `btn-primary` | Main actions (Save, Create) |
| Secondary | `btn-secondary` | Secondary actions (Cancel) |
| Danger | `btn-danger` | Destructive actions (Delete) |
| Ghost | `btn-ghost` | Tertiary actions, icons |

**Sizes:**
- Default: `px-4 py-2 text-sm`
- Small: `px-3 py-1.5 text-xs`
- Large: `px-6 py-3 text-base`

### Form Inputs

```html
<label class="label">Email</label>
<input type="email" class="input" placeholder="you@example.com">
```

Error state:
```html
<input type="email" class="input-error">
<p class="text-xs text-red-500 mt-1">Invalid email address</p>
```

### Badges

| Variant | Class | Usage |
|---------|-------|-------|
| Success | `badge-success` | Positive status |
| Warning | `badge-warning` | Caution needed |
| Error | `badge-error` | Problem/negative |
| Info | `badge-info` | Neutral information |

### Progress Bar

```html
<div class="progress">
  <div class="progress-bar bg-indigo-500" style="width: 45%"></div>
</div>
```

### KPI Card

```html
<div class="kpi-card">
  <p class="kpi-value">2,560,227 kr.</p>
  <p class="kpi-label">Net Worth</p>
  <p class="kpi-change-positive">+12.5% this month</p>
</div>
```

---

## Layout

### Page Structure

```html
<div class="min-h-screen bg-gray-50 dark:bg-dark-bg">
  <!-- Sidebar (fixed) -->
  <aside class="fixed inset-y-0 left-0 w-64 bg-white dark:bg-dark-surface border-r border-gray-200 dark:border-dark-border">
    <!-- Navigation -->
  </aside>

  <!-- Main Content -->
  <main class="ml-64 p-6">
    <!-- Page content -->
  </main>
</div>
```

### Grid System

```html
<!-- Dashboard KPI cards -->
<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
  <!-- Cards -->
</div>

<!-- Two-column layout -->
<div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
  <div class="lg:col-span-2"><!-- Main content --></div>
  <div><!-- Sidebar --></div>
</div>
```

### Breakpoints

| Name | Min Width | Usage |
|------|-----------|-------|
| sm | 640px | Mobile landscape |
| md | 768px | Tablet |
| lg | 1024px | Desktop |
| xl | 1280px | Large desktop |

---

## Dark Mode

### Implementation

Dark mode is enabled by adding `class="dark"` to the `<html>` element.

```html
<html class="dark">
```

### Toggle Pattern

```javascript
// Alpine.js dark mode toggle
<div x-data="{ dark: localStorage.getItem('theme') === 'dark' }"
     x-init="$watch('dark', val => {
       localStorage.setItem('theme', val ? 'dark' : 'light');
       document.documentElement.classList.toggle('dark', val);
     })">
  <button @click="dark = !dark">Toggle</button>
</div>
```

### Color Mapping

Always include dark variants:
- `bg-white dark:bg-dark-surface`
- `text-gray-900 dark:text-gray-100`
- `border-gray-200 dark:border-dark-border`
- `hover:bg-gray-50 dark:hover:bg-dark-hover`

---

## Charts (Chart.js)

### Color Palette

```javascript
const chartColors = {
  primary: '#6366f1',
  success: '#22c55e',
  danger: '#ef4444',
  grid: isDark ? '#262626' : '#e5e7eb',
  text: isDark ? '#a1a1aa' : '#6b7280',
};
```

### Default Options

```javascript
Chart.defaults.color = chartColors.text;
Chart.defaults.borderColor = chartColors.grid;
Chart.defaults.font.family = "'Inter', sans-serif";
```

### Pie Chart (Distribution)

- Maximum 7 segments
- Use category colors
- Show percentages in tooltips
- Legend on right side

### Line Chart (History)

- Smooth curves (tension: 0.4)
- Fill area below line
- Y-axis: currency formatted
- X-axis: date formatted

---

## Accessibility

### Color Contrast

- Text on background: minimum 4.5:1 ratio
- Large text: minimum 3:1 ratio
- All interactive elements: visible focus state

### Focus States

```css
focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2
/* Dark mode offset */
dark:focus:ring-offset-dark-bg
```

### Touch Targets

- Minimum 44x44px for all interactive elements
- Adequate spacing between clickable items

---

## Animation

### Transitions

- Default duration: 200ms
- Easing: ease-out
- Properties: colors, opacity, transform

```css
transition-colors duration-200
transition-opacity duration-200
transition-transform duration-200
```

### HTMX Loading States

```html
<button class="btn-primary">
  <span class="htmx-indicator">
    <svg class="animate-spin h-4 w-4">...</svg>
  </span>
  <span>Save</span>
</button>
```

---

## Icons

Use Heroicons (outline style) via CDN or inline SVG.

Common icons:
- Dashboard: `home`
- Accounts: `wallet`
- Categories: `tag`
- Transactions: `arrows-right-left`
- Goals: `flag`
- Settings: `cog`
- Add: `plus`
- Edit: `pencil`
- Delete: `trash`
- Success: `check-circle`
- Warning: `exclamation-triangle`
- Error: `x-circle`
