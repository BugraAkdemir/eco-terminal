import '../../models/models.dart';
import '../api_client.dart';

/// `/admin/users*`, `/admin/audit-logs` (admin). Feed admin lives in FeedsApi.
class AdminApi {
  const AdminApi(this._c);
  final ApiClient _c;

  Future<Paged<User>> users({int? page, int? pageSize}) async {
    final env =
        await _c.get('/admin/users', query: {'page': page, 'page_size': pageSize});
    return Paged(
      items: env.list.map(User.fromJson).toList(),
      meta: env.meta ?? const PageMeta(),
    );
  }

  Future<User> updateUser(String id, AdminUserUpdate update) async =>
      User.fromJson(
          (await _c.patch('/admin/users/$id', body: update.toJson())).object);

  Future<Paged<AuditLog>> auditLogs({
    String? entityType,
    String? entityId,
    String? actorId,
    int? page,
    int? pageSize,
  }) async {
    final env = await _c.get('/admin/audit-logs', query: {
      'entity_type': entityType,
      'entity_id': entityId,
      'actor_id': actorId,
      'page': page,
      'page_size': pageSize,
    });
    return Paged(
      items: env.list.map(AuditLog.fromJson).toList(),
      meta: env.meta ?? const PageMeta(),
    );
  }
}
