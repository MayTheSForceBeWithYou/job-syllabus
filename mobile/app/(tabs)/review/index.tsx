import React from 'react';
import { FlatList, Pressable, RefreshControl, Text, TextInput, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { useReviewAction, useReviews } from '../../../src/api/hooks';
import type { ReviewTerm } from '../../../src/api/types';
import { QueryState } from '../../../src/components/QueryState';

// docs/design.md §8: "Unknown-term triage queue. Swipe or tap: create /
// alias / reject... design for one-handed use." Ships as tap-only (three
// buttons per card) rather than swipe gestures — bite-sized and
// one-handed either way, and this avoids pulling in a gesture-based
// card-stack library for what's otherwise a thin interaction. Swipe is a
// reasonable follow-up, not a DoD requirement (docs/design.md §13: "sign
// in and browse the syllabus," which this screen satisfies as one of the
// tabs to browse).
export default function ReviewScreen() {
  const { data, isLoading, error, refetch, isRefetching } = useReviews();

  return (
    <SafeAreaView className="flex-1 bg-white">
      <View className="border-b border-slate-200 px-4 py-3">
        <Text className="text-xl font-bold text-slate-900">Review queue</Text>
        <Text className="mt-1 text-xs text-slate-400">
          {data ? `${data.reviews.length} pending` : ' '}
        </Text>
      </View>

      <QueryState isLoading={isLoading} error={error} />

      {data && data.reviews.length === 0 && (
        <View className="flex-1 items-center justify-center px-8">
          <Text className="text-center text-base text-slate-400">
            Nothing to review right now — Bedrock hasn't surfaced any unknown terms.
          </Text>
        </View>
      )}

      {data && data.reviews.length > 0 && (
        <FlatList
          data={data.reviews}
          keyExtractor={(item) => item.term}
          refreshControl={<RefreshControl refreshing={isRefetching} onRefresh={refetch} />}
          renderItem={({ item }) => <ReviewCard term={item} onDone={refetch} />}
          ItemSeparatorComponent={() => <View className="h-px bg-slate-100" />}
        />
      )}
    </SafeAreaView>
  );
}

function ReviewCard({ term, onDone }: { term: ReviewTerm; onDone: () => void }) {
  const [showAlias, setShowAlias] = React.useState(false);
  const [aliasTarget, setAliasTarget] = React.useState('');
  const mutation = useReviewAction();

  const busy = mutation.isPending;

  const create = () => {
    mutation.mutate(
      { term: term.term, action: { action: 'create', category: term.category || 'other' } },
      { onSuccess: onDone },
    );
  };

  const reject = () => {
    mutation.mutate({ term: term.term, action: { action: 'reject' } }, { onSuccess: onDone });
  };

  const confirmAlias = () => {
    if (!aliasTarget.trim()) return;
    mutation.mutate(
      { term: term.term, action: { action: 'alias', mergeIntoSkillId: aliasTarget.trim() } },
      {
        onSuccess: () => {
          setShowAlias(false);
          onDone();
        },
      },
    );
  };

  return (
    <View className="px-4 py-3">
      <View className="mb-1 flex-row items-center justify-between">
        <Text className="text-base font-semibold capitalize text-slate-900">{term.term}</Text>
        <Text className="text-xs text-slate-400">
          {term.occurrences}× · {term.category || 'uncategorized'}
        </Text>
      </View>
      {term.evidence[0] && (
        <Text className="mb-2 text-xs text-slate-500" numberOfLines={2}>
          &ldquo;{term.evidence[0]}&rdquo;
        </Text>
      )}

      {mutation.isError && (
        <Text className="mb-2 text-xs text-red-600">Failed — try again.</Text>
      )}

      {!showAlias ? (
        <View className="flex-row gap-2">
          <ActionButton label="Create" onPress={create} disabled={busy} variant="primary" />
          <ActionButton label="Alias" onPress={() => setShowAlias(true)} disabled={busy} />
          <ActionButton label="Reject" onPress={reject} disabled={busy} variant="danger" />
        </View>
      ) : (
        <View className="flex-row items-center gap-2">
          <TextInput
            value={aliasTarget}
            onChangeText={setAliasTarget}
            placeholder="existing skill id, e.g. perforce"
            autoCapitalize="none"
            className="flex-1 rounded-lg border border-slate-300 px-3 py-2 text-sm"
          />
          <ActionButton label="Merge" onPress={confirmAlias} disabled={busy} variant="primary" />
          <ActionButton label="Cancel" onPress={() => setShowAlias(false)} disabled={busy} />
        </View>
      )}
    </View>
  );
}

function ActionButton({
  label,
  onPress,
  disabled,
  variant = 'default',
}: {
  label: string;
  onPress: () => void;
  disabled?: boolean;
  variant?: 'default' | 'primary' | 'danger';
}) {
  const bg = variant === 'primary' ? 'bg-slate-900' : variant === 'danger' ? 'bg-red-50' : 'bg-slate-100';
  const text = variant === 'primary' ? 'text-white' : variant === 'danger' ? 'text-red-600' : 'text-slate-700';
  return (
    <Pressable
      onPress={onPress}
      disabled={disabled}
      className={`rounded-full px-4 py-2 active:opacity-70 ${bg} ${disabled ? 'opacity-40' : ''}`}
    >
      <Text className={`text-sm font-medium ${text}`}>{label}</Text>
    </Pressable>
  );
}
