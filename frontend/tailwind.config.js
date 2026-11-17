/** @type {import('tailwindcss').Config} */
export default {
  darkMode: 'class',
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'sans-serif'],
        mono: ['JetBrains Mono', 'Fira Code', 'monospace'],
      },
      boxShadow: {
        'glow-cyan': '0 0 20px -2px rgba(34, 211, 238, 0.45)',
        'glow-emerald': '0 0 18px -3px rgba(16, 185, 129, 0.4)',
        'glow-rose': '0 0 18px -3px rgba(244, 63, 94, 0.45)',
        'glow-amber': '0 0 18px -3px rgba(245, 158, 11, 0.45)',
      },
    },
  },
  plugins: [],
};
