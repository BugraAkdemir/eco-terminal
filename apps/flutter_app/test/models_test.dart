import 'package:eco_terminal/core/models/models.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  final observationJson = <String, dynamic>{
    'id': 'o1',
    'reference_no': 'ET-2026-000123',
    'user_id': 'u1',
    'record_type': 'observation',
    'category': 'injured_wildlife',
    'title': 'Yaralı karaca',
    'description': 'Yolun kenarında',
    'species_guess': 'karaca',
    'species_verified': '',
    'count_observed': 1,
    'details': {'alive': true},
    'lat': 41.4,
    'lon': 33.8,
    'exact_location': {'lat': 41.4012, 'lon': 33.8013},
    'location_generalized': true,
    'location_source': 'gps',
    'accuracy_m': 12,
    'observed_at': '2026-05-01T10:00:00Z',
    'status': 'needs_information',
    'priority': 'high',
    'is_sensitive': true,
    'is_demo': true,
    'primary_region_id': 'r1',
    'primary_region_name': 'Kastamonu Merkez',
    'thumbnail_media_id': 'm1',
    'media_count': 2,
    'created_at': '2026-05-01T10:05:00Z',
    'updated_at': '2026-05-02T10:05:00Z',
  };

  test('Observation parses and round-trips', () {
    final o = Observation.fromJson(observationJson);
    expect(o.status, ObservationStatus.needsInformation);
    expect(o.category, ObservationCategory.injuredWildlife);
    expect(o.accuracyM, 12.0); // int in JSON -> double
    expect(o.exactLocation?.lon, 33.8013);
    expect(o.locationGeneralized, isTrue);
    final again = Observation.fromJson(o.toJson());
    expect(again.toJson(), o.toJson());
  });

  test('Observation tolerates missing/null fields and unknown enums', () {
    final o = Observation.fromJson({
      'id': 'x',
      'status': 'something_new',
      'category': 'weird',
      'lat': 1,
      'lon': 2,
      'count_observed': null,
      'exact_location': null,
    });
    expect(o.status, ObservationStatus.unknown);
    expect(o.category, ObservationCategory.other);
    expect(o.countObserved, isNull);
    expect(o.exactLocation, isNull);
    expect(o.lat, 1.0);
  });

  test('ObservationDetail parses nested lists', () {
    final d = ObservationDetail.fromJson({
      ...observationJson,
      'media': [
        {'id': 'm1', 'url': '/api/v1/media/m1', 'width': 800, 'height': 600},
      ],
      'ai_analysis': [
        {
          'id': 'a1',
          'status': 'done',
          'suggested_species': 'Capreolus capreolus',
          'confidence': 0.7,
          'disclaimer': 'doğrulanmadı',
        },
      ],
      'history': [
        {'id': 'h1', 'from_status': null, 'to_status': 'submitted', 'actor_name': 'Ayşe'},
      ],
      'reviews': [
        {'id': 'rv1', 'decision': 'request_info', 'note': 'Fotoğraf ekleyin'},
      ],
      'owner_display_name': 'Ayşe',
    });
    expect(d.media.single.url, '/api/v1/media/m1');
    expect(d.aiAnalysis.single.status, AiAnalysisStatus.done);
    expect(d.aiAnalysis.single.confidence, 0.7);
    expect(d.history.single.fromStatus, isNull);
    expect(d.history.single.toStatus, ObservationStatus.submitted);
    expect(d.reviews.single.decision, 'request_info');
    expect(d.id, 'o1');
    expect(ObservationDetail.fromJson(d.toJson()).toJson(), d.toJson());
  });

  test('User and AuthSession', () {
    final s = AuthSession.fromJson({
      'token': 't',
      'expires_at': '2026-12-01T00:00:00Z',
      'user': {
        'id': 'u1',
        'email': 'a@b.co',
        'role': 'reviewer',
        'display_name': 'A',
        'locale': 'tr',
        'location_consent': true,
      },
    });
    expect(s.user.role, UserRole.reviewer);
    expect(s.user.locationConsent, isTrue);
    expect(s.expiresAt?.year, 2026);
  });

  test('request bodies omit nulls and use contract field names', () {
    expect(
      const RegisterRequest(email: 'a@b.co', password: 'xxxxxxxxxx').toJson(),
      {'email': 'a@b.co', 'password': 'xxxxxxxxxx'},
    );
    expect(
      const ProfileUpdate(locationConsent: true).toJson(),
      {'consent_location': true},
    );
    final create = ObservationCreate(
      category: ObservationCategory.water,
      lat: 41.0,
      lon: 33.0,
      observedAt: DateTime.utc(2026, 5, 1),
      draft: true,
    ).toJson();
    expect(create['record_type'], 'observation');
    expect(create['category'], 'water');
    expect(create['draft'], true);
    expect(create.containsKey('title'), isFalse);
    expect(
      const ReviewDecision(decision: ReviewDecisionType.requestInfo, note: 'n')
          .toJson(),
      {'decision': 'request_info', 'note': 'n'},
    );
  });

  test('Region and RegionAnalytics', () {
    final r = Region.fromJson({
      'id': 'r1',
      'kind': 'district',
      'name': 'Merkez',
      'bbox': [33.0, 41.0, 34.0, 42],
      'centroid': {'lon': 33.5, 'lat': 41.5},
      'geometry': {'type': 'Polygon', 'coordinates': []},
      'is_demo': true,
    });
    expect(r.hasBbox, isTrue);
    expect(r.bbox.last, 42.0);
    expect(r.centroidLat, 41.5);
    expect(Region.fromJson(r.toJson()), r);

    final a = RegionAnalytics.fromJson({
      'region_id': 'r1',
      'period': {'from': '2025-01-01', 'to': '2026-01-01'},
      'totals': {'observations': 10, 'issues': 2, 'verified': 4, 'pending_review': 3},
      'by_category': [
        {'category': 'animal', 'count': 6},
        {'category': 'plant', 'count': 4},
      ],
      'by_month': [
        {'month': '2025-05', 'count': 3, 'distinct_observers': 2},
      ],
      'top_species': [
        {'species': 'karaca', 'count': 2},
      ],
      'environmental': null,
      'data_quality': {
        'distinct_observers': 2,
        'months_with_data': 1,
        'months_in_period': 12,
        'warnings': ['Az sayıda gözlemci'],
      },
    });
    expect(a.totals.pendingReview, 3);
    expect(a.byCategory.first.category, 'animal');
    expect(a.byMonth.single.distinctObservers, 2);
    expect(a.environmental, isNull);
    expect(a.dataQuality.warnings.single, 'Az sayıda gözlemci');
    expect(RegionAnalytics.fromJson(a.toJson()).toJson(), a.toJson());
  });

  test('ChatAnswer with sources and facts', () {
    final c = ChatAnswer.fromJson({
      'answer': 'Bölgede 10 gözlem var [1].',
      'mode': 'retrieval_only',
      'region': {'id': 'r1', 'name': 'Merkez'},
      'period': {'from': '2025-01-01', 'to': '2026-01-01'},
      'facts': [
        {'label': 'Gözlem', 'value': '10', 'query_note': 'COUNT(*)'},
      ],
      'sources': [
        {
          'id': '[1]',
          'type': 'observation',
          'title': 'Karaca',
          'url': null,
          'entity_id': 'o1',
          'status': 'verified',
        },
      ],
      'limitations': ['Örneklem sistematik değil'],
      'needs_region': false,
    });
    expect(c.mode, ChatMode.retrievalOnly);
    expect(c.sources.single.entityId, 'o1');
    expect(c.facts.single.queryNote, 'COUNT(*)');
    expect(ChatAnswer.fromJson(c.toJson()).toJson(), c.toJson());
  });

  test('SimulationResult, Notification, Feed, AuditLog, PageMeta', () {
    final sim = SimulationResult.fromJson({
      'id': 's1',
      'status': 'insufficient_data',
      'indicators': [
        {'key': 'spi', 'label': 'SPI', 'unit': '', 'reference': 0, 'scenario': -0.4, 'change': -0.4},
      ],
      'series': [
        {
          'key': 'precip',
          'points': [
            {'month': '2020-01', 'reference': 50, 'scenario': 40},
          ],
        },
      ],
      'data_sources': [
        {'dataset': 'ERA5', 'source': 'x', 'license': 'CC-BY', 'coverage': 0.9},
      ],
      'assumptions': ['a'],
      'limitations': ['b'],
      'insufficient_reason': 'Az veri',
      'narrative': null,
    });
    expect(sim.isOk, isFalse);
    expect(sim.indicators.single.scenario, -0.4);
    expect(sim.series.single.points.single.reference, 50.0);
    expect(SimulationResult.fromJson(sim.toJson()).toJson(), sim.toJson());

    final n = AppNotification.fromJson({
      'id': 'n1',
      'kind': 'status',
      'title': 't',
      'body': 'b',
      'observation_id': 'o1',
      'read_at': null,
      'created_at': '2026-01-01T00:00:00Z',
    });
    expect(n.isRead, isFalse);
    expect(n.markedRead().isRead, isTrue);

    final art = FeedArticle.fromJson({
      'id': 'f1',
      'title': 'Duyuru',
      'url': 'https://example.org',
      'published_at': null,
      'regions': [
        {'id': 'r1', 'name': 'Merkez'},
      ],
    });
    expect(art.regions.single.name, 'Merkez');
    expect(FeedSource.fromJson({'id': 's', 'enabled': false}).enabled, isFalse);

    expect(
      AuditLog.fromJson({'id': 'l', 'action': 'verify', 'before': null, 'after': {'status': 'verified'}}).after,
      {'status': 'verified'},
    );
    final meta = PageMeta.fromJson({'page': 1, 'page_size': 50, 'total': 120});
    expect(meta.hasMore, isTrue);
  });

  test('EnvironmentSeries, HealthStatus, AiProvidersStatus', () {
    final e = EnvironmentSeries.fromJson({
      'dataset': 'chirps',
      'unit': 'mm',
      'coverage': 1,
      'points': [
        {'ts': '2020-01', 'value': 12},
      ],
    });
    expect(e.coverage, 1.0);
    expect(e.points.single.value, 12.0);
    expect(
      HealthStatus.fromJson({'status': 'ok', 'ai_configured': true}).aiConfigured,
      isTrue,
    );
    expect(
      AiProvidersStatus.fromJson({
        'vision': {'configured': true, 'provider': 'x', 'model': 'm'},
      }).vision.configured,
      isTrue,
    );
  });
}
