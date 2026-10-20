import '../../models/models.dart';
import '../api_client.dart';

/// `/reviews/*` (reviewer/admin).
class ReviewApi {
  const ReviewApi(this._c);
  final ApiClient _c;

  Future<Paged<ObservationDetail>> queue({
    String? status,
    String? category,
    String? recordType,
    String? priority,
    String? regionId,

    /// pending | done | failed | skipped
    String? aiStatus,
    DateTime? from,
    DateTime? to,
    int? page,
    int? pageSize,
  }) async {
    final env = await _c.get('/reviews/queue', query: {
      'status': status,
      'category': category,
      'record_type': recordType,
      'priority': priority,
      'region_id': regionId,
      'ai_status': aiStatus,
      'from': from?.toUtc().toIso8601String(),
      'to': to?.toUtc().toIso8601String(),
      'page': page,
      'page_size': pageSize,
    });
    return Paged(
      items: env.list.map(ObservationDetail.fromJson).toList(),
      meta: env.meta ?? const PageMeta(),
    );
  }

  Future<ObservationDetail> decide(
    String observationId,
    ReviewDecision decision,
  ) async =>
      ObservationDetail.fromJson((await _c.post(
        '/reviews/$observationId/decision',
        body: decision.toJson(),
      ))
          .object);
}
