import 'package:fl_chart/fl_chart.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../../core/l10n/format.dart';
import '../../../../core/l10n/strings.dart';
import '../../../../core/models/models.dart';
import '../../../../core/theme/tokens.dart';
import '../../../../shared_widgets/shared_widgets.dart';
import '../../data/map_providers.dart';
import '../../domain/map_state.dart';
import '../map_stack.dart';
import '../observation_tile.dart';

/// Panel tab "Information": summary of the selected region from
/// GET /regions/{id}/analytics (all numbers computed server-side by SQL).
class InformationTab extends ConsumerWidget {
  const InformationTab({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final region = ref.watch(selectedRegionProvider);
    if (region == null) return const _NoRegion();
    final async = ref.watch(regionAnalyticsProvider(region.id));
    final s = context.s;
    return ListView(
      padding: const EdgeInsets.all(Dimens.lg),
      children: [
        _RegionHeader(region: region),
        const SizedBox(height: Dimens.md),
        async.when(
          loading: () => const SkeletonLoader(lines: 5, itemHeight: 64),
          error: (e, _) => ErrorState(
            error: e,
            compact: true,
            onRetry: () => ref.invalidate(regionAnalyticsProvider(region.id)),
          ),
          data: (a) => _AnalyticsView(regionId: region.id, analytics: a),
        ),
        const SizedBox(height: Dimens.lg),
        SectionHeader(title: s.latestObservations),
        _LatestObservations(regionId: region.id),
      ],
    );
  }
}

class _NoRegion extends ConsumerWidget {
  const _NoRegion();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = context.s;
    final shapes = ref.watch(regionShapesProvider);
    final quick = shapes.value
            ?.where((x) =>
                x.region.kind == RegionKind.province ||
                x.region.kind == RegionKind.district)
            .take(12)
            .toList() ??
        const [];
    return ListView(
      padding: const EdgeInsets.all(Dimens.lg),
      children: [
        EmptyState(
          icon: Icons.touch_app_outlined,
          title: s.noRegionSelected,
          message: s.selectRegionHint,
        ),
        if (shapes.hasError)
          ErrorState(
            error: shapes.error!,
            compact: true,
            onRetry: () => ref.invalidate(regionShapesProvider),
          ),
        if (quick.isNotEmpty) ...[
          SectionHeader(title: s.quickRegions),
          Wrap(
            spacing: Dimens.sm,
            runSpacing: Dimens.sm,
            children: [
              for (final q in quick)
                ActionChip(
                  label: Text(q.region.name),
                  onPressed: () {
                    ref.read(selectedRegionProvider.notifier).select(q.region);
                    ref.read(mapFocusProvider.notifier).fitBounds(q.bbox);
                  },
                ),
            ],
          ),
        ],
      ],
    );
  }
}

class _RegionHeader extends ConsumerWidget {
  const _RegionHeader({required this.region});
  final Region region;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = context.s;
    final eco = context.eco;
    return Row(
      children: [
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                region.name,
                style: Theme.of(context)
                    .textTheme
                    .titleMedium
                    ?.copyWith(fontWeight: FontWeight.w700),
              ),
              Text(
                [
                  region.kind.name,
                  if (region.isDemo) s.demoBadge,
                ].join(' · '),
                style: Theme.of(context)
                    .textTheme
                    .bodySmall
                    ?.copyWith(color: eco.textMuted),
              ),
            ],
          ),
        ),
        IconButton(
          tooltip: s.clearRegion,
          onPressed: () => ref.read(selectedRegionProvider.notifier).clear(),
          icon: const Icon(Icons.close),
        ),
      ],
    );
  }
}

class _AnalyticsView extends StatelessWidget {
  const _AnalyticsView({required this.regionId, required this.analytics});
  final String regionId;
  final RegionAnalytics analytics;

  @override
  Widget build(BuildContext context) {
    final s = context.s;
    final eco = context.eco;
    final t = analytics.totals;
    final q = analytics.dataQuality;
    final hasData = analytics.byCategory.isNotEmpty || t.observations + t.issues > 0;

    Widget metric(String label, int value, IconData icon, Color color) =>
        SizedBox(
          width: 150,
          child: MetricCard(
            label: label,
            value: formatNumber(context, value),
            icon: icon,
            accent: color,
          ),
        );

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Wrap(
          spacing: Dimens.sm,
          runSpacing: Dimens.sm,
          children: [
            metric(s.totalObservations, t.observations, Icons.visibility, eco.teal),
            metric(s.totalIssues, t.issues, Icons.report_outlined, eco.warning),
            metric(s.totalVerified, t.verified, Icons.verified, eco.green),
            metric(s.totalPending, t.pendingReview, Icons.hourglass_top, eco.info),
          ],
        ),
        const SizedBox(height: Dimens.lg),
        if (q.warnings.isNotEmpty) _QualityWarnings(quality: q),
        if (q.warnings.isEmpty && hasData) _QualityNote(quality: q),
        const SizedBox(height: Dimens.lg),
        if (!hasData)
          EmptyState(
            compact: true,
            icon: Icons.insights_outlined,
            title: s.noObservationsInRegion,
          )
        else ...[
          SectionHeader(title: s.categoryDistribution),
          _CategoryChart(items: analytics.byCategory),
          const SizedBox(height: Dimens.lg),
          if (analytics.byMonth.isNotEmpty) ...[
            SectionHeader(title: s.monthlyTrend),
            _MonthlyChart(items: analytics.byMonth),
            const SizedBox(height: Dimens.lg),
          ],
          if (analytics.topSpecies.isNotEmpty) ...[
            SectionHeader(title: s.topSpecies),
            for (final sp in analytics.topSpecies.take(5))
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 3),
                child: Row(
                  children: [
                    Expanded(child: Text(sp.species)),
                    Text(
                      '${sp.count}',
                      style: TextStyle(
                        color: eco.textMuted,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  ],
                ),
              ),
          ],
        ],
        if (analytics.environmental != null &&
            analytics.environmental!.isNotEmpty) ...[
          const SizedBox(height: Dimens.lg),
          SectionHeader(title: s.environmental),
          _Environmental(data: analytics.environmental!),
        ],
      ],
    );
  }
}

class _QualityWarnings extends StatelessWidget {
  const _QualityWarnings({required this.quality});
  final DataQuality quality;

  @override
  Widget build(BuildContext context) {
    final s = context.s;
    final eco = context.eco;
    return Container(
      key: const ValueKey('data-quality-warnings'),
      padding: const EdgeInsets.all(Dimens.md),
      decoration: BoxDecoration(
        color: eco.warning.withValues(alpha: 0.10),
        borderRadius: BorderRadius.circular(Dimens.radius),
        border: Border.all(color: eco.warning.withValues(alpha: 0.6)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(Icons.warning_amber_rounded, size: 18, color: eco.warning),
              const SizedBox(width: 6),
              Expanded(
                child: Text(
                  s.dataQualityWarnings,
                  style: TextStyle(
                    color: eco.warning,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: 6),
          for (final w in quality.warnings)
            Padding(
              padding: const EdgeInsets.only(top: 4),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text('• '),
                  Expanded(child: Text(w)),
                ],
              ),
            ),
          const SizedBox(height: 6),
          Text(
            '${s.distinctObservers(quality.distinctObservers)} · ${s.monthsCoverage(quality.monthsWithData, quality.monthsInPeriod)}',
            style: Theme.of(context)
                .textTheme
                .bodySmall
                ?.copyWith(color: eco.textMuted),
          ),
        ],
      ),
    );
  }
}

class _QualityNote extends StatelessWidget {
  const _QualityNote({required this.quality});
  final DataQuality quality;

  @override
  Widget build(BuildContext context) {
    final s = context.s;
    final eco = context.eco;
    return SolidPanel(
      padding: const EdgeInsets.all(Dimens.md),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(s.dataQuality, style: const TextStyle(fontWeight: FontWeight.w700)),
          const SizedBox(height: 4),
          Text(
            '${s.distinctObservers(quality.distinctObservers)} · ${s.monthsCoverage(quality.monthsWithData, quality.monthsInPeriod)}',
            style: TextStyle(color: eco.textMuted, fontSize: 13),
          ),
          const SizedBox(height: 4),
          Text(
            s.samplingNote,
            style: TextStyle(color: eco.textMuted, fontSize: 12),
          ),
        ],
      ),
    );
  }
}

class _CategoryChart extends StatelessWidget {
  const _CategoryChart({required this.items});
  final List<CategoryCount> items;

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    final s = context.s;
    final total = items.fold<int>(0, (a, b) => a + b.count);
    if (total == 0) return const SizedBox.shrink();
    return Row(
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        SizedBox(
          width: 130,
          height: 130,
          child: PieChart(
            PieChartData(
              sectionsSpace: 2,
              centerSpaceRadius: 32,
              sections: [
                for (final c in items)
                  if (c.count > 0)
                    PieChartSectionData(
                      value: c.count.toDouble(),
                      color: eco.categoryColor(
                        ObservationCategory.fromWire(c.category),
                      ),
                      radius: 22,
                      showTitle: false,
                    ),
              ],
            ),
          ),
        ),
        const SizedBox(width: Dimens.lg),
        Expanded(
          child: Column(
            children: [
              for (final c in items)
                Padding(
                  padding: const EdgeInsets.symmetric(vertical: 3),
                  child: Row(
                    children: [
                      Container(
                        width: 10,
                        height: 10,
                        decoration: BoxDecoration(
                          color: eco.categoryColor(
                            ObservationCategory.fromWire(c.category),
                          ),
                          shape: BoxShape.circle,
                        ),
                      ),
                      const SizedBox(width: 8),
                      Expanded(
                        child: Text(
                          s.categoryLabelWire(c.category),
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                        ),
                      ),
                      Text(
                        '${c.count}',
                        style: TextStyle(
                          color: eco.textMuted,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                    ],
                  ),
                ),
            ],
          ),
        ),
      ],
    );
  }
}

class _MonthlyChart extends StatelessWidget {
  const _MonthlyChart({required this.items});
  final List<MonthCount> items;

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    final maxY = items.fold<int>(1, (m, e) => e.count > m ? e.count : m);
    final step = (items.length / 6).ceil().clamp(1, 12);
    return SizedBox(
      height: 140,
      child: BarChart(
        BarChartData(
          maxY: maxY * 1.15,
          gridData: const FlGridData(show: false),
          borderData: FlBorderData(show: false),
          barTouchData: BarTouchData(enabled: false),
          titlesData: FlTitlesData(
            leftTitles: const AxisTitles(),
            topTitles: const AxisTitles(),
            rightTitles: const AxisTitles(),
            bottomTitles: AxisTitles(
              sideTitles: SideTitles(
                showTitles: true,
                reservedSize: 22,
                getTitlesWidget: (value, meta) {
                  final i = value.toInt();
                  if (i < 0 || i >= items.length || i % step != 0) {
                    return const SizedBox.shrink();
                  }
                  final m = items[i].month;
                  return Padding(
                    padding: const EdgeInsets.only(top: 4),
                    child: Text(
                      m.length >= 7 ? m.substring(2, 7) : m,
                      style: TextStyle(fontSize: 10, color: eco.textMuted),
                    ),
                  );
                },
              ),
            ),
          ),
          barGroups: [
            for (var i = 0; i < items.length; i++)
              BarChartGroupData(
                x: i,
                barRods: [
                  BarChartRodData(
                    toY: items[i].count.toDouble(),
                    width: items.length > 24 ? 4 : 8,
                    color: eco.teal,
                    borderRadius: BorderRadius.circular(2),
                  ),
                ],
              ),
          ],
        ),
      ),
    );
  }
}

class _Environmental extends StatelessWidget {
  const _Environmental({required this.data});
  final Map<String, dynamic> data;

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    final rows = <MapEntry<String, String>>[];
    void walk(String prefix, Object? v) {
      if (v is Map) {
        v.forEach((k, val) => walk(prefix.isEmpty ? '$k' : '$prefix · $k', val));
      } else if (v != null && '$v'.isNotEmpty) {
        rows.add(MapEntry(prefix, v is double ? v.toStringAsFixed(1) : '$v'));
      }
    }

    walk('', data);
    return SolidPanel(
      padding: const EdgeInsets.all(Dimens.md),
      child: Column(
        children: [
          for (final r in rows.take(12))
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 2),
              child: Row(
                children: [
                  Expanded(
                    child: Text(
                      r.key,
                      style: TextStyle(color: eco.textMuted, fontSize: 13),
                    ),
                  ),
                  Text(r.value, style: const TextStyle(fontWeight: FontWeight.w600)),
                ],
              ),
            ),
        ],
      ),
    );
  }
}

class _LatestObservations extends ConsumerWidget {
  const _LatestObservations({required this.regionId});
  final String regionId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final async = ref.watch(regionLatestObservationsProvider(regionId));
    final s = context.s;
    return async.when(
      loading: () => const SkeletonLoader(lines: 3, itemHeight: 56),
      error: (e, _) => ErrorState(
        error: e,
        compact: true,
        onRetry: () => ref.invalidate(regionLatestObservationsProvider(regionId)),
      ),
      data: (page) {
        if (page.items.isEmpty) {
          return EmptyState(
            compact: true,
            icon: Icons.visibility_off_outlined,
            title: s.noObservations,
          );
        }
        return Column(
          children: [
            for (final o in page.items)
              ObservationTile(
                observation: o,
                onTap: () => focusObservation(ref, o.id, o.lat, o.lon),
              ),
          ],
        );
      },
    );
  }
}
