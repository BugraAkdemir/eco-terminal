import '../../models/models.dart';
import '../api_client.dart';

/// `/feeds*` (public) and `/admin/feeds*` (admin).
class FeedsApi {
  const FeedsApi(this._c);
  final ApiClient _c;

  Future<List<FeedSource>> sources() async =>
      (await _c.get('/feeds')).list.map(FeedSource.fromJson).toList();

  Future<Paged<FeedArticle>> articles({
    String? regionId,
    String? sourceId,
    String? category,
    DateTime? from,
    DateTime? to,
    int? page,
    int? pageSize,
  }) async {
    final env = await _c.get('/feeds/articles', query: {
      'region_id': regionId,
      'source_id': sourceId,
      'category': category,
      'from': from?.toUtc().toIso8601String(),
      'to': to?.toUtc().toIso8601String(),
      'page': page,
      'page_size': pageSize,
    });
    return Paged(
      items: env.list.map(FeedArticle.fromJson).toList(),
      meta: env.meta ?? const PageMeta(),
    );
  }

  /// Includes disabled sources (admin).
  Future<List<FeedSource>> adminSources() async =>
      (await _c.get('/admin/feeds')).list.map(FeedSource.fromJson).toList();

  Future<FeedSource> addSource(FeedSourceCreate req) async =>
      FeedSource.fromJson((await _c.post('/admin/feeds', body: req.toJson())).object);

  /// Triggers an async sync; returns the job id.
  Future<String> sync({String? sourceId}) async {
    final env = await _c.post(
      '/admin/feeds/sync',
      body: sourceId == null ? <String, Object?>{} : {'source_id': sourceId},
    );
    return '${env.object['job_id'] ?? ''}';
  }
}
