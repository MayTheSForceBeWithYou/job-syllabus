import { useLocalSearchParams } from 'expo-router';
import React from 'react';
import { Linking, Pressable, ScrollView, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { usePosting } from '../../../src/api/hooks';
import type { PostingSkill } from '../../../src/api/types';
import { QueryState } from '../../../src/components/QueryState';

export default function PostingDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { data, isLoading, error } = usePosting(id);

  return (
    <SafeAreaView className="flex-1 bg-white" edges={['bottom']}>
      <QueryState isLoading={isLoading} error={error} />
      {data && (
        <ScrollView className="flex-1 px-4 py-4">
          <Text className="text-2xl font-bold text-slate-900">{data.title}</Text>
          <Text className="mb-1 text-sm text-slate-500">
            {data.companySlug} · {data.location || 'Remote/Unspecified'}
          </Text>
          <Pressable onPress={() => Linking.openURL(data.url)}>
            <Text className="mb-6 text-sm text-blue-600">View original posting →</Text>
          </Pressable>

          <Text className="mb-2 text-sm font-semibold uppercase text-slate-400">
            Extracted skills ({data.skills.length})
          </Text>
          {data.skills.map((skill) => (
            <SkillEvidenceRow key={skill.skillId} skill={skill} />
          ))}
        </ScrollView>
      )}
    </SafeAreaView>
  );
}

function SkillEvidenceRow({ skill }: { skill: PostingSkill }) {
  return (
    <View className="mb-2 rounded-lg bg-slate-50 p-3">
      <View className="mb-1 flex-row items-center justify-between">
        <Text className="text-sm font-semibold text-slate-900">{skill.display}</Text>
        <Text className={`text-xs ${skill.required ? 'text-red-600' : 'text-slate-400'}`}>
          {skill.required ? 'Required' : 'Nice to have'}
        </Text>
      </View>
      <Text className="text-xs text-slate-500">{skill.evidence}</Text>
    </View>
  );
}
