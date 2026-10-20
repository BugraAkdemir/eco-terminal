import 'json.dart';

class FeedSource {
  const FeedSource({
    required this.id,
    this.name = '',
    this.organization = '',
    this.feedUrl = '',
    this.category = '',
    this.enabled = true,
    this.verifiedAt,
    this.lastFetchedAt,
    this.lastStatus = '',
    this.lastError = '',
  });

  final String id;
  final String name;
  final String organization;
  final String feedUrl;
  final String category;
  final bool enabled;
  final String? verifiedAt;
  final String? lastFetchedAt;
  final String lastStatus;
  final String lastError;

  factory FeedSource.fromJson(Json j) => FeedSource(
        id: str(j['id']),
        name: str(j['name']),
        organization: str(j['organization']),
        feedUrl: str(j['feed_url']),
        category: str(j['category']),
        enabled: boolOf(j['enabled'], true),
        verifiedAt: strOrNull(j['verified_at']),
        lastFetchedAt: strOrNull(j['last_fetched_at']),
        lastStatus: str(j['last_status']),
        lastError: str(j['last_error']),
      );

  Json toJson() => {
        'id': id,
        'name': name,
        'organization': organization,
        'feed_url': feedUrl,
        'category': category,
        'enabled': enabled,
        'verified_at': verifiedAt,
        'last_fetched_at': lastFetchedAt,
        'last_status': lastStatus,
        'last_error': lastError,
      };
}

class FeedArticleRegion {
  const FeedArticleRegion({required this.id, this.name = ''});
  final String id;
  final String name;

  factory FeedArticleRegion.fromJson(Json j) =>
      FeedArticleRegion(id: str(j['id']), name: str(j['name']));
  Json toJson() => {'id': id, 'name': name};
}

class FeedArticle {
  const FeedArticle({
    required this.id,
    this.sourceId = '',
    this.sourceName = '',
    this.title = '',
    this.summary = '',
    this.url = '',
    this.category = '',
    this.publishedAt,
    this.ingestedAt = '',
    this.regions = const [],
    this.locationNote = '',
  });

  final String id;
  final String sourceId;
  final String sourceName;
  final String title;
  final String summary;
  final String url;
  final String category;
  final String? publishedAt;
  final String ingestedAt;
  final List<FeedArticleRegion> regions;
  final String locationNote;

  factory FeedArticle.fromJson(Json j) => FeedArticle(
        id: str(j['id']),
        sourceId: str(j['source_id']),
        sourceName: str(j['source_name']),
        title: str(j['title']),
        summary: str(j['summary']),
        url: str(j['url']),
        category: str(j['category']),
        publishedAt: strOrNull(j['published_at']),
        ingestedAt: str(j['ingested_at']),
        regions: listOf(j['regions'], FeedArticleRegion.fromJson),
        locationNote: str(j['location_note']),
      );

  Json toJson() => {
        'id': id,
        'source_id': sourceId,
        'source_name': sourceName,
        'title': title,
        'summary': summary,
        'url': url,
        'category': category,
        'published_at': publishedAt,
        'ingested_at': ingestedAt,
        'regions': regions.map((e) => e.toJson()).toList(),
        'location_note': locationNote,
      };
}

/// Body of POST /admin/feeds.
class FeedSourceCreate {
  const FeedSourceCreate({
    required this.name,
    required this.feedUrl,
    this.organization,
    this.category,
    this.intervalMinutes,
  });
  final String name;
  final String feedUrl;
  final String? organization;
  final String? category;
  final int? intervalMinutes;

  factory FeedSourceCreate.fromJson(Json j) => FeedSourceCreate(
        name: str(j['name']),
        feedUrl: str(j['feed_url']),
        organization: strOrNull(j['organization']),
        category: strOrNull(j['category']),
        intervalMinutes: intOrNull(j['interval_minutes']),
      );

  Json toJson() => compact({
        'name': name,
        'feed_url': feedUrl,
        'organization': organization,
        'category': category,
        'interval_minutes': intervalMinutes,
      });
}
