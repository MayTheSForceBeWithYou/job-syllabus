import { Redirect } from 'expo-router';
import React from 'react';

import { useAuth } from '../src/auth/AuthContext';

// The anchor route Stack.Protected redirects to when a guard flips false
// (see app/_layout.tsx) needs to itself resolve to something concrete —
// this is that "something," routing straight into whichever of
// (auth)/sign-in or (tabs)/syllabus the current auth state actually
// allows.
export default function Index() {
  const { isSignedIn, isLoading } = useAuth();
  if (isLoading) {
    return null;
  }
  return <Redirect href={isSignedIn ? '/(tabs)/syllabus' : '/(auth)/sign-in'} />;
}
