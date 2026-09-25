// Extracted from client/src/index.css — Physical Workbench + Cyberdeck Night tokens

export const colors = {
  // Light (Physical Workbench)
  paper: "#faf7f0",
  ink: "#1c1917",
  muted: "#78716c",
  line: "#e7e0d3",
  accent: "#ea580c",
  accentHover: "#c2410c",
  accentDeep: "#a33900",
  moss: "#16a34a",
  statusAmber: "#d97706",
  brick: "#dc2626",
  stone: "#a8a29e",
  tint: "#f6ece6",
  skel: "#eae1da",
  white: "#ffffff",

  // Dark (Cyberdeck Night)
  abyss: "#121316",
  panel: "#1b1e22",
  emboss: "#24272c",
  bone: "#ece7df",
  fog: "#a39c90",
  edge: "#2c3036",
  ember: "#f59e0b",
  emberHover: "#d97706",
  dangerNight: "#251a10",
} as const;

// Fleet status to color mapping (mirrors web DOT_TONE)
export const statusColors = {
  sailing: colors.moss,
  choppy: colors.statusAmber,
  lost: colors.brick,
  docked: colors.stone,
} as const;
