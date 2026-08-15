import React from 'react';
import { ActivityIndicator, Text, View } from 'react-native';

import { ApiError } from '../api/client';

// Every screen's loading/error boilerplate in one place — react-query
// gives us isLoading/error/data directly, this just renders the two
// non-happy-path states consistently.
export function QueryState({ isLoading, error }: { isLoading: boolean; error: unknown }) {
  if (isLoading) {
    return (
      <View className="flex-1 items-center justify-center py-12">
        <ActivityIndicator />
      </View>
    );
  }
  if (error) {
    const message = error instanceof ApiError ? error.detail ?? error.message : 'Something went wrong.';
    return (
      <View className="flex-1 items-center justify-center px-8 py-12">
        <Text className="text-center text-base text-red-600">{message}</Text>
      </View>
    );
  }
  return null;
}
