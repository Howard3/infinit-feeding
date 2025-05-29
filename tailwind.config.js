/** @type {import('tailwindcss').Config} */
module.exports = {
    content: [
        "./internal/webapi/templates/**/*.{templ,go}",
        "./static/**/*.{js,html}",
    ],
    safelist: [
        // Badge classes for Admin
        "bg-green-50",
        "text-green-700",
        "ring-green-600/20",
        // Badge classes for Feeder
        "bg-blue-50",
        "text-blue-700",
        "ring-blue-600/20",
        // Common badge classes
        "inline-flex",
        "items-center",
        "rounded-full",
        "px-2",
        "py-1",
        "text-xs",
        "font-medium",
        "ring-1",
        "ring-inset",
        // Pagination classes
        "disabled:opacity-40",
        "disabled:cursor-not-allowed",
        "disabled:hover:bg-transparent",
        "hover:bg-gray-100",
        "hover:text-gray-900",
        "focus:outline-none",
        "focus:ring-2",
        "focus:ring-blue-500",
        "focus:ring-offset-1",
        "focus:border-blue-500",
        "transition-all",
        "duration-200",
        "text-gray-500",
        "text-gray-700",
        "border-gray-200",
        "shadow-sm",
        // Table styling classes
        "even:bg-gray-50/50",
        "bg-gray-50",
        "data-[state=selected]:bg-muted",
        "bg-blue-500",
        "bg-yellow-500",
        "bg-green-500",
        "bg-red-500",
        "bg-purple-500",
        "bg-gray-500",
    ],
    theme: {
        extend: {},
    },
    plugins: [],
};
