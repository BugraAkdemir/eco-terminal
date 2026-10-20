import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/providers.dart';
import '../../../core/l10n/format.dart';
import '../../../core/l10n/strings.dart';
import '../../../core/models/models.dart';
import '../../../core/theme/tokens.dart';
import '../../../shared_widgets/shared_widgets.dart';
import '../data/map_providers.dart';

/// Detail card (map overlay / bottom sheet / full page) for one observation.
class ObservationDetailCard extends ConsumerWidget {
  const ObservationDetailCard({
    super.key,
    required this.observationId,
    this.onClose,
    this.onOpenFull,
    this.scrollController,
  });

  final String observationId;
  final VoidCallback? onClose;
  final VoidCallback? onOpenFull;
  final ScrollController? scrollController;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = context.s;
    final async = ref.watch(observationDetailProvider(observationId));
    return SolidPanel(
      padding: EdgeInsets.zero,
      child: Column(
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(Dimens.lg, Dimens.sm, Dimens.sm, 0),
            child: Row(
              children: [
                Expanded(
                  child: Text(
                    s.detail,
                    style: Theme.of(context).textTheme.titleSmall,
                  ),
                ),
                if (onOpenFull != null)
                  IconButton(
                    tooltip: s.openFullPage,
                    onPressed: onOpenFull,
                    icon: const Icon(Icons.open_in_full, size: 18),
                  ),
                if (onClose != null)
                  IconButton(
                    tooltip: s.close,
                    onPressed: onClose,
                    icon: const Icon(Icons.close),
                  ),
              ],
            ),
          ),
          const Divider(),
          Expanded(
            child: async.when(
              loading: () => const Padding(
                padding: EdgeInsets.all(Dimens.lg),
                child: SkeletonLoader(lines: 5, itemHeight: 48),
              ),
              error: (e, _) => ErrorState(
                error: e,
                compact: true,
                onRetry: () =>
                    ref.invalidate(observationDetailProvider(observationId)),
              ),
              data: (d) => ObservationDetailBody(
                detail: d,
                scrollController: scrollController,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class ObservationDetailBody extends ConsumerWidget {
  const ObservationDetailBody({
    super.key,
    required this.detail,
    this.scrollController,
  });

  final ObservationDetail detail;
  final ScrollController? scrollController;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = context.s;
    final eco = context.eco;
    final textTheme = Theme.of(context).textTheme;
    final media = ref.watch(mediaApiProvider);
    final headers = ref.watch(authHeadersProvider);
    final cat = detail.category;

    return ListView(
      controller: scrollController,
      padding: const EdgeInsets.all(Dimens.lg),
      children: [
        Text(
          detail.displayTitle,
          style: textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w700),
        ),
        const SizedBox(height: Dimens.sm),
        Wrap(
          spacing: Dimens.sm,
          runSpacing: Dimens.sm,
          children: [
            StatusBadge(status: detail.status),
            _Pill(
              icon: categoryIcon(cat),
              label: s.categoryLabel(cat),
              color: eco.categoryColor(cat),
            ),
            if (detail.recordType == RecordType.issue)
              _Pill(
                icon: Icons.report_outlined,
                label: s.issueReport,
                color: eco.warning,
              ),
            if (detail.locationGeneralized)
              _Pill(
                icon: Icons.blur_on,
                label: s.locationGeneralized,
                color: eco.info,
              ),
            if (detail.isDemo)
              _Pill(
                icon: Icons.science_outlined,
                label: s.demoBadge,
                color: eco.textMuted,
              ),
          ],
        ),
        if (detail.locationGeneralized) ...[
          const SizedBox(height: Dimens.xs),
          Text(
            s.locationGeneralizedHint,
            style: textTheme.bodySmall?.copyWith(color: eco.textMuted),
          ),
        ],
        if (detail.isDemo) ...[
          const SizedBox(height: Dimens.xs),
          Text(
            s.demoRecord,
            style: textTheme.bodySmall?.copyWith(color: eco.textMuted),
          ),
        ],
        if (detail.media.isNotEmpty) ...[
          const SizedBox(height: Dimens.lg),
          SizedBox(
            height: 160,
            child: ListView.separated(
              scrollDirection: Axis.horizontal,
              itemCount: detail.media.length,
              separatorBuilder: (_, _) => const SizedBox(width: Dimens.sm),
              itemBuilder: (context, i) {
                final m = detail.media[i];
                return ClipRRect(
                  borderRadius: BorderRadius.circular(Dimens.radiusSm),
                  child: Image.network(
                    media.thumbUrlFor(m),
                    headers: headers,
                    height: 160,
                    width: m.width > 0 && m.height > 0
                        ? 160 * m.width / m.height
                        : 200,
                    fit: BoxFit.cover,
                    errorBuilder: (_, _, _) => Container(
                      width: 200,
                      height: 160,
                      color: eco.surfaceAlt,
                      child: Icon(Icons.broken_image_outlined,
                          color: eco.textMuted),
                    ),
                  ),
                );
              },
            ),
          ),
        ],
        const SizedBox(height: Dimens.lg),
        _Field(label: s.reference, value: detail.referenceNo),
        if (detail.speciesVerified.isNotEmpty)
          _Field(
            label: s.verifiedSpecies,
            value: detail.speciesVerified,
            valueColor: eco.green,
          ),
        if (detail.speciesGuess.isNotEmpty)
          _Field(label: s.userGuess, value: detail.speciesGuess),
        _Field(
          label: s.observedAt,
          value: formatDateTime(context, detail.observedAt),
        ),
        if (detail.primaryRegionName != null)
          _Field(label: s.region, value: detail.primaryRegionName!),
        if (detail.countObserved != null)
          _Field(label: s.count, value: '${detail.countObserved}'),
        if (detail.accuracyM != null)
          _Field(
            label: s.accuracy,
            value: '${detail.accuracyM!.round()} m',
          ),
        if (detail.ownerDisplayName.isNotEmpty)
          _Field(label: s.observer, value: detail.ownerDisplayName),
        if (detail.description.isNotEmpty) ...[
          const SizedBox(height: Dimens.sm),
          Text(s.notes, style: textTheme.labelMedium?.copyWith(color: eco.textMuted)),
          const SizedBox(height: 2),
          Text(detail.description, style: textTheme.bodyMedium),
        ],
        for (final a in detail.aiAnalysis)
          if (a.status == AiAnalysisStatus.done) _AiSuggestion(analysis: a),
        if (detail.history.isNotEmpty) ...[
          const SizedBox(height: Dimens.lg),
          SectionHeader(title: s.history),
          for (var i = 0; i < detail.history.length; i++)
            _HistoryTile(
              entry: detail.history[i],
              last: i == detail.history.length - 1,
            ),
        ],
      ],
    );
  }
}

class _Pill extends StatelessWidget {
  const _Pill({required this.icon, required this.label, required this.color});
  final IconData icon;
  final String label;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(999),
        border: Border.all(color: color.withValues(alpha: 0.6)),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(icon, size: 14, color: color),
          const SizedBox(width: 4),
          Flexible(
            child: Text(
              label,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                color: color,
                fontSize: 12,
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _Field extends StatelessWidget {
  const _Field({required this.label, required this.value, this.valueColor});
  final String label;
  final String value;
  final Color? valueColor;

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 3),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 120,
            child: Text(
              label,
              style: Theme.of(context)
                  .textTheme
                  .bodySmall
                  ?.copyWith(color: eco.textMuted),
            ),
          ),
          Expanded(
            child: Text(
              value,
              style: Theme.of(context)
                  .textTheme
                  .bodyMedium
                  ?.copyWith(color: valueColor),
            ),
          ),
        ],
      ),
    );
  }
}

class _AiSuggestion extends StatelessWidget {
  const _AiSuggestion({required this.analysis});
  final AIAnalysis analysis;

  @override
  Widget build(BuildContext context) {
    final s = context.s;
    final eco = context.eco;
    final parts = <String>[
      if (analysis.suggestedSpecies != null && analysis.suggestedSpecies!.isNotEmpty)
        analysis.suggestedSpecies!,
      if (analysis.suggestedCategory != null && analysis.suggestedCategory!.isNotEmpty)
        s.categoryLabelWire(analysis.suggestedCategory!),
    ];
    return Padding(
      padding: const EdgeInsets.only(top: Dimens.lg),
      child: Container(
        padding: const EdgeInsets.all(Dimens.md),
        decoration: BoxDecoration(
          color: eco.info.withValues(alpha: 0.08),
          borderRadius: BorderRadius.circular(Dimens.radius),
          border: Border.all(color: eco.info.withValues(alpha: 0.5)),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(Icons.auto_awesome, size: 16, color: eco.info),
                const SizedBox(width: 6),
                Expanded(
                  child: Text(
                    s.aiSuggestion,
                    style: TextStyle(
                      color: eco.info,
                      fontWeight: FontWeight.w700,
                      fontSize: 13,
                    ),
                  ),
                ),
                if (analysis.confidence != null)
                  Text(
                    '${(analysis.confidence! * 100).round()}%',
                    style: TextStyle(color: eco.textMuted, fontSize: 12),
                  ),
              ],
            ),
            if (parts.isNotEmpty) ...[
              const SizedBox(height: 6),
              Text(parts.join(' · ')),
            ],
            if (analysis.suggestedDescription != null &&
                analysis.suggestedDescription!.isNotEmpty) ...[
              const SizedBox(height: 4),
              Text(
                analysis.suggestedDescription!,
                style: Theme.of(context).textTheme.bodySmall,
              ),
            ],
            const SizedBox(height: 6),
            Text(
              analysis.disclaimer.isNotEmpty
                  ? analysis.disclaimer
                  : s.aiDisclaimerDefault,
              style: Theme.of(context)
                  .textTheme
                  .bodySmall
                  ?.copyWith(color: eco.textMuted, fontStyle: FontStyle.italic),
            ),
          ],
        ),
      ),
    );
  }
}

class _HistoryTile extends StatelessWidget {
  const _HistoryTile({required this.entry, required this.last});
  final StatusHistoryEntry entry;
  final bool last;

  @override
  Widget build(BuildContext context) {
    final s = context.s;
    final eco = context.eco;
    final color = eco.statusColor(entry.toStatus);
    return IntrinsicHeight(
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          SizedBox(
            width: 24,
            child: Column(
              children: [
                const SizedBox(height: 4),
                Container(
                  width: 12,
                  height: 12,
                  decoration: BoxDecoration(color: color, shape: BoxShape.circle),
                ),
                if (!last)
                  Expanded(
                    child: Container(width: 2, color: eco.border),
                  ),
              ],
            ),
          ),
          Expanded(
            child: Padding(
              padding: const EdgeInsets.only(bottom: Dimens.md),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    s.statusLabel(entry.toStatus),
                    style: TextStyle(color: color, fontWeight: FontWeight.w700),
                  ),
                  Text(
                    [
                      if (entry.actorName.isNotEmpty) entry.actorName,
                      formatDateTime(context, entry.createdAt),
                    ].where((e) => e.isNotEmpty).join(' · '),
                    style: Theme.of(context)
                        .textTheme
                        .bodySmall
                        ?.copyWith(color: eco.textMuted),
                  ),
                  if (entry.note.isNotEmpty)
                    Padding(
                      padding: const EdgeInsets.only(top: 2),
                      child: Text(entry.note),
                    ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}
