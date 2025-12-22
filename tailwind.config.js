/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./web/templates/**/*.html",
    "./web/static/js/**/*.js",
  ],
  safelist: [
    'lg:grid-cols-4',
    'md:grid-cols-2',
    'lg:col-span-2',
    'lg:grid-cols-3',
    'from-amber-500',
    'via-orange-500',
    'to-rose-500',
    'from-emerald-500',
    'via-green-500',
    'to-teal-500',
    'from-rose-500',
    'via-pink-500',
    'to-red-500',
    'from-indigo-500',
    'via-purple-500',
    'to-violet-500',
    'from-green-500',
    'to-emerald-500',
    'from-purple-500',
    'from-blue-500',
    'to-cyan-500',
    'shadow-amber-500/20',
    'shadow-amber-500/25',
    'shadow-amber-500/40',
    'shadow-emerald-500/20',
    'shadow-rose-500/20',
    'shadow-indigo-500/20',
    'shadow-blue-500/20',
    'shadow-purple-500/20',
    'shadow-green-500/20',
    'from-amber-500/20',
    'to-orange-500/20',
    'from-green-500/20',
    'to-emerald-500/20',
    'from-purple-500/20',
    'to-violet-500/20',
    'from-blue-500/20',
    'to-cyan-500/20',
  ],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // Custom dark mode colors
        dark: {
          bg: '#0a0a0a',
          surface: '#171717',
          border: '#262626',
          hover: '#1f1f1f',
        },
        // Category colors for charts
        category: {
          likvider: '#3b82f6',   // blue
          aktier: '#22c55e',     // green
          krypto: '#f97316',     // orange
          udlaan: '#a855f7',     // purple
          ejendom: '#06b6d4',    // cyan
          bil: '#ec4899',        // pink
          pension: '#6366f1',    // indigo
        },
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'sans-serif'],
        mono: ['JetBrains Mono', 'Fira Code', 'monospace'],
      },
    },
  },
  plugins: [],
}
