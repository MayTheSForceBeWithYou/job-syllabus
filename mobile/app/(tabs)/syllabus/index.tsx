import { Link } from 'expo-router';
import React from 'react';
import { FlatList, Pressable, RefreshControl, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { useSkills } from '../../../src/api/hooks';
import type { Skill } from '../../../src/api/types';
import { useAuth } from '../../../src/auth/AuthContext';
import { QueryState } from '../../../src/components/QueryState';

// docs/design.md §8: "Home. Ranked skill list, % of postings,
// required-vs-preferred toggle, role-family filter, category grouping."
// Category grouping and role-family filtering are left for a follow-up —
// this ships the ranked list + required toggle, the part of the screen
// that's actually "the money endpoint" (GET /v1/skills) end to end.
export default function SyllabusScreen() {
  const [requiredOnly, setRequiredOnly] = React.useState(false);
  const { data, isLoading, error, refetch, isRefetching } = useSkills(
    requiredOnly ? { required: 'true' } : undefined,
  );
  const { signOut } = useAuth();

  return (
    <SafeAreaView className="flex-1 bg-white">
      <View className="flex-row items-center justify-between border-b border-slate-200 px-4 py-3">
        <Text className="text-xl font-bold text-slate-900">Syllabus</Text>
        <Pressable onPress={() => signOut()}>
          <Text className="text-sm text-slate-500">Sign out</Text>
        </Pressable>
      </View>

      <View className="flex-row items-center justify-between px-4 py-2">
        <Text className="text-sm text-slate-500">
          {data ? `${data.totalPostings} active postings` : ' '}
        </Text>
        <Pressable
          onPress={() => setRequiredOnly((v) => !v)}
          className={`rounded-full px-3 py-1 ${requiredOnly ? 'bg-slate-900' : 'bg-slate-100'}`}
        >
          <Text className={`text-xs font-medium ${requiredOnly ? 'text-white' : 'text-slate-600'}`}>
            Required only
          </Text>
        </Pressable>
      </View>

      <QueryState isLoading={isLoading} error={error} />

      {data && (
        <FlatList
          data={data.skills}
          keyExtractor={(item) => item.id}
          refreshControl={<RefreshControl refreshing={isRefetching} onRefresh={refetch} />}
          renderItem={({ item }) => <SkillRow skill={item} />}
          ItemSeparatorComponent={() => <View className="h-px bg-slate-100" />}
        />
      )}
    </SafeAreaView>
  );
}

function SkillRow({ skill }: { skill: Skill }) {
  return (
    <Link href={`/(tabs)/syllabus/${skill.id}`} asChild>
      <Pressable className="flex-row items-center justify-between px-4 py-3 active:bg-slate-50">
        <View className="flex-1 pr-3">
          <Text className="text-base font-medium text-slate-900">{skill.display}</Text>
          <Text className="text-xs text-slate-400">{skill.category}</Text>
        </View>
        <View className="items-end">
          <Text className="text-base font-semibold text-slate-900">{skill.pctOfPostings.toFixed(1)}%</Text>
          <Text className="text-xs text-slate-400">
            {skill.required} req · {skill.niceToHave} nice
          </Text>
        </View>
      </Pressable>
    </Link>
  );
}
