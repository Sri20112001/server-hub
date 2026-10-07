const path = require("path");
const { getDefaultConfig } = require("expo/metro-config");
const { withNativeWind } = require("nativewind/metro");

const config = getDefaultConfig(__dirname);

// Resolve the shared workspace package (alias-based, no npm workspaces):
// watch it so Metro rebuilds on change, and map the package name to it.
const sharedDir = path.resolve(__dirname, "../packages/shared/src");
config.watchFolders = [...(config.watchFolders || []), sharedDir];
config.resolver.extraNodeModules = {
  ...(config.resolver.extraNodeModules || {}),
  "@serverhub/shared": sharedDir,
};

module.exports = withNativeWind(config, { input: "./global.css" });
