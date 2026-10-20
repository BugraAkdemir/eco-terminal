import 'json.dart';

class PageMeta {
  const PageMeta({this.page = 1, this.pageSize = 0, this.total = 0});
  final int page;
  final int pageSize;
  final int total;

  factory PageMeta.fromJson(Json j) => PageMeta(
        page: intOf(j['page'], 1),
        pageSize: intOf(j['page_size']),
        total: intOf(j['total']),
      );
  Json toJson() => {'page': page, 'page_size': pageSize, 'total': total};

  bool get hasMore => pageSize > 0 && page * pageSize < total;
}

/// A page of items with its [PageMeta].
class Paged<T> {
  const Paged({required this.items, required this.meta});
  final List<T> items;
  final PageMeta meta;

  factory Paged.empty() => Paged<T>(items: const [], meta: const PageMeta());
}

class AppNotification {
  const AppNotification({
    required this.id,
    this.kind = '',
    this.title = '',
    this.body = '',
    this.observationId,
    this.readAt,
    this.createdAt,
  });
  final String id;
  final String kind;
  final String title;
  final String body;
  final String? observationId;
  final String? readAt;
  final DateTime? createdAt;

  bool get isRead => readAt != null && readAt!.isNotEmpty;

  factory AppNotification.fromJson(Json j) => AppNotification(
        id: str(j['id']),
        kind: str(j['kind']),
        title: str(j['title']),
        body: str(j['body']),
        observationId: strOrNull(j['observation_id']),
        readAt: strOrNull(j['read_at']),
        createdAt: dateOrNull(j['created_at']),
      );

  Json toJson() => {
        'id': id,
        'kind': kind,
        'title': title,
        'body': body,
        'observation_id': observationId,
        'read_at': readAt,
        'created_at': isoOrNull(createdAt),
      };

  AppNotification markedRead() => AppNotification(
        id: id,
        kind: kind,
        title: title,
        body: body,
        observationId: observationId,
        readAt: readAt ?? DateTime.now().toUtc().toIso8601String(),
        createdAt: createdAt,
      );
}

class AuditLog {
  const AuditLog({
    required this.id,
    this.actorId,
    this.actorRole = '',
    this.action = '',
    this.entityType = '',
    this.entityId = '',
    this.before,
    this.after,
    this.createdAt,
  });
  final String id;
  final String? actorId;
  final String actorRole;
  final String action;
  final String entityType;
  final String entityId;
  final Map<String, dynamic>? before;
  final Map<String, dynamic>? after;
  final DateTime? createdAt;

  factory AuditLog.fromJson(Json j) => AuditLog(
        id: str(j['id']),
        actorId: strOrNull(j['actor_id']),
        actorRole: str(j['actor_role']),
        action: str(j['action']),
        entityType: str(j['entity_type']),
        entityId: str(j['entity_id']),
        before: asMapOrNull(j['before']),
        after: asMapOrNull(j['after']),
        createdAt: dateOrNull(j['created_at']),
      );

  Json toJson() => {
        'id': id,
        'actor_id': actorId,
        'actor_role': actorRole,
        'action': action,
        'entity_type': entityType,
        'entity_id': entityId,
        'before': before,
        'after': after,
        'created_at': isoOrNull(createdAt),
      };
}
