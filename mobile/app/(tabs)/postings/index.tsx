import { Link } from 'expo-router';
import React from 'react';
import { FlatList, Pressable, RefreshControl, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { usePostings } from '../../../src/api/hooks';
import type { PostingSummary } from '../../../src/api/types';
import { QueryState } from '../../../src/components/QueryState';

// docs/design.md §8: "Filterable list, infinite scroll." Filtering (by
// company/roleFamily) and cursor-based infinite scroll are both left for
// a follow-up — GET /v1/postings already supports both params, this
// screen just doesn't expose UI for them yet. Ships the first page,
// paginated by the API's own default limit.
export default function PostingsScreen() {
  const { data, isLoading, error, refetch, isRefetching } = usePostings();

  return (
    <SafeAreaView className="flex-1 bg-white">
      <View className="border-b border-slate-200 px-4 py-3">
        <Text className="text-xl font-bold text-slate-900">Postings</Text>
      </View>

      <QueryState isLoading={isLoading} error={error} />

      {data && (
        <FlatList
          data={data.postings}
          keyExtractor={(item) => item.id}
          refreshControl={<RefreshControl refreshing={isRefetching} onRefresh={refetch} />}
          renderItem={({ item }) => <PostingRow posting={item} />}
          ItemSeparatorComponent={() => <View className="h-px bg-slate-100" />}
        />
      )}
    </SafeAreaView>
  );
}

function PostingRow({ posting }: { posting: PostingSummary }) {
  return (
    <Link href={`/(tabs)/postings/${posting.id}`} asChild>
      <Pressable className="px-4 py-3 active:bg-slate-50">
        <Text className="text-base font-medium text-slate-900">{posting.title}</Text>
        <Text className="text-sm text-slate-500">
          {posting.companySlug} · {posting.location || 'Remote/Unspecified'}
        </Text>
        <Text className="text-xs text-slate-400">{posting.skillCount} skills matched</Text>
      </Pressable>
    </Link>
  );
}
