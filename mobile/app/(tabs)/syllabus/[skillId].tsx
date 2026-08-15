import { useLocalSearchParams } from 'expo-router';
import React from 'react';
import { ScrollView, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { useSkill } from '../../../src/api/hooks';
import { QueryState } from '../../../src/components/QueryState';

// docs/design.md §8: "Trend chart, evidence snippets, companies demanding
// it." No trend chart — internal/api/dto.go's skillDetailDTO deliberately
// has no trend series yet (that needs time-bucketed history the API
// doesn't track, see that DTO's own comment: "Phase 8, Trend charts").
// Evidence snippets are real; "companies demanding it" would need a new
// API query this phase doesn't add.
export default function SkillDetailScreen() {
  const { skillId } = useLocalSearchParams<{ skillId: string }>();
  const { data, isLoading, error } = useSkill(skillId);

  return (
    <SafeAreaView className="flex-1 bg-white" edges={['bottom']}>
      <QueryState isLoading={isLoading} error={error} />
      {data && (
        <ScrollView className="flex-1 px-4 py-4">
          <Text className="text-2xl font-bold text-slate-900">{data.display}</Text>
          <Text className="mb-4 text-sm text-slate-400">{data.category}</Text>

          <View className="mb-6 flex-row gap-6">
            <Stat label="Mentioned in" value={String(data.count)} />
            <Stat label="Required" value={String(data.required)} />
            <Stat label="Nice to have" value={String(data.niceToHave)} />
          </View>

          {data.exampleEvidence.length > 0 && (
            <>
              <Text className="mb-2 text-sm font-semibold uppercase text-slate-400">
                Example evidence
              </Text>
              {data.exampleEvidence.map((ev, i) => (
                <View key={i} className="mb-2 rounded-lg bg-slate-50 p-3">
                  <Text className="text-sm text-slate-700">{ev}</Text>
                </View>
              ))}
            </>
          )}
        </ScrollView>
      )}
    </SafeAreaView>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <View>
      <Text className="text-lg font-bold text-slate-900">{value}</Text>
      <Text className="text-xs text-slate-400">{label}</Text>
    </View>
  );
}
