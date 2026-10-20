import '../../models/models.dart';
import '../api_client.dart';

/// `/regions*`.
class RegionsApi {
  const RegionsApi(this._c);
  final ApiClient _c;

  Future<List<Region>> list({
    RegionKind? kind,
    String? parentId,
    bool geometry = false,
    String? q,
    double? lon,
    double? lat,
  }) async {
    final env = await _c.get('/regions', query: {
      'kind': kind?.name,
      'parent_id': parentId,
      'geometry': geometry ? true : null,
      'q': q,
      'lon': lon,
      'lat': lat,
    });
    return env.list.map(Region.fromJson).toList();
  }

  Future<Region> get(String id) async =>
      Region.fromJson((await _c.get('/regions/$id')).object);

  Future<Paged<Observation>> observations(
    String regionId, {
    String? category,
    DateTime? from,
    DateTime? to,
    int? page,
    int? pageSize,
  }) async {
    final env = await _c.get('/regions/$regionId/observations', query: {
      'category': category,
      'from': from?.toUtc().toIso8601String(),
      'to': to?.toUtc().toIso8601String(),
      'page': page,
      'page_size': pageSize,
    });
    return Paged(
      items: env.list.map(Observation.fromJson).toList(),
      meta: env.meta ?? const PageMeta(),
    );
  }

  Future<RegionAnalytics> analytics(
    String regionId, {
    DateTime? from,
    DateTime? to,
  }) async {
    final env = await _c.get('/regions/$regionId/analytics', query: {
      'from': from?.toUtc().toIso8601String(),
      'to': to?.toUtc().toIso8601String(),
    });
    return RegionAnalytics.fromJson(env.object);
  }

  /// Custom analysis region from a GeoJSON polygon (researcher/admin).
  Future<Region> create(RegionCreate req) async =>
      Region.fromJson((await _c.post('/regions', body: req.toJson())).object);
}
