import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./app/**/*.{ts,tsx}", "./components/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        bg:          "#121212",
        surface:     "#181818",
        "surface-2": "#282828",
        "surface-3": "#3E3E3E",
        accent:      "#1DB954",
        "accent-hover": "#1ed760",
        muted:       "#A7A7A7",
        subdued:     "#6A6A6A",
      },
    },
  },
  plugins: [],
};

export default config;
