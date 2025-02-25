
/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./src/app/**/*.{js,ts,jsx,tsx}",
    "./src/components/**/*.{js,ts,jsx,tsx}",
    "./src/styles/**/*.{css}"
  ],
  theme: {
    extend: {
	colors: {
		font: "#0D1321", // Цвет шрифта
		accent: "#DCEBFA", // Акцентный цвет
		background: "#F5F5F5", // Цвет фона
		shade: "#8f8f8f", // Цвет тени
		font2: "#0D1321", // Цвет неакцентного шрифта
		},
	boxShadow: {
        	"custom": "0px 4px 24px rgba(143, 143, 143, 0.16)", // #8F8F8F с прозрачностью 16%
      },
	fontFamily: {
        	sans: ["Golos Text VF", "sans-serif"],
      },
	fontSize: {
        	button: "8px",
        	text: "12px",
        	h4: "16px",
        	h3: "24px",
        	h2: "32px",
        	h1: "40px",
      },
	},
  },
  plugins: [],
};
