import React from "react";
import { Redirect } from "expo-router";

// Projects section temporarily hidden from the UI. The full fleet screen
// implementation is preserved in git history — this stub only redirects
// direct visits back to the dashboard. Revert this file to restore it.
export default function FleetScreen() {
  return <Redirect href="/(app)/(tabs)" />;
}
