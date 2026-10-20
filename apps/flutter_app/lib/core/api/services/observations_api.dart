import '../../models/models.dart';
import '../api_client.dart';

/// Filters shared by GET /observations and the region/review listings.
class ObservationQuery {
  const ObservationQuery({
    this.bbox,
    this.regionId,
    this.category,
    this.recordType,
    this.status,
    this.from,
    this.to,
    this.mine,
    this.q,
    this.page,
    this.pageSize,
  });

  /// `[min_lon, min_lat, max_lon, max_lat]`
  final List<double>? bbox;
  final String? regionId;
  final String? category;
  final RecordType? recordType;
  final String? status;
  final DateTime? from;
  final DateTime? to;
  final bool? mine;
  final String? q;
  final int? page;
  final int? pageSize;

  Map<String, Object?> toQuery() => {
        'bbox': bbox?.map((e) => e.toStringAsFixed(5)).join(','),
        'region_id': regionId,
        'category': category,
        'record_type': recordType?.wire,
        'status': status,
        'from': from?.toUtc().toIso8601String(),
        'to': to?.toUtc().toIso8601String(),
        'mine': mine,
        'q': q,
        'page': page,
        'page_size': pageSize,
      };
}

/// `/observations`, `/observations/{id}`.
class ObservationsApi {
  const ObservationsApi(this._c);
  final ApiClient _c;

  Future<Paged<Observation>> list(ObservationQuery query) async {
    final env = await _c.get('/observations', query: query.toQuery());
    return Paged(
      items: env.list.map(Observation.fromJson).toList(),
      meta: env.meta ?? const PageMeta(),
    );
  }

  Future<ObservationDetail> get(String id) async =>
      ObservationDetail.fromJson((await _c.get('/observations/$id')).object);

  Future<Observation> create(ObservationCreate req) async =>
      Observation.fromJson((await _c.post('/observations', body: req.toJson())).object);

  Future<Observation> patch(String id, ObservationPatch req) async =>
      Observation.fromJson(
          (await _c.patch('/observations/$id', body: req.toJson())).object);

  /// Soft delete of an own record.
  Future<void> delete(String id) async {
    await _c.delete('/observations/$id');
  }
}
