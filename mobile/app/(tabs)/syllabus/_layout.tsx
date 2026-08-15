import { Stack } from 'expo-router';
import React from 'react';

export default function SyllabusLayout() {
  return (
    <Stack>
      <Stack.Screen name="index" options={{ title: 'Syllabus' }} />
      <Stack.Screen name="[skillId]" options={{ title: 'Skill' }} />
    </Stack>
  );
}
