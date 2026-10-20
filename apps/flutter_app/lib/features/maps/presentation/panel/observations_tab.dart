import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../../core/l10n/strings.dart';
import '../../../../core/models/models.dart';
import '../../../../core/theme/tokens.dart';
import '../../../../shared_widgets/shared_widgets.dart';
import '../../data/map_providers.dart';
import '../../domain/map_state.dart';
import '../map_controls.dart';
import '../map_stack.dart';
import '../observation_tile.dart';

/// Panel tab "Observations": filterable list of observations in the selected
/// region (or the current map viewport when no region is selected).
class ObservationsTab extends ConsumerWidget {
  const ObservationsTab({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = context.s;
    final eco = context.eco;
    final region = ref.watch(selectedRegionProvider);
    final status = ref.watch(statusFilterProvider);
    final async = ref.watch(observationsListProvider);
    final selectedId = ref.watch(selectedObservationIdProvider);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(Dimens.lg, Dimens.md, Dimens.lg, 0),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Expanded(
                    child: Text(
                      region != null
                          ? '${s.observationsInRegion}: ${region.name}'
                          : s.observationsInView,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: Theme.of(context).textTheme.titleSmall,
                    ),
                  ),
                  if (async.value != null)
                    Text(
                      s.totalCount(async.value!.total),
                      style: TextStyle(color: eco.textMuted, fontSize: 12),
                    ),
                ],
              ),
              const SizedBox(height: Dimens.sm),
              const CategoryChipsBar(),
              const SizedBox(height: Dimens.sm),
              FilterChipRow<ObservationStatus>(
                selected: {?status},
                onChanged: (v) => ref
                    .read(statusFilterProvider.notifier)
                    .set(v.isEmpty ? null : v.first),
                options: [
                  for (final st in ObservationStatus.lifecycle)
                    FilterOption(
                      value: st,
                      label: s.statusLabel(st),
                      icon: statusIcon(st),
                      color: eco.statusColor(st),
                    ),
                ],
              ),
              const SizedBox(height: Dimens.sm),
            ],
          ),
        ),
        const Divider(),
        Expanded(
          child: async.when(
            loading: () => const Padding(
              padding: EdgeInsets.all(Dimens.lg),
              child: SkeletonLoader(lines: 6, itemHeight: 64),
            ),
            error: (e, _) => ErrorState(
              error: e,
              onRetry: () => ref.invalidate(observationsListProvider),
            ),
            data: (data) {
              if (data.items.isEmpty) {
                return EmptyState(
                  icon: Icons.visibility_off_outlined,
                  title: s.noObservations,
                  message: region == null ? null : s.noObservationsInRegion,
                );
              }
              return ListView.builder(
                padding: const EdgeInsets.all(Dimens.sm),
                itemCount: data.items.length + (data.hasMore ? 1 : 0),
                itemBuilder: (context, i) {
                  if (i == data.items.length) {
                    return Padding(
                      padding: const EdgeInsets.all(Dimens.md),
                      child: Center(
                        child: data.loadingMore
                            ? const CircularProgressIndicator(strokeWidth: 2)
                            : OutlinedButton(
                                onPressed: () => ref
                                    .read(observationsListProvider.notifier)
                                    .loadMore(),
                                child: Text(
                                  data.loadMoreError != null ? s.retry : s.more,
                                ),
                              ),
                      ),
                    );
                  }
                  final o = data.items[i];
                  return ObservationTile(
                    observation: o,
                    selected: o.id == selectedId,
                    onTap: () => focusObservation(ref, o.id, o.lat, o.lon),
                  );
                },
              );
            },
          ),
        ),
      ],
    );
  }
}
