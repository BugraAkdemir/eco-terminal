import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:latlong2/latlong.dart';

import '../../../core/l10n/strings.dart';
import '../../../core/router/route_access.dart';
import '../../../core/theme/tokens.dart';
import '../../../shared_widgets/shared_widgets.dart';
import '../data/map_providers.dart';
import '../domain/map_state.dart';
import 'map_controls.dart';
import 'map_view.dart';
import 'observation_detail.dart';

/// Map + floating chrome: category chips, active-region chip, loading/capped
/// notes and the observation detail card.
class MapStack extends ConsumerWidget {
  const MapStack({super.key, this.showRegionChip = true});

  /// On mobile the region chip is shown by the page itself.
  final bool showRegionChip;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = context.s;
    final selectedId = ref.watch(selectedObservationIdProvider);
    final viewport = ref.watch(viewportObservationsProvider);

    return LayoutBuilder(
      builder: (context, constraints) {
        final narrow = constraints.maxWidth < Dimens.mobileMax;
        final cardHeight = narrow
            ? constraints.maxHeight * 0.58
            : math.max(240.0, constraints.maxHeight - 140);
        final total = viewport.value?.meta.total ?? 0;
        final shown = viewport.value?.items.length ?? 0;

        return Stack(
          children: [
            const Positioned.fill(child: MapView()),
            Positioned(
              top: Dimens.md,
              left: Dimens.md,
              right: Dimens.md,
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  if (showRegionChip) const ActiveRegionChip(),
                  const SizedBox(height: Dimens.sm),
                  const CategoryChipsBar(),
                ],
              ),
            ),
            if (viewport.isLoading)
              const Positioned(
                top: 0,
                left: 0,
                right: 0,
                child: LinearProgressIndicator(minHeight: 2),
              ),
            if (viewport.hasError && !viewport.isLoading)
              Positioned(
                left: Dimens.md,
                right: Dimens.md,
                bottom: Dimens.xxl,
                child: Align(
                  alignment: Alignment.bottomCenter,
                  child: ConstrainedBox(
                    constraints: const BoxConstraints(maxWidth: 420),
                    child: GlassPanel(
                      child: Row(
                        children: [
                          Expanded(child: Text(s.errorText(viewport.error!))),
                          TextButton(
                            onPressed: () =>
                                ref.invalidate(viewportObservationsProvider),
                            child: Text(s.retry),
                          ),
                        ],
                      ),
                    ),
                  ),
                ),
              ),
            if (total > shown && shown > 0)
              Positioned(
                left: Dimens.md,
                bottom: 36,
                child: GlassPanel(
                  padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
                  child: Text(
                    s.mapMarkersCapped(shown, total),
                    style: const TextStyle(fontSize: 12),
                  ),
                ),
              ),
            if (selectedId != null)
              Positioned(
                left: narrow ? 0 : Dimens.md,
                right: narrow ? 0 : null,
                bottom: narrow ? 0 : null,
                top: narrow ? null : 96,
                width: narrow ? null : 380,
                height: cardHeight,
                child: ObservationDetailCard(
                  observationId: selectedId,
                  onClose: () => ref
                      .read(selectedObservationIdProvider.notifier)
                      .select(null),
                  onOpenFull: () {
                    context.push(AppPaths.observation(selectedId));
                  },
                ),
              ),
          ],
        );
      },
    );
  }
}

/// Focus helper used by lists/search: select the observation and centre it.
void focusObservation(WidgetRef ref, String id, double lat, double lon) {
  ref.read(selectedObservationIdProvider.notifier).select(id);
  ref.read(mapFocusProvider.notifier).flyTo(LatLng(lat, lon), zoom: 14);
}
