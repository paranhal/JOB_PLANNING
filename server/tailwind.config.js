/** @type {import('tailwindcss').Config} */
const c = (n) => `rgb(var(--c-${n}) / <alpha-value>)`;

module.exports = {
  content: [
    "./web/templates/**/*.html",
    "./internal/**/*.go",
  ],
  theme: {
    extend: {
      colors: {
        canvas: c('canvas'), surface: c('surface'), hover: c('hover'), selected: c('selected'),
        sidebar: { DEFAULT: c('sidebar'), text: c('sidebar-text') },
        line: { DEFAULT: c('border'), strong: c('border-strong') },
        ink: { DEFAULT: c('text'), muted: c('text-muted'), subtle: c('text-subtle') },
        primary: { DEFAULT: c('primary'), hover: c('primary-hover'), soft: c('primary-soft'), fg: c('on-primary') },
        success: { DEFAULT: c('success'), soft: c('success-soft') },
        warning: { DEFAULT: c('warning'), soft: c('warning-soft') },
        danger:  { DEFAULT: c('danger'),  soft: c('danger-soft') },
        info:    { DEFAULT: c('info'),    soft: c('info-soft') },
        target: { a: c('target-a'), b: c('target-b'), c: c('target-c') },
        overdue: c('overdue'),
      },
      borderRadius: { control: 'var(--radius-control)', card: 'var(--radius-card)' },
      boxShadow: { pop: 'var(--shadow-pop)' },
      fontFamily: { sans: ['Pretendard Variable', 'Pretendard', '-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'Malgun Gothic', 'sans-serif'] },
      fontSize: { 'ui-xs': ['0.6875rem','1rem'], 'ui-sm': ['0.75rem','1.125rem'], ui: ['0.8125rem','1.25rem'], 'ui-lg': ['0.9375rem','1.375rem'], 'ui-xl': ['1.125rem','1.5rem'] },
    },
  },
  plugins: [],
}
