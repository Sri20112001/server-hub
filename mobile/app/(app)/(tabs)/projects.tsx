import React from "react";
import { Redirect } from "expo-router";

// Projects section temporarily hidden from the UI. The full projects
// screen implementation is preserved in git history — this stub only
// redirects direct visits back to the dashboard. Revert this file to
// restore it.
export default function ProjectsScreen() {
  return <Redirect href="/(app)/(tabs)" />;
}
