import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/providers.dart';
import '../../../core/l10n/format.dart';
import '../../../core/l10n/strings.dart';
import '../../../core/models/models.dart';
import '../../../core/theme/tokens.dart';
import '../../../shared_widgets/shared_widgets.dart';

/// List row for an observation: thumbnail / category icon, title, meta and a
/// status badge. Used by the Observations tab, the Information tab and the
/// profile "Bildirimlerim" list.
class ObservationTile extends ConsumerWidget {
  const ObservationTile({
    super.key,
    required this.observation,
    required this.onTap,
    this.selected = false,
  });

  final Observation observation;
  final VoidCallback onTap;
  final bool selected;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final eco = context.eco;
    final s = context.s;
    final o = observation;
    final catColor = eco.categoryColor(o.category);
    final media = ref.watch(mediaApiProvider);
    final headers = ref.watch(authHeadersProvider);

    Widget leading = Container(
      width: 48,
      height: 48,
      decoration: BoxDecoration(
        color: catColor.withValues(alpha: 0.18),
        borderRadius: BorderRadius.circular(10),
      ),
      child: Icon(categoryIcon(o.category), color: catColor),
    );
    if (o.thumbnailMediaId != null) {
      leading = ClipRRect(
        borderRadius: BorderRadius.circular(10),
        child: Image.network(
          media.thumbUrlForId(o.thumbnailMediaId!),
          headers: headers,
          width: 48,
          height: 48,
          fit: BoxFit.cover,
          errorBuilder: (_, _, _) => Container(
            width: 48,
            height: 48,
            color: catColor.withValues(alpha: 0.18),
            child: Icon(categoryIcon(o.category), color: catColor),
          ),
        ),
      );
    }

    final meta = [
      s.categoryLabel(o.category),
      formatDate(context, o.observedAt),
      if (o.primaryRegionName != null) o.primaryRegionName!,
    ].join(' · ');

    return Material(
      color: selected ? eco.teal.withValues(alpha: 0.10) : Colors.transparent,
      borderRadius: BorderRadius.circular(Dimens.radius),
      child: InkWell(
        borderRadius: BorderRadius.circular(Dimens.radius),
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.all(Dimens.sm),
          child: Row(
            children: [
              leading,
              const SizedBox(width: Dimens.md),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      o.displayTitle,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(fontWeight: FontWeight.w600),
                    ),
                    const SizedBox(height: 2),
                    Text(
                      meta,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: Theme.of(context)
                          .textTheme
                          .bodySmall
                          ?.copyWith(color: eco.textMuted),
                    ),
                    const SizedBox(height: 4),
                    Wrap(
                      spacing: 6,
                      runSpacing: 4,
                      crossAxisAlignment: WrapCrossAlignment.center,
                      children: [
                        StatusBadge(status: o.status, compact: true, explain: false),
                        if (o.locationGeneralized)
                          Tooltip(
                            message: s.locationGeneralized,
                            child: Icon(Icons.blur_on, size: 16, color: eco.info),
                          ),
                        if (o.isDemo)
                          Text(
                            s.demoBadge,
                            style: TextStyle(fontSize: 11, color: eco.textMuted),
                          ),
                      ],
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
