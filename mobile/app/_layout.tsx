import '../global.css';

import { Stack } from 'expo-router';
import React from 'react';
import { GestureHandlerRootView } from 'react-native-gesture-handler';
import { SafeAreaProvider } from 'react-native-safe-area-context';
import { StyleSheet } from 'react-native';

import { useAuth } from '../src/auth/AuthContext';
import { AppProviders } from '../src/providers/AppProviders';

// Nativewind on web defaults to media-query-based dark mode detection,
// which react-native-web's own StyleSheet rejects if anything tries to
// set a color scheme manually — a real, documented interop bug
// (nativewind/nativewind#1489), not something this app's code triggers
// directly. This app doesn't implement a dark-mode toggle at all, so the
// underlying warning is harmless either way; `setFlag` is only real on
// react-native-css-interop's patched StyleSheet (nativewind's
// native-platform metro transform), not on this SDK's react-native-web
// build, where calling it unconditionally crashed the initial render
// outright — confirmed against a real `expo start --web` run, not just
// reasoned about — hence the existence check rather than a bare call.
const patchedStyleSheet = StyleSheet as unknown as { setFlag?: (name: string, value: string) => void };
if (typeof patchedStyleSheet.setFlag === 'function') {
  patchedStyleSheet.setFlag('darkMode', 'class');
}

// docs/design.md §8: (auth)/sign-in vs (tabs)/* — Stack.Protected (SDK 53+)
// redirects to the anchor route automatically when its guard flips false,
// rather than this layout needing to manually push/replace routes on
// every auth state change.
function RootNavigator() {
  const { isSignedIn, isLoading } = useAuth();

  if (isLoading) {
    // Restoring a persisted session (SecureStore/localStorage read) — a
    // blank frame here is intentionally brief, not a real loading screen;
    // it should resolve within a tick on every platform.
    return null;
  }

  return (
    <Stack screenOptions={{ headerShown: false }}>
      {/* Always accessible — resolves "/" itself and is the anchor route
          Stack.Protected below redirects to when a guard flips false. */}
      <Stack.Screen name="index" />
      <Stack.Protected guard={!isSignedIn}>
        <Stack.Screen name="(auth)" />
      </Stack.Protected>
      <Stack.Protected guard={isSignedIn}>
        <Stack.Screen name="(tabs)" />
      </Stack.Protected>
    </Stack>
  );
}

export default function RootLayout() {
  return (
    <GestureHandlerRootView style={{ flex: 1 }}>
      <SafeAreaProvider>
        <AppProviders>
          <RootNavigator />
        </AppProviders>
      </SafeAreaProvider>
    </GestureHandlerRootView>
  );
}
