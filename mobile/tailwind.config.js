/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./app/**/*.{js,jsx,ts,tsx}", "./src/**/*.{js,jsx,ts,tsx}"],
  presets: [require("nativewind/preset")],
  theme: {
    extend: {
      colors: {
        // Physical Workbench (light) tokens — from client/src/index.css
        paper: "#faf7f0",
        ink: "#1c1917",
        muted: "#78716c",
        line: "#e7e0d3",
        accent: "#ea580c",
        "accent-hover": "#c2410c",
        "accent-deep": "#a33900",
        moss: "#16a34a",
        "status-amber": "#d97706",
        brick: "#dc2626",
        stone: "#a8a29e",
        tint: "#f6ece6",
        skel: "#eae1da",
        // Cyberdeck Night (dark) tokens
        abyss: "#121316",
        panel: "#1b1e22",
        emboss: "#24272c",
        bone: "#ece7df",
        fog: "#a39c90",
        edge: "#2c3036",
        ember: "#f59e0b",
        "ember-hover": "#d97706",
        "danger-night": "#251a10",
      },
      fontFamily: {
        head: ["SpaceGrotesk_700Bold"],
        body: ["Inter_400Regular"],
        "body-medium": ["Inter_500Medium"],
        mono: ["JetBrainsMono_400Regular"],
      },
      borderRadius: {
        card: 16,
        input: 10,
      },
    },
  },
  plugins: [],
};
