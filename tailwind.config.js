/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    './internal/web/templates/**/*.html',
  ],
  darkMode: 'class', // <html class="dark"> activates dark variants (currently unused — templates hardcode dark colors)
  theme: {
    extend: {
      colors: {
        // 'surface' palette aliases the slate scale, used as page/card/sidebar backgrounds.
        // Values match what the original precompiled CSS shipped, kept here so future rebuilds reproduce the same look.
        surface: {
          700: 'rgb(30 41 59)',  // slate-700
          800: 'rgb(15 23 42)',  // slate-900 (yes, 800 was mapped to slate-900)
          900: 'rgb(2 6 23)',    // slate-950
        },
      },
    },
  },
  plugins: [],
}
