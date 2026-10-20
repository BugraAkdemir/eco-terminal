import 'package:latlong2/latlong.dart';

import '../../../core/models/models.dart';

/// One polygon: outer ring plus optional holes.
class GeoPolygon {
  const GeoPolygon({required this.outer, this.holes = const []});
  final List<LatLng> outer;
  final List<List<LatLng>> holes;

  bool contains(LatLng p) {
    if (!pointInRing(p, outer)) return false;
    for (final h in holes) {
      if (pointInRing(p, h)) return false;
    }
    return true;
  }
}

/// Ray-casting point-in-ring test (ring may or may not be closed).
bool pointInRing(LatLng p, List<LatLng> ring) {
  var inside = false;
  final n = ring.length;
  if (n < 3) return false;
  for (var i = 0, j = n - 1; i < n; j = i++) {
    final a = ring[i];
    final b = ring[j];
    final intersects = (a.latitude > p.latitude) != (b.latitude > p.latitude) &&
        p.longitude <
            (b.longitude - a.longitude) *
                    (p.latitude - a.latitude) /
                    (b.latitude - a.latitude) +
                a.longitude;
    if (intersects) inside = !inside;
  }
  return inside;
}

List<LatLng> _ring(Object? raw) {
  if (raw is! List) return const [];
  final out = <LatLng>[];
  for (final c in raw) {
    if (c is List && c.length >= 2 && c[0] is num && c[1] is num) {
      // GeoJSON is [lon, lat].
      out.add(LatLng((c[1] as num).toDouble(), (c[0] as num).toDouble()));
    }
  }
  return out;
}

List<GeoPolygon> _polygon(Object? coords) {
  if (coords is! List || coords.isEmpty) return const [];
  final rings = coords.map(_ring).where((r) => r.length >= 3).toList();
  if (rings.isEmpty) return const [];
  return [GeoPolygon(outer: rings.first, holes: rings.sublist(1))];
}

/// Parses a GeoJSON Polygon / MultiPolygon geometry (other types -> empty).
List<GeoPolygon> parseGeometry(Map<String, dynamic>? geometry) {
  if (geometry == null) return const [];
  final type = geometry['type'];
  final coords = geometry['coordinates'];
  if (type == 'Polygon') return _polygon(coords);
  if (type == 'MultiPolygon' && coords is List) {
    return [for (final p in coords) ..._polygon(p)];
  }
  return const [];
}

/// A region with parsed geometry and a cached bbox (for cheap hit-testing).
class RegionShape {
  RegionShape(this.region, this.polygons)
      : bbox = _bboxOf(region, polygons);

  final Region region;
  final List<GeoPolygon> polygons;

  /// `[minLon, minLat, maxLon, maxLat]`.
  final List<double> bbox;

  double get bboxArea => (bbox[2] - bbox[0]) * (bbox[3] - bbox[1]);

  LatLng get center => LatLng((bbox[1] + bbox[3]) / 2, (bbox[0] + bbox[2]) / 2);

  bool contains(LatLng p) {
    if (p.longitude < bbox[0] ||
        p.longitude > bbox[2] ||
        p.latitude < bbox[1] ||
        p.latitude > bbox[3]) {
      return false;
    }
    return polygons.any((poly) => poly.contains(p));
  }

  static List<double> _bboxOf(Region r, List<GeoPolygon> polys) {
    if (r.hasBbox) return r.bbox;
    var minLon = 180.0, minLat = 90.0, maxLon = -180.0, maxLat = -90.0;
    for (final poly in polys) {
      for (final p in poly.outer) {
        if (p.longitude < minLon) minLon = p.longitude;
        if (p.longitude > maxLon) maxLon = p.longitude;
        if (p.latitude < minLat) minLat = p.latitude;
        if (p.latitude > maxLat) maxLat = p.latitude;
      }
    }
    return [minLon, minLat, maxLon, maxLat];
  }
}

List<RegionShape> shapesFromRegions(List<Region> regions) => [
      for (final r in regions)
        if (parseGeometry(r.geometry).isNotEmpty)
          RegionShape(r, parseGeometry(r.geometry)),
    ];

/// Smallest (finest) region containing [p] among those accepted by [allowed].
RegionShape? pickRegionAt(
  LatLng p,
  Iterable<RegionShape> shapes, {
  bool Function(Region r)? allowed,
}) {
  RegionShape? best;
  for (final s in shapes) {
    if (allowed != null && !allowed(s.region)) continue;
    if (!s.contains(p)) continue;
    if (best == null || s.bboxArea < best.bboxArea) best = s;
  }
  return best;
}
