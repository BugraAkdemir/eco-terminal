import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:flutter_map_marker_cluster/flutter_map_marker_cluster.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:latlong2/latlong.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../../core/config/app_config.dart';
import '../../../core/extensions/extension_registry.dart';
import '../../../core/l10n/strings.dart';
import '../../../core/models/models.dart';
import '../../../core/theme/tokens.dart';
import '../data/map_providers.dart';
import '../data/map_services.dart';
import '../domain/geo.dart';
import '../domain/map_state.dart';
import 'map_controls.dart';
import 'observation_marker.dart';

/// The interactive map: tiles, region polygons, clustered observation markers
/// (or density view), extension overlays. Camera moves are debounced before
/// they update [viewportProvider] (which triggers the bbox refetch).
class MapView extends ConsumerStatefulWidget {
  const MapView({super.key});

  @override
  ConsumerState<MapView> createState() => _MapViewState();
}

class _MapViewState extends ConsumerState<MapView> {
  final MapController _controller = MapController();
  Timer? _debounce;
  bool _ready = false;

  // Marker cache: rebuilt only when the observation list instance changes,
  // not on every camera frame or unrelated rebuild.
  List<Observation>? _markerSource;
  List<Marker> _markers = const [];
  Map<Key, Observation> _byKey = const {};

  static const _debounceDelay = Duration(milliseconds: 450);

  @override
  void dispose() {
    _debounce?.cancel();
    _controller.dispose();
    super.dispose();
  }

  void _publishViewport({bool immediate = false}) {
    _debounce?.cancel();
    void publish() {
      if (!mounted || !_ready) return;
      final cam = _controller.camera;
      final b = cam.visibleBounds;
      ref.read(viewportProvider.notifier).set(
            MapViewport(
              bbox: [b.west, b.south, b.east, b.north],
              zoom: cam.zoom,
            ),
          );
      ref.read(cameraProvider.notifier).set(CameraState(cam.center, cam.zoom));
    }

    if (immediate) {
      publish();
    } else {
      _debounce = Timer(_debounceDelay, publish);
    }
  }

  void _applyFocus(MapFocus? f) {
    if (f == null || !_ready) return;
    if (f.bbox != null) {
      final b = f.bbox!;
      _controller.fitCamera(
        CameraFit.bounds(
          bounds: LatLngBounds(LatLng(b[1], b[0]), LatLng(b[3], b[2])),
          padding: const EdgeInsets.all(48),
          maxZoom: 14,
        ),
      );
    } else if (f.center != null) {
      _controller.move(f.center!, f.zoom ?? 13);
    }
  }

  void _onTap(LatLng point) {
    final shapes = ref.read(regionShapesProvider).value;
    final layers = ref.read(mapLayersProvider);
    ref.read(selectedObservationIdProvider.notifier).select(null);
    if (shapes == null) return;
    bool allowed(Region r) {
      switch (r.kind) {
        case RegionKind.grid:
        case RegionKind.custom:
          return layers.contains(MapLayerId.analysis);
        case RegionKind.unknown:
          return false;
        default:
          return layers.contains(MapLayerId.boundaries);
      }
    }

    final hit = pickRegionAt(point, shapes, allowed: allowed);
    if (hit != null) {
      ref.read(selectedRegionProvider.notifier).select(hit.region);
    }
  }

  void _rebuildMarkers(List<Observation> list) {
    if (identical(list, _markerSource)) return;
    _markerSource = list;
    final byKey = <Key, Observation>{};
    _markers = [
      for (final o in list)
        () {
          final key = ValueKey('obs-${o.id}');
          byKey[key] = o;
          return Marker(
            key: key,
            point: LatLng(o.lat, o.lon),
            width: 36,
            height: 36,
            child: GestureDetector(
              behavior: HitTestBehavior.opaque,
              onTap: () => ref
                  .read(selectedObservationIdProvider.notifier)
                  .select(o.id),
              child: ObservationMarkerWidget(observation: o),
            ),
          );
        }(),
    ];
    _byKey = byKey;
  }

  List<Polygon> _polygons(
    List<RegionShape> shapes,
    Set<MapLayerId> layers,
    Region? selected,
    EcoColors eco,
  ) {
    final out = <Polygon>[];
    for (final s in shapes) {
      final r = s.region;
      final isAnalysis =
          r.kind == RegionKind.grid || r.kind == RegionKind.custom;
      final on = isAnalysis
          ? layers.contains(MapLayerId.analysis)
          : layers.contains(MapLayerId.boundaries);
      final isSelected = selected != null && selected.id == r.id;
      if (!on && !isSelected) continue;
      final base = isAnalysis ? eco.warning : eco.teal;
      for (final poly in s.polygons) {
        out.add(
          Polygon(
            points: poly.outer,
            holePointsList: poly.holes.isEmpty ? null : poly.holes,
            color: isSelected
                ? eco.green.withValues(alpha: 0.20)
                : base.withValues(alpha: 0.05),
            borderColor: isSelected ? eco.green : base.withValues(alpha: 0.75),
            borderStrokeWidth: isSelected ? 3 : 1.2,
          ),
        );
      }
    }
    // Selected region on top.
    out.sort((a, b) =>
        (a.borderStrokeWidth == 3 ? 1 : 0) - (b.borderStrokeWidth == 3 ? 1 : 0));
    return out;
  }

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    final s = context.s;
    final layers = ref.watch(mapLayersProvider);
    final selected = ref.watch(selectedRegionProvider);
    final dark = ref.watch(mapDarkProvider);
    final density = ref.watch(densityModeProvider);
    final shapes = ref.watch(regionShapesProvider).value ?? const [];
    final visible = ref.watch(visibleObservationsProvider);
    final registry = ref.watch(extensionRegistryProvider);
    final tileProvider = ref.watch(tileProviderProvider);
    final urlTemplate = ref.watch(tileUrlProvider);
    final cam = ref.read(cameraProvider);

    ref.listen<MapFocus?>(mapFocusProvider, (_, f) => _applyFocus(f));

    _rebuildMarkers(visible);
    final polygons = _polygons(shapes, layers, selected, eco);
    final overlays = <Widget>[
      if (layers.contains(MapLayerId.official))
        for (final o in registry.overlaysFor(MapLayerId.official.overlayId))
          Builder(builder: o.builder),
    ];

    return FlutterMap(
      mapController: _controller,
      options: MapOptions(
        initialCenter: cam?.center ??
            const LatLng(MapConfig.startLat, MapConfig.startLon),
        initialZoom: cam?.zoom ?? MapConfig.startZoom,
        minZoom: 4,
        maxZoom: 18,
        backgroundColor: dark ? const Color(0xFF0B1620) : const Color(0xFFDDE6EA),
        interactionOptions: const InteractionOptions(
          flags: InteractiveFlag.all & ~InteractiveFlag.rotate,
        ),
        onMapReady: () {
          _ready = true;
          _publishViewport(immediate: true);
          _applyFocus(ref.read(mapFocusProvider));
        },
        onPositionChanged: (camera, hasGesture) => _publishViewport(),
        onTap: (tapPosition, point) => _onTap(point),
      ),
      children: [
        TileLayer(
          urlTemplate: urlTemplate,
          userAgentPackageName: MapConfig.userAgentPackageName,
          tileProvider: tileProvider,
          maxNativeZoom: 19,
          tileBuilder: dark ? darkModeTileBuilder : null,
        ),
        if (polygons.isNotEmpty)
          PolygonLayer(polygons: polygons, simplificationTolerance: 0.6),
        ...overlays,
        if (density)
          CircleLayer(
            circles: [
              for (final o in visible)
                CircleMarker(
                  point: LatLng(o.lat, o.lon),
                  radius: 22,
                  color: eco.categoryColor(o.category).withValues(alpha: 0.16),
                ),
            ],
          )
        else
          MarkerClusterLayerWidget(
            options: MarkerClusterLayerOptions(
              markers: _markers,
              maxClusterRadius: 56,
              size: const Size(40, 40),
              disableClusteringAtZoom: 16,
              markerChildBehavior: true,
              showPolygon: false,
              spiderfyCluster: true,
              builder: (context, markers) => ClusterBubble(count: markers.length),
            ),
          ),
        const _MyLocationMarker(),
        const MapControls(),
        SimpleAttributionWidget(
          source: Text(
            s.osmAttribution,
            style: TextStyle(fontSize: 11, color: eco.isDark && dark ? Colors.white : Colors.black87),
          ),
          backgroundColor:
              (dark ? Colors.black : Colors.white).withValues(alpha: 0.55),
          onTap: () => launchUrl(
            Uri.parse('https://www.openstreetmap.org/copyright'),
            mode: LaunchMode.externalApplication,
          ),
        ),
      ],
    );
  }

  // Exposed for tests that tap a marker key.
  Observation? observationForKey(Key key) => _byKey[key];
}

/// Position of the user, set by the my-location button.
final myLocationProvider = NotifierProvider<_MyLocation, LatLng?>(_MyLocation.new);

class _MyLocation extends Notifier<LatLng?> {
  @override
  LatLng? build() => null;
  void set(LatLng? p) => state = p;
}

class _MyLocationMarker extends ConsumerWidget {
  const _MyLocationMarker();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final p = ref.watch(myLocationProvider);
    if (p == null) return const SizedBox.shrink();
    final eco = context.eco;
    return MarkerLayer(
      markers: [
        Marker(
          point: p,
          width: 22,
          height: 22,
          child: Container(
            decoration: BoxDecoration(
              color: eco.info,
              shape: BoxShape.circle,
              border: Border.all(color: Colors.white, width: 3),
              boxShadow: const [BoxShadow(color: Color(0x66000000), blurRadius: 6)],
            ),
          ),
        ),
      ],
    );
  }
}
