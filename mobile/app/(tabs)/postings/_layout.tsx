import { Stack } from 'expo-router';
import React from 'react';

export default function PostingsLayout() {
  return (
    <Stack>
      <Stack.Screen name="index" options={{ title: 'Postings' }} />
      <Stack.Screen name="[id]" options={{ title: 'Posting' }} />
    </Stack>
  );
}
