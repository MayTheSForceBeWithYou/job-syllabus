import React from 'react';
import { FlatList, RefreshControl, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { useCompanies, useStatsOverview } from '../../../src/api/hooks';
import type { Company } from '../../../src/api/types';
import { QueryState } from '../../../src/components/QueryState';

// docs/design.md §8: "Registry, coverage, ingest health." Coverage/health
// (last-ingest timestamp, DLQ status) beyond what GET /v1/stats/overview
// already reports isn't built into the API yet — this shows the registry
// plus that one overview stat, not a full health dashboard.
export default function CompaniesScreen() {
  const { data, isLoading, error, refetch, isRefetching } = useCompanies();
  const stats = useStatsOverview();

  return (
    <SafeAreaView className="flex-1 bg-white">
      <View className="border-b border-slate-200 px-4 py-3">
        <Text className="text-xl font-bold text-slate-900">Companies</Text>
        {stats.data && (
          <Text className="mt-1 text-xs text-slate-400">
            {stats.data.companyCount} companies · {stats.data.postingCount} active postings ·{' '}
            {stats.data.coveragePct.toFixed(1)}% skill coverage
          </Text>
        )}
      </View>

      <QueryState isLoading={isLoading} error={error} />

      {data && (
        <FlatList
          data={[...data.companies].sort((a, b) => b.postingCount - a.postingCount)}
          keyExtractor={(item) => item.slug}
          refreshControl={<RefreshControl refreshing={isRefetching} onRefresh={refetch} />}
          renderItem={({ item }) => <CompanyRow company={item} />}
          ItemSeparatorComponent={() => <View className="h-px bg-slate-100" />}
        />
      )}
    </SafeAreaView>
  );
}

function CompanyRow({ company }: { company: Company }) {
  return (
    <View className="flex-row items-center justify-between px-4 py-3">
      <View>
        <Text className="text-base font-medium text-slate-900">{company.name}</Text>
        <Text className="text-xs text-slate-400">
          {company.tier} · {company.ats}
        </Text>
      </View>
      <Text className="text-sm font-semibold text-slate-600">{company.postingCount}</Text>
    </View>
  );
}
