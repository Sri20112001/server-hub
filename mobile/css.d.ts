// Ambient declaration for side-effect stylesheet imports (e.g. global.css).
// The same declaration also ships with expo/types via the generated
// expo-env.d.ts, but that file does not exist on fresh checkouts/CI
// (it is gitignored and created by `expo start`), which caused TS2882 there.
declare module "*.css";
