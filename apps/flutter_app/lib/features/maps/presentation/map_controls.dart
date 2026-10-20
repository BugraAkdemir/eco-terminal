import 'package:flutter/material.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/l10n/strings.dart';
import '../../../core/models/models.dart';
import '../../../core/theme/tokens.dart';
import '../../../shared_widgets/shared_widgets.dart';
import '../data/map_services.dart';
import '../domain/map_state.dart';
import 'map_view.dart';

/// Zoom, my-location, density and light/dark map toggles. Lives inside the
/// FlutterMap children so it can use the MapController.
class MapControls extends ConsumerWidget {
  const MapControls({super.key});

  Future<void> _locate(BuildContext context, WidgetRef ref) async {
    final controller = MapController.of(context);
    final s = context.s;
    final messenger = ScaffoldMessenger.of(context);
    final res = await ref.read(locationServiceProvider).current();
    if (res.outcome == LocationOutcome.ok && res.position != null) {
      ref.read(myLocationProvider.notifier).set(res.position);
      controller.move(res.position!, 14);
      return;
    }
    final msg = (res.outcome == LocationOutcome.denied ||
            res.outcome == LocationOutcome.deniedForever)
        ? s.locationDenied
        : s.locationUnavailable;
    messenger
      ..hideCurrentSnackBar()
      ..showSnackBar(SnackBar(content: Text(msg)));
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = context.s;
    final eco = context.eco;
    final controller = MapController.of(context);
    final camera = MapCamera.of(context);
    final dark = ref.watch(mapDarkProvider);
    final density = ref.watch(densityModeProvider);

    Widget btn(IconData icon, String tip, VoidCallback onTap,
        {bool active = false}) {
      return Tooltip(
        message: tip,
        child: Material(
          color: active ? eco.teal.withValues(alpha: 0.25) : Colors.transparent,
          child: InkWell(
            onTap: onTap,
            child: SizedBox(
              width: 40,
              height: 40,
              child: Icon(icon, size: 20, color: active ? eco.teal : eco.text),
            ),
          ),
        ),
      );
    }

    return Align(
      alignment: Alignment.bottomRight,
      child: Padding(
        padding: const EdgeInsets.only(right: Dimens.md, bottom: 36),
        child: GlassPanel(
          padding: EdgeInsets.zero,
          radius: Dimens.radius,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              btn(Icons.add, 'Zoom +', () {
                controller.move(camera.center, (camera.zoom + 1).clamp(4, 18));
              }),
              btn(Icons.remove, 'Zoom -', () {
                controller.move(camera.center, (camera.zoom - 1).clamp(4, 18));
              }),
              btn(Icons.my_location, s.myLocation, () => _locate(context, ref)),
              btn(
                Icons.blur_circular,
                s.densityMode,
                () => ref.read(densityModeProvider.notifier).set(!density),
                active: density,
              ),
              btn(
                dark ? Icons.dark_mode : Icons.light_mode,
                s.mapDark,
                () => ref.read(mapDarkProvider.notifier).set(!dark),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// The visible "active region" chip: the shared context for all panel tabs
/// and the AI assistant.
class ActiveRegionChip extends ConsumerWidget {
  const ActiveRegionChip({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = context.s;
    final eco = context.eco;
    final region = ref.watch(selectedRegionProvider);
    final active = region != null;
    return Container(
      key: const ValueKey('active-region-chip'),
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      decoration: BoxDecoration(
        color: (active ? eco.green : eco.textMuted).withValues(alpha: 0.14),
        borderRadius: BorderRadius.circular(999),
        border: Border.all(color: active ? eco.green : eco.border),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.place, size: 16, color: active ? eco.green : eco.textMuted),
          const SizedBox(width: 6),
          Flexible(
            child: Text(
              active ? '${s.activeRegion}: ${region.name}' : s.noRegionSelected,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w600,
                color: active ? eco.text : eco.textMuted,
              ),
            ),
          ),
          if (active) ...[
            const SizedBox(width: 4),
            InkWell(
              onTap: () => ref.read(selectedRegionProvider.notifier).clear(),
              borderRadius: BorderRadius.circular(999),
              child: Tooltip(
                message: s.clearRegion,
                child: Padding(
                  padding: const EdgeInsets.all(2),
                  child: Icon(Icons.close, size: 16, color: eco.textMuted),
                ),
              ),
            ),
          ],
        ],
      ),
    );
  }
}

/// Category filter chips shown over the map.
class CategoryChipsBar extends ConsumerWidget {
  const CategoryChipsBar({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = context.s;
    final eco = context.eco;
    final selected = ref.watch(categoryFilterProvider);
    return FilterChipRow<ObservationCategory>(
      multiSelect: true,
      selected: selected,
      onChanged: (v) => ref.read(categoryFilterProvider.notifier).set(v),
      options: [
        for (final c in ObservationCategory.values)
          FilterOption(
            value: c,
            label: s.categoryLabel(c),
            icon: categoryIcon(c),
            color: eco.categoryColor(c),
          ),
      ],
    );
  }
}
