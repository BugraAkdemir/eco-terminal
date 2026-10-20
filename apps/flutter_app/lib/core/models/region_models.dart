import 'enums.dart';
import 'json.dart';

class Region {
  const Region({
    required this.id,
    this.kind = RegionKind.unknown,
    this.name = '',
    this.parentId,
    this.bbox = const [],
    this.centroidLat,
    this.centroidLon,
    this.geometry,
    this.source = '',
    this.isDemo = false,
  });

  final String id;
  final RegionKind kind;
  final String name;
  final String? parentId;

  /// `[min_lon, min_lat, max_lon, max_lat]` or empty.
  final List<double> bbox;
  final double? centroidLat;
  final double? centroidLon;

  /// GeoJSON geometry (Polygon / MultiPolygon); null unless requested.
  final Map<String, dynamic>? geometry;
  final String source;
  final bool isDemo;

  bool get hasBbox => bbox.length == 4;

  factory Region.fromJson(Json j) {
    final centroid = asMapOrNull(j['centroid']);
    return Region(
      id: str(j['id']),
      kind: RegionKind.fromWire(strOrNull(j['kind'])),
      name: str(j['name']),
      parentId: strOrNull(j['parent_id']),
      bbox: doubleList(j['bbox']),
      centroidLat: centroid == null ? null : doubleOrNull(centroid['lat']),
      centroidLon: centroid == null ? null : doubleOrNull(centroid['lon']),
      geometry: asMapOrNull(j['geometry']),
      source: str(j['source']),
      isDemo: boolOf(j['is_demo']),
    );
  }

  Json toJson() => {
        'id': id,
        'kind': kind.name,
        'name': name,
        'parent_id': parentId,
        'bbox': bbox,
        'centroid': centroidLat == null || centroidLon == null
            ? null
            : {'lon': centroidLon, 'lat': centroidLat},
        'geometry': geometry,
        'source': source,
        'is_demo': isDemo,
      };

  @override
  bool operator ==(Object other) => other is Region && other.id == id;

  @override
  int get hashCode => id.hashCode;
}

class AnalyticsPeriod {
  const AnalyticsPeriod({this.from = '', this.to = ''});
  final String from;
  final String to;

  factory AnalyticsPeriod.fromJson(Json j) =>
      AnalyticsPeriod(from: str(j['from']), to: str(j['to']));
  Json toJson() => {'from': from, 'to': to};
}

class AnalyticsTotals {
  const AnalyticsTotals({
    this.observations = 0,
    this.issues = 0,
    this.verified = 0,
    this.pendingReview = 0,
  });
  final int observations;
  final int issues;
  final int verified;
  final int pendingReview;

  factory AnalyticsTotals.fromJson(Json j) => AnalyticsTotals(
        observations: intOf(j['observations']),
        issues: intOf(j['issues']),
        verified: intOf(j['verified']),
        pendingReview: intOf(j['pending_review']),
      );
  Json toJson() => {
        'observations': observations,
        'issues': issues,
        'verified': verified,
        'pending_review': pendingReview,
      };
}

class CategoryCount {
  const CategoryCount({required this.category, this.count = 0});
  final String category;
  final int count;

  factory CategoryCount.fromJson(Json j) =>
      CategoryCount(category: str(j['category']), count: intOf(j['count']));
  Json toJson() => {'category': category, 'count': count};
}

class MonthCount {
  const MonthCount({
    required this.month,
    this.count = 0,
    this.distinctObservers = 0,
  });
  final String month;
  final int count;
  final int distinctObservers;

  factory MonthCount.fromJson(Json j) => MonthCount(
        month: str(j['month']),
        count: intOf(j['count']),
        distinctObservers: intOf(j['distinct_observers']),
      );
  Json toJson() => {
        'month': month,
        'count': count,
        'distinct_observers': distinctObservers,
      };
}

class SpeciesCount {
  const SpeciesCount({required this.species, this.count = 0});
  final String species;
  final int count;

  factory SpeciesCount.fromJson(Json j) =>
      SpeciesCount(species: str(j['species']), count: intOf(j['count']));
  Json toJson() => {'species': species, 'count': count};
}

class DataQuality {
  const DataQuality({
    this.distinctObservers = 0,
    this.monthsWithData = 0,
    this.monthsInPeriod = 0,
    this.warnings = const [],
  });
  final int distinctObservers;
  final int monthsWithData;
  final int monthsInPeriod;

  /// e.g. sampling-bias / sparse-data notes (Turkish, from the server).
  final List<String> warnings;

  factory DataQuality.fromJson(Json j) => DataQuality(
        distinctObservers: intOf(j['distinct_observers']),
        monthsWithData: intOf(j['months_with_data']),
        monthsInPeriod: intOf(j['months_in_period']),
        warnings: stringList(j['warnings']),
      );
  Json toJson() => {
        'distinct_observers': distinctObservers,
        'months_with_data': monthsWithData,
        'months_in_period': monthsInPeriod,
        'warnings': warnings,
      };
}

class RegionAnalytics {
  const RegionAnalytics({
    required this.regionId,
    this.period = const AnalyticsPeriod(),
    this.totals = const AnalyticsTotals(),
    this.byCategory = const [],
    this.byMonth = const [],
    this.topSpecies = const [],
    this.environmental,
    this.dataQuality = const DataQuality(),
  });

  final String regionId;
  final AnalyticsPeriod period;
  final AnalyticsTotals totals;
  final List<CategoryCount> byCategory;
  final List<MonthCount> byMonth;
  final List<SpeciesCount> topSpecies;

  /// Precip/temperature summaries when data are present (free-form).
  final Map<String, dynamic>? environmental;
  final DataQuality dataQuality;

  factory RegionAnalytics.fromJson(Json j) => RegionAnalytics(
        regionId: str(j['region_id']),
        period: AnalyticsPeriod.fromJson(asMap(j['period'])),
        totals: AnalyticsTotals.fromJson(asMap(j['totals'])),
        byCategory: listOf(j['by_category'], CategoryCount.fromJson),
        byMonth: listOf(j['by_month'], MonthCount.fromJson),
        topSpecies: listOf(j['top_species'], SpeciesCount.fromJson),
        environmental: asMapOrNull(j['environmental']),
        dataQuality: DataQuality.fromJson(asMap(j['data_quality'])),
      );

  Json toJson() => {
        'region_id': regionId,
        'period': period.toJson(),
        'totals': totals.toJson(),
        'by_category': byCategory.map((e) => e.toJson()).toList(),
        'by_month': byMonth.map((e) => e.toJson()).toList(),
        'top_species': topSpecies.map((e) => e.toJson()).toList(),
        'environmental': environmental,
        'data_quality': dataQuality.toJson(),
      };
}

/// Body of POST /regions (researcher/admin).
class RegionCreate {
  const RegionCreate({required this.name, required this.geometry});
  final String name;
  final Map<String, dynamic> geometry;

  factory RegionCreate.fromJson(Json j) =>
      RegionCreate(name: str(j['name']), geometry: asMap(j['geometry']));
  Json toJson() => {'name': name, 'geometry': geometry};
}
