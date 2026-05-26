import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./app/**/*.{ts,tsx}", "./components/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        bg: "#121212",
        surface: "#181818",
        "surface-2": "#282828",
        accent: "#1DB954",
        "accent-hover": "#1ed760",
        muted: "#A7A7A7",
      },
    },
  },
  plugins: [],
};

export default config;
