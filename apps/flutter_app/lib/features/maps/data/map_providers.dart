import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_riverpod/misc.dart' show ProviderListenable;

import '../../../core/api/providers.dart';
import '../../../core/api/services/observations_api.dart';
import '../../../core/models/models.dart';
import '../../../core/session/session_controller.dart';
import '../domain/geo.dart';
import '../domain/map_state.dart';

/// Server page size for map requests (API max is 200): bounded marker count.
const int kMapPageSize = 200;

/// Page size for list views.
const int kListPageSize = 30;

/// All regions with geometry, parsed once (keep-alive).
final regionShapesProvider = FutureProvider<List<RegionShape>>((ref) async {
  final regions = await ref.watch(regionsApiProvider).list(geometry: true);
  return shapesFromRegions(regions);
});

/// Observations in the current viewport (server-paginated, capped).
final viewportObservationsProvider =
    FutureProvider<Paged<Observation>>((ref) async {
  final vp = ref.watch(viewportProvider);
  // Re-query after login/logout: visibility depends on the caller.
  ref.watch(sessionProvider.select((s) => s.token));
  final range = ref.watch(resolvedRangeProvider);
  final cats = ref.watch(categoryFilterProvider);
  if (vp == null) return Paged<Observation>.empty();
  return ref.watch(observationsApiProvider).list(
        ObservationQuery(
          bbox: vp.bbox,
          category: cats.length == 1 ? cats.first.wire : null,
          from: range.from,
          to: range.to,
          pageSize: kMapPageSize,
        ),
      );
});

/// Markers to draw: viewport result filtered by layer toggles + categories.
final visibleObservationsProvider = Provider<List<Observation>>((ref) {
  final items =
      ref.watch(viewportObservationsProvider).value?.items ?? const [];
  final layers = ref.watch(mapLayersProvider);
  final cats = ref.watch(categoryFilterProvider);
  if (!layers.contains(MapLayerId.citizen)) return const [];
  return [
    for (final o in items)
      if (layers.contains(layerForObservation(o)) &&
          (cats.isEmpty || cats.contains(o.category)))
        o,
  ];
});

/// Detail (media, AI, history) of one observation.
final observationDetailProvider =
    FutureProvider.family<ObservationDetail, String>((ref, id) {
  ref.watch(sessionProvider.select((s) => s.token));
  return ref.watch(observationsApiProvider).get(id);
});

/// Analytics for a region in the active date range.
final regionAnalyticsProvider =
    FutureProvider.family<RegionAnalytics, String>((ref, regionId) {
  final range = ref.watch(resolvedRangeProvider);
  return ref
      .watch(regionsApiProvider)
      .analytics(regionId, from: range.from, to: range.to);
});

/// Latest observations of a region (Information tab).
final regionLatestObservationsProvider =
    FutureProvider.family<Paged<Observation>, String>((ref, regionId) {
  ref.watch(sessionProvider.select((s) => s.token));
  final range = ref.watch(resolvedRangeProvider);
  return ref.watch(regionsApiProvider).observations(
        regionId,
        from: range.from,
        to: range.to,
        page: 1,
        pageSize: 5,
      );
});

/// Accumulated, paginated list backing the Observations tab.
class ObservationsListData {
  const ObservationsListData({
    required this.items,
    required this.total,
    required this.page,
    this.loadingMore = false,
    this.loadMoreError,
  });
  final List<Observation> items;
  final int total;
  final int page;
  final bool loadingMore;
  final Object? loadMoreError;

  bool get hasMore => items.length < total && page > 0;

  ObservationsListData copyWith({
    List<Observation>? items,
    int? total,
    int? page,
    bool? loadingMore,
    Object? loadMoreError,
    bool clearError = false,
  }) =>
      ObservationsListData(
        items: items ?? this.items,
        total: total ?? this.total,
        page: page ?? this.page,
        loadingMore: loadingMore ?? this.loadingMore,
        loadMoreError: clearError ? null : (loadMoreError ?? this.loadMoreError),
      );
}

class ObservationsListController extends AsyncNotifier<ObservationsListData> {
  @override
  Future<ObservationsListData> build() async {
    final res = await _fetch(1, watch: true);
    return ObservationsListData(
      items: _filter(res.items),
      total: res.meta.total,
      page: 1,
    );
  }

  T _dep<T>(ProviderListenable<T> p, bool watch) =>
      watch ? ref.watch(p) : ref.read(p);

  // `watch` is true inside build() (registers dependencies); loadMore() reads.
  Future<Paged<Observation>> _fetch(int page, {required bool watch}) {
    _dep(sessionProvider.select((s) => s.token), watch);
    final region = _dep(selectedRegionProvider, watch);
    final range = _dep(resolvedRangeProvider, watch);
    final cats = _dep(categoryFilterProvider, watch);
    final status = _dep(statusFilterProvider, watch);
    final category = cats.length == 1 ? cats.first.wire : null;
    if (region != null) {
      return ref.read(regionsApiProvider).observations(
            region.id,
            category: category,
            from: range.from,
            to: range.to,
            page: page,
            pageSize: kListPageSize,
          );
    }
    final vp = _dep(viewportProvider, watch);
    return ref.read(observationsApiProvider).list(
          ObservationQuery(
            bbox: vp?.bbox,
            category: category,
            status: status?.wire,
            from: range.from,
            to: range.to,
            page: page,
            pageSize: kListPageSize,
          ),
        );
  }

  List<Observation> _filter(List<Observation> items) {
    final cats = ref.read(categoryFilterProvider);
    final status = ref.read(statusFilterProvider);
    return [
      for (final o in items)
        if ((cats.isEmpty || cats.contains(o.category)) &&
            (status == null || o.status == status))
          o,
    ];
  }

  Future<void> loadMore() async {
    final cur = state.value;
    if (cur == null || cur.loadingMore || !cur.hasMore) return;
    state = AsyncData(cur.copyWith(loadingMore: true, clearError: true));
    try {
      final res = await _fetch(cur.page + 1, watch: false);
      state = AsyncData(
        cur.copyWith(
          items: [...cur.items, ..._filter(res.items)],
          total: res.meta.total,
          page: cur.page + 1,
          loadingMore: false,
        ),
      );
    } catch (e) {
      state = AsyncData(cur.copyWith(loadingMore: false, loadMoreError: e));
    }
  }
}

final observationsListProvider =
    AsyncNotifierProvider<ObservationsListController, ObservationsListData>(
  ObservationsListController.new,
);

/// Own submissions (profile "Bildirimlerim").
final myObservationsProvider = FutureProvider<Paged<Observation>>((ref) {
  ref.watch(sessionProvider.select((s) => s.token));
  return ref.watch(observationsApiProvider).list(
        const ObservationQuery(mine: true, pageSize: 50),
      );
});
