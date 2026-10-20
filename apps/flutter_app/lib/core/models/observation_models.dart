import 'enums.dart';
import 'json.dart';

class ExactLocation {
  const ExactLocation({required this.lat, required this.lon});
  final double lat;
  final double lon;

  factory ExactLocation.fromJson(Json j) =>
      ExactLocation(lat: doubleOf(j['lat']), lon: doubleOf(j['lon']));
  Json toJson() => {'lat': lat, 'lon': lon};
}

class Observation {
  const Observation({
    required this.id,
    this.referenceNo = '',
    this.userId = '',
    this.recordType = RecordType.observation,
    this.category = ObservationCategory.other,
    this.title = '',
    this.description = '',
    this.speciesGuess = '',
    this.speciesVerified = '',
    this.countObserved,
    this.details = const {},
    required this.lat,
    required this.lon,
    this.exactLocation,
    this.locationGeneralized = false,
    this.locationSource = '',
    this.accuracyM,
    required this.observedAt,
    this.status = ObservationStatus.submitted,
    this.priority = 'normal',
    this.isSensitive = false,
    this.isDemo = false,
    this.primaryRegionId,
    this.primaryRegionName,
    this.thumbnailMediaId,
    this.mediaCount = 0,
    this.createdAt,
    this.updatedAt,
  });

  final String id;
  final String referenceNo;
  final String userId;
  final RecordType recordType;
  final ObservationCategory category;
  final String title;
  final String description;

  /// The user's own, unverified guess (UI label: "Kullanıcı tahmini").
  final String speciesGuess;

  /// Set by a reviewer only.
  final String speciesVerified;
  final int? countObserved;
  final Map<String, dynamic> details;

  /// Public coordinate (generalized when [isSensitive] and caller is not staff/owner).
  final double lat;
  final double lon;
  final ExactLocation? exactLocation;
  final bool locationGeneralized;
  final String locationSource;
  final double? accuracyM;
  final DateTime observedAt;
  final ObservationStatus status;
  final String priority;
  final bool isSensitive;
  final bool isDemo;
  final String? primaryRegionId;
  final String? primaryRegionName;
  final String? thumbnailMediaId;
  final int mediaCount;
  final DateTime? createdAt;
  final DateTime? updatedAt;

  /// Best human title: explicit title, else verified species, else guess.
  String get displayTitle {
    if (title.isNotEmpty) return title;
    if (speciesVerified.isNotEmpty) return speciesVerified;
    if (speciesGuess.isNotEmpty) return speciesGuess;
    return referenceNo;
  }

  factory Observation.fromJson(Json j) => Observation(
        id: str(j['id']),
        referenceNo: str(j['reference_no']),
        userId: str(j['user_id']),
        recordType: RecordType.fromWire(strOrNull(j['record_type'])),
        category: ObservationCategory.fromWire(strOrNull(j['category'])),
        title: str(j['title']),
        description: str(j['description']),
        speciesGuess: str(j['species_guess']),
        speciesVerified: str(j['species_verified']),
        countObserved: intOrNull(j['count_observed']),
        details: asMap(j['details']),
        lat: doubleOf(j['lat']),
        lon: doubleOf(j['lon']),
        exactLocation: asMapOrNull(j['exact_location']) == null
            ? null
            : ExactLocation.fromJson(asMap(j['exact_location'])),
        locationGeneralized: boolOf(j['location_generalized']),
        locationSource: str(j['location_source']),
        accuracyM: doubleOrNull(j['accuracy_m']),
        observedAt: dateOf(j['observed_at']),
        status: ObservationStatus.fromWire(strOrNull(j['status'])),
        priority: str(j['priority'], 'normal'),
        isSensitive: boolOf(j['is_sensitive']),
        isDemo: boolOf(j['is_demo']),
        primaryRegionId: strOrNull(j['primary_region_id']),
        primaryRegionName: strOrNull(j['primary_region_name']),
        thumbnailMediaId: strOrNull(j['thumbnail_media_id']),
        mediaCount: intOf(j['media_count']),
        createdAt: dateOrNull(j['created_at']),
        updatedAt: dateOrNull(j['updated_at']),
      );

  Json toJson() => {
        'id': id,
        'reference_no': referenceNo,
        'user_id': userId,
        'record_type': recordType.wire,
        'category': category.wire,
        'title': title,
        'description': description,
        'species_guess': speciesGuess,
        'species_verified': speciesVerified,
        'count_observed': countObserved,
        'details': details,
        'lat': lat,
        'lon': lon,
        'exact_location': exactLocation?.toJson(),
        'location_generalized': locationGeneralized,
        'location_source': locationSource,
        'accuracy_m': accuracyM,
        'observed_at': isoOrNull(observedAt),
        'status': status.wire,
        'priority': priority,
        'is_sensitive': isSensitive,
        'is_demo': isDemo,
        'primary_region_id': primaryRegionId,
        'primary_region_name': primaryRegionName,
        'thumbnail_media_id': thumbnailMediaId,
        'media_count': mediaCount,
        'created_at': isoOrNull(createdAt),
        'updated_at': isoOrNull(updatedAt),
      };
}

class Media {
  const Media({
    required this.id,
    this.url = '',
    this.thumbUrl = '',
    this.mimeType = '',
    this.width = 0,
    this.height = 0,
    this.sizeBytes = 0,
  });

  final String id;

  /// Relative URL `/api/v1/media/{id}`; resolve with `AppConfig.resolveUrl`.
  final String url;
  final String thumbUrl;
  final String mimeType;
  final int width;
  final int height;
  final int sizeBytes;

  factory Media.fromJson(Json j) => Media(
        id: str(j['id']),
        url: str(j['url']),
        thumbUrl: str(j['thumb_url']),
        mimeType: str(j['mime_type']),
        width: intOf(j['width']),
        height: intOf(j['height']),
        sizeBytes: intOf(j['size_bytes']),
      );

  Json toJson() => {
        'id': id,
        'url': url,
        'thumb_url': thumbUrl,
        'mime_type': mimeType,
        'width': width,
        'height': height,
        'size_bytes': sizeBytes,
      };
}

class AIAnalysis {
  const AIAnalysis({
    required this.id,
    this.status = AiAnalysisStatus.pending,
    this.provider = '',
    this.model = '',
    this.suggestedCategory,
    this.suggestedSpecies,
    this.suggestedDescription,
    this.confidence,
    this.disclaimer = '',
    this.error,
    this.createdAt,
  });

  final String id;
  final AiAnalysisStatus status;
  final String provider;
  final String model;
  final String? suggestedCategory;
  final String? suggestedSpecies;
  final String? suggestedDescription;
  final double? confidence;

  /// Always states that this is an unverified AI suggestion.
  final String disclaimer;
  final String? error;
  final DateTime? createdAt;

  factory AIAnalysis.fromJson(Json j) => AIAnalysis(
        id: str(j['id']),
        status: AiAnalysisStatus.fromWire(strOrNull(j['status'])),
        provider: str(j['provider']),
        model: str(j['model']),
        suggestedCategory: strOrNull(j['suggested_category']),
        suggestedSpecies: strOrNull(j['suggested_species']),
        suggestedDescription: strOrNull(j['suggested_description']),
        confidence: doubleOrNull(j['confidence']),
        disclaimer: str(j['disclaimer']),
        error: strOrNull(j['error']),
        createdAt: dateOrNull(j['created_at']),
      );

  Json toJson() => {
        'id': id,
        'status': status.name,
        'provider': provider,
        'model': model,
        'suggested_category': suggestedCategory,
        'suggested_species': suggestedSpecies,
        'suggested_description': suggestedDescription,
        'confidence': confidence,
        'disclaimer': disclaimer,
        'error': error,
        'created_at': isoOrNull(createdAt),
      };
}

class StatusHistoryEntry {
  const StatusHistoryEntry({
    required this.id,
    this.actorId,
    this.actorType = '',
    this.actorName = '',
    this.fromStatus,
    required this.toStatus,
    this.note = '',
    this.createdAt,
  });

  final String id;
  final String? actorId;
  final String actorType;
  final String actorName;
  final ObservationStatus? fromStatus;
  final ObservationStatus toStatus;
  final String note;
  final DateTime? createdAt;

  factory StatusHistoryEntry.fromJson(Json j) => StatusHistoryEntry(
        id: str(j['id']),
        actorId: strOrNull(j['actor_id']),
        actorType: str(j['actor_type']),
        actorName: str(j['actor_name']),
        fromStatus: j['from_status'] == null
            ? null
            : ObservationStatus.fromWire(str(j['from_status'])),
        toStatus: ObservationStatus.fromWire(strOrNull(j['to_status'])),
        note: str(j['note']),
        createdAt: dateOrNull(j['created_at']),
      );

  Json toJson() => {
        'id': id,
        'actor_id': actorId,
        'actor_type': actorType,
        'actor_name': actorName,
        'from_status': fromStatus?.wire,
        'to_status': toStatus.wire,
        'note': note,
        'created_at': isoOrNull(createdAt),
      };
}

class Review {
  const Review({
    required this.id,
    this.reviewerId = '',
    this.decision = '',
    this.correctedCategory,
    this.correctedSpecies,
    this.note = '',
    this.createdAt,
  });

  final String id;
  final String reviewerId;
  final String decision;
  final String? correctedCategory;
  final String? correctedSpecies;
  final String note;
  final DateTime? createdAt;

  factory Review.fromJson(Json j) => Review(
        id: str(j['id']),
        reviewerId: str(j['reviewer_id']),
        decision: str(j['decision']),
        correctedCategory: strOrNull(j['corrected_category']),
        correctedSpecies: strOrNull(j['corrected_species']),
        note: str(j['note']),
        createdAt: dateOrNull(j['created_at']),
      );

  Json toJson() => {
        'id': id,
        'reviewer_id': reviewerId,
        'decision': decision,
        'corrected_category': correctedCategory,
        'corrected_species': correctedSpecies,
        'note': note,
        'created_at': isoOrNull(createdAt),
      };
}

/// Observation + media, AI suggestions, history and (staff only) reviews.
class ObservationDetail extends Observation {
  ObservationDetail({
    required Observation base,
    this.media = const [],
    this.aiAnalysis = const [],
    this.history = const [],
    this.reviews = const [],
    this.ownerDisplayName = '',
  }) : super(
          id: base.id,
          referenceNo: base.referenceNo,
          userId: base.userId,
          recordType: base.recordType,
          category: base.category,
          title: base.title,
          description: base.description,
          speciesGuess: base.speciesGuess,
          speciesVerified: base.speciesVerified,
          countObserved: base.countObserved,
          details: base.details,
          lat: base.lat,
          lon: base.lon,
          exactLocation: base.exactLocation,
          locationGeneralized: base.locationGeneralized,
          locationSource: base.locationSource,
          accuracyM: base.accuracyM,
          observedAt: base.observedAt,
          status: base.status,
          priority: base.priority,
          isSensitive: base.isSensitive,
          isDemo: base.isDemo,
          primaryRegionId: base.primaryRegionId,
          primaryRegionName: base.primaryRegionName,
          thumbnailMediaId: base.thumbnailMediaId,
          mediaCount: base.mediaCount,
          createdAt: base.createdAt,
          updatedAt: base.updatedAt,
        );

  final List<Media> media;
  final List<AIAnalysis> aiAnalysis;
  final List<StatusHistoryEntry> history;
  final List<Review> reviews;
  final String ownerDisplayName;

  factory ObservationDetail.fromJson(Json j) => ObservationDetail(
        base: Observation.fromJson(j),
        media: listOf(j['media'], Media.fromJson),
        aiAnalysis: listOf(j['ai_analysis'], AIAnalysis.fromJson),
        history: listOf(j['history'], StatusHistoryEntry.fromJson),
        reviews: listOf(j['reviews'], Review.fromJson),
        ownerDisplayName: str(j['owner_display_name']),
      );

  @override
  Json toJson() => {
        ...super.toJson(),
        'media': media.map((e) => e.toJson()).toList(),
        'ai_analysis': aiAnalysis.map((e) => e.toJson()).toList(),
        'history': history.map((e) => e.toJson()).toList(),
        'reviews': reviews.map((e) => e.toJson()).toList(),
        'owner_display_name': ownerDisplayName,
      };
}

/// Body of POST /observations.
class ObservationCreate {
  const ObservationCreate({
    this.recordType = RecordType.observation,
    required this.category,
    this.title,
    this.description,
    this.speciesGuess,
    this.countObserved,
    this.details,
    required this.lat,
    required this.lon,
    this.locationSource,
    this.accuracyM,
    required this.observedAt,
    this.draft = false,
  });

  final RecordType recordType;
  final ObservationCategory category;
  final String? title;
  final String? description;
  final String? speciesGuess;
  final int? countObserved;
  final Map<String, dynamic>? details;
  final double lat;
  final double lon;

  /// `gps` or `manual`.
  final String? locationSource;
  final double? accuracyM;
  final DateTime observedAt;
  final bool draft;

  factory ObservationCreate.fromJson(Json j) => ObservationCreate(
        recordType: RecordType.fromWire(strOrNull(j['record_type'])),
        category: ObservationCategory.fromWire(strOrNull(j['category'])),
        title: strOrNull(j['title']),
        description: strOrNull(j['description']),
        speciesGuess: strOrNull(j['species_guess']),
        countObserved: intOrNull(j['count_observed']),
        details: asMapOrNull(j['details']),
        lat: doubleOf(j['lat']),
        lon: doubleOf(j['lon']),
        locationSource: strOrNull(j['location_source']),
        accuracyM: doubleOrNull(j['accuracy_m']),
        observedAt: dateOf(j['observed_at']),
        draft: boolOf(j['draft']),
      );

  Json toJson() => compact({
        'record_type': recordType.wire,
        'category': category.wire,
        'title': title,
        'description': description,
        'species_guess': speciesGuess,
        'count_observed': countObserved,
        'details': details,
        'lat': lat,
        'lon': lon,
        'location_source': locationSource,
        'accuracy_m': accuracyM,
        'observed_at': isoOrNull(observedAt),
        'draft': draft ? true : null,
      });
}

/// Body of PATCH /observations/{id}.
class ObservationPatch {
  const ObservationPatch({
    this.title,
    this.description,
    this.speciesGuess,
    this.countObserved,
    this.details,
    this.lat,
    this.lon,
    this.submit,
  });

  final String? title;
  final String? description;
  final String? speciesGuess;
  final int? countObserved;
  final Map<String, dynamic>? details;
  final double? lat;
  final double? lon;

  /// draft -> submitted, or needs_information -> under_review.
  final bool? submit;

  factory ObservationPatch.fromJson(Json j) => ObservationPatch(
        title: strOrNull(j['title']),
        description: strOrNull(j['description']),
        speciesGuess: strOrNull(j['species_guess']),
        countObserved: intOrNull(j['count_observed']),
        details: asMapOrNull(j['details']),
        lat: doubleOrNull(j['lat']),
        lon: doubleOrNull(j['lon']),
        submit: j['submit'] == null ? null : boolOf(j['submit']),
      );

  Json toJson() => compact({
        'title': title,
        'description': description,
        'species_guess': speciesGuess,
        'count_observed': countObserved,
        'details': details,
        'lat': lat,
        'lon': lon,
        'submit': submit,
      });
}

/// Body of POST /reviews/{observationId}/decision.
class ReviewDecision {
  const ReviewDecision({
    required this.decision,
    this.note,
    this.correctedCategory,
    this.correctedSpecies,
    this.markSensitive,
    this.priority,
  });

  final ReviewDecisionType decision;

  /// Required by the server for reject and request_info.
  final String? note;
  final String? correctedCategory;
  final String? correctedSpecies;
  final bool? markSensitive;

  /// low | normal | high | urgent
  final String? priority;

  factory ReviewDecision.fromJson(Json j) => ReviewDecision(
        decision: ReviewDecisionType.values.firstWhere(
          (d) => d.wire == j['decision'],
          orElse: () => ReviewDecisionType.startReview,
        ),
        note: strOrNull(j['note']),
        correctedCategory: strOrNull(j['corrected_category']),
        correctedSpecies: strOrNull(j['corrected_species']),
        markSensitive:
            j['mark_sensitive'] == null ? null : boolOf(j['mark_sensitive']),
        priority: strOrNull(j['priority']),
      );

  Json toJson() => compact({
        'decision': decision.wire,
        'note': note,
        'corrected_category': correctedCategory,
        'corrected_species': correctedSpecies,
        'mark_sensitive': markSensitive,
        'priority': priority,
      });
}
