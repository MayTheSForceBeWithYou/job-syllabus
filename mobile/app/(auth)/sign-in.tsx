import React from 'react';
import { ActivityIndicator, Pressable, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { useAuth } from '../../src/auth/AuthContext';

export default function SignInScreen() {
  const { signIn, isLoading } = useAuth();
  const [isSigningIn, setIsSigningIn] = React.useState(false);

  const handleSignIn = async () => {
    setIsSigningIn(true);
    try {
      await signIn();
    } finally {
      setIsSigningIn(false);
    }
  };

  return (
    <SafeAreaView className="flex-1 items-center justify-center bg-white px-8">
      <Text className="mb-2 text-3xl font-bold text-slate-900">Job Syllabus</Text>
      <Text className="mb-10 text-center text-base text-slate-500">
        A study syllabus built from real game-industry job postings.
      </Text>
      <Pressable
        onPress={handleSignIn}
        disabled={isLoading || isSigningIn}
        className="w-full rounded-lg bg-slate-900 px-6 py-4 active:opacity-80"
      >
        {isSigningIn ? (
          <ActivityIndicator color="white" />
        ) : (
          <Text className="text-center text-base font-semibold text-white">Sign in</Text>
        )}
      </Pressable>
    </SafeAreaView>
  );
}
