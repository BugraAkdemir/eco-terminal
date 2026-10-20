import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:latlong2/latlong.dart';

import '../../../core/models/models.dart';

/// Clock seam for tests.
final nowProvider = Provider<DateTime>((ref) => DateTime.now().toUtc());

// ---------------------------------------------------------------------------
// Date range
// ---------------------------------------------------------------------------

enum DatePreset { days30, year1, years5, custom }

class DateRangeFilter {
  const DateRangeFilter({
    this.preset = DatePreset.year1,
    this.customFrom,
    this.customTo,
  });

  final DatePreset preset;
  final DateTime? customFrom;
  final DateTime? customTo;

  DateTime from(DateTime now) {
    switch (preset) {
      case DatePreset.days30:
        return now.subtract(const Duration(days: 30));
      case DatePreset.year1:
        return DateTime.utc(now.year - 1, now.month, now.day);
      case DatePreset.years5:
        return DateTime.utc(now.year - 5, now.month, now.day);
      case DatePreset.custom:
        return customFrom ?? DateTime.utc(now.year - 1, now.month, now.day);
    }
  }

  DateTime to(DateTime now) =>
      preset == DatePreset.custom ? (customTo ?? now) : now;
}

class DateRangeController extends Notifier<DateRangeFilter> {
  @override
  DateRangeFilter build() => const DateRangeFilter();

  void setPreset(DatePreset p) => state = DateRangeFilter(
        preset: p,
        customFrom: state.customFrom,
        customTo: state.customTo,
      );

  void setCustom(DateTime from, DateTime to) => state = DateRangeFilter(
        preset: DatePreset.custom,
        customFrom: from,
        customTo: to,
      );
}

final dateRangeProvider =
    NotifierProvider<DateRangeController, DateRangeFilter>(
  DateRangeController.new,
);

/// Resolved (from, to) pair, day-granular so keys stay stable within a day.
class ResolvedRange {
  const ResolvedRange(this.from, this.to);
  final DateTime from;
  final DateTime to;

  @override
  bool operator ==(Object other) =>
      other is ResolvedRange && other.from == from && other.to == to;
  @override
  int get hashCode => Object.hash(from, to);
}

final resolvedRangeProvider = Provider<ResolvedRange>((ref) {
  final f = ref.watch(dateRangeProvider);
  final now = ref.watch(nowProvider);
  DateTime day(DateTime d) => DateTime.utc(d.year, d.month, d.day);
  return ResolvedRange(day(f.from(now)), day(f.to(now)));
});

// ---------------------------------------------------------------------------
// Layers
// ---------------------------------------------------------------------------

enum MapLayerId {
  citizen,
  wildlife,
  plant,
  water,
  forest,
  issues,
  official,
  boundaries,
  analysis;

  /// Id used by extension overlays (see MapOverlayLayerIds).
  String get overlayId =>
      this == official ? 'official_announcements' : name;

  bool get isObservationLayer => index <= MapLayerId.issues.index;
}

/// Which layer an observation belongs to. Issues always go to [issues].
MapLayerId layerForObservation(Observation o) {
  if (o.recordType == RecordType.issue) return MapLayerId.issues;
  switch (o.category) {
    case ObservationCategory.animal:
    case ObservationCategory.injuredWildlife:
      return MapLayerId.wildlife;
    case ObservationCategory.plant:
      return MapLayerId.plant;
    case ObservationCategory.water:
      return MapLayerId.water;
    case ObservationCategory.forest:
      return MapLayerId.forest;
    case ObservationCategory.pollution:
      return MapLayerId.issues;
    case ObservationCategory.other:
      return MapLayerId.citizen;
  }
}

class MapLayersController extends Notifier<Set<MapLayerId>> {
  @override
  Set<MapLayerId> build() => {
        MapLayerId.citizen,
        MapLayerId.wildlife,
        MapLayerId.plant,
        MapLayerId.water,
        MapLayerId.forest,
        MapLayerId.issues,
        MapLayerId.boundaries,
      };

  void toggle(MapLayerId id, bool on) {
    final next = Set<MapLayerId>.of(state);
    on ? next.add(id) : next.remove(id);
    state = next;
  }
}

final mapLayersProvider =
    NotifierProvider<MapLayersController, Set<MapLayerId>>(
  MapLayersController.new,
);

class BoolController extends Notifier<bool> {
  BoolController(this._initial);
  final bool _initial;
  @override
  bool build() => _initial;
  void set(bool v) => state = v;
}

/// Density ("heatmap-style") view instead of individual markers.
final densityModeProvider =
    NotifierProvider<BoolController, bool>(() => BoolController(false));

/// Dark raster tiles (colour filter) vs light tiles.
final mapDarkProvider =
    NotifierProvider<BoolController, bool>(() => BoolController(true));

/// Category filter chips; empty means all categories.
class CategoryFilterController extends Notifier<Set<ObservationCategory>> {
  @override
  Set<ObservationCategory> build() => {};
  void set(Set<ObservationCategory> v) => state = v;
}

final categoryFilterProvider =
    NotifierProvider<CategoryFilterController, Set<ObservationCategory>>(
  CategoryFilterController.new,
);

/// Optional status filter for the Observations tab.
class StatusFilterController extends Notifier<ObservationStatus?> {
  @override
  ObservationStatus? build() => null;
  void set(ObservationStatus? v) => state = v;
}

final statusFilterProvider =
    NotifierProvider<StatusFilterController, ObservationStatus?>(
  StatusFilterController.new,
);

// ---------------------------------------------------------------------------
// Shared context: selected region / observation / camera
// ---------------------------------------------------------------------------

/// THE shared UX context. Every panel tab (and the AI assistant) reads this.
class SelectedRegionController extends Notifier<Region?> {
  @override
  Region? build() => null;

  void select(Region? region) => state = region;
  void clear() => state = null;
}

final selectedRegionProvider =
    NotifierProvider<SelectedRegionController, Region?>(
  SelectedRegionController.new,
);

class SelectedObservationController extends Notifier<String?> {
  @override
  String? build() => null;
  void select(String? id) => state = id;
}

/// Id of the observation whose detail card is open.
final selectedObservationIdProvider =
    NotifierProvider<SelectedObservationController, String?>(
  SelectedObservationController.new,
);

/// One-shot camera request (search results, panel taps, my-location).
class MapFocus {
  const MapFocus._({this.center, this.zoom, this.bbox, required this.nonce});
  factory MapFocus.point(LatLng center, {double zoom = 13, required int nonce}) =>
      MapFocus._(center: center, zoom: zoom, nonce: nonce);
  factory MapFocus.bounds(List<double> bbox, {required int nonce}) =>
      MapFocus._(bbox: bbox, nonce: nonce);

  final LatLng? center;
  final double? zoom;

  /// `[minLon, minLat, maxLon, maxLat]`
  final List<double>? bbox;
  final int nonce;
}

class MapFocusController extends Notifier<MapFocus?> {
  int _n = 0;
  @override
  MapFocus? build() => null;

  void flyTo(LatLng p, {double zoom = 13}) =>
      state = MapFocus.point(p, zoom: zoom, nonce: ++_n);

  void fitBounds(List<double> bbox) =>
      state = MapFocus.bounds(bbox, nonce: ++_n);
}

final mapFocusProvider =
    NotifierProvider<MapFocusController, MapFocus?>(MapFocusController.new);

/// Visible map window, updated (debounced) when the camera settles.
class MapViewport {
  const MapViewport({required this.bbox, required this.zoom});

  /// `[minLon, minLat, maxLon, maxLat]`
  final List<double> bbox;
  final double zoom;

  @override
  bool operator ==(Object other) =>
      other is MapViewport &&
      other.zoom.round() == zoom.round() &&
      _same(other.bbox, bbox);

  static bool _same(List<double> a, List<double> b) {
    for (var i = 0; i < 4; i++) {
      if ((a[i] - b[i]).abs() > 0.0005) return false;
    }
    return true;
  }

  @override
  int get hashCode => Object.hash(
        zoom.round(),
        bbox.map((e) => (e * 2000).round()).join(','),
      );
}

class ViewportController extends Notifier<MapViewport?> {
  @override
  MapViewport? build() => null;
  void set(MapViewport v) {
    if (state != v) state = v;
  }
}

final viewportProvider =
    NotifierProvider<ViewportController, MapViewport?>(ViewportController.new);

/// Last camera (kept so the map keeps its place when the shell swaps pages).
class CameraState {
  const CameraState(this.center, this.zoom);
  final LatLng center;
  final double zoom;
}

class CameraController extends Notifier<CameraState?> {
  @override
  CameraState? build() => null;
  void set(CameraState c) => state = c;
}

final cameraProvider =
    NotifierProvider<CameraController, CameraState?>(CameraController.new);
