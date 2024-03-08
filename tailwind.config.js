/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./internal/webapi/templates/**/*.templ"],
  theme: {
    extend: {
       // Adding custom text shadow utilities
      textShadow: {
        default: '1px 1px 2px rgba(0, 0, 0, 0.3)', // Custom default shadow
        md: '2px 2px 4px rgba(0, 0, 0, 0.3)', // Medium shadow
        lg: '4px 4px 6px rgba(0, 0, 0, 0.3)', // Large shadow
        xl: '6px 6px 8px rgba(0, 0, 0, 0.3)', // Extra large shadow
        // Add as many custom shadows as you need
      }
    },
  },
  plugins: [
    // Plugin for applying text shadow utility
    require('tailwindcss-textshadow')
  ],
}

