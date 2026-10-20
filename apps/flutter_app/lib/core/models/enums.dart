// Wire enums from openapi.yaml. Each has a tolerant `fromWire` that maps
// unknown values to `unknown`/`other` instead of crashing on a newer backend.

enum ObservationStatus {
  draft,
  submitted,
  aiProcessed('ai_processed'),
  underReview('under_review'),
  verified,
  needsInformation('needs_information'),
  rejected,
  resolved,
  unknown('unknown');

  const ObservationStatus([String? wireName]) : _wire = wireName;
  final String? _wire;

  String get wire => _wire ?? name;

  /// The 8 statuses of the lifecycle (without [unknown]).
  static const List<ObservationStatus> lifecycle = [
    draft,
    submitted,
    aiProcessed,
    underReview,
    verified,
    needsInformation,
    rejected,
    resolved,
  ];

  static ObservationStatus fromWire(String? v) {
    for (final s in values) {
      if (s.wire == v) return s;
    }
    return unknown;
  }

  /// Statuses a non-staff caller can see for other people's records.
  bool get isPublic => this == verified || this == resolved;
}

enum ObservationCategory {
  animal,
  plant,
  water,
  forest,
  pollution,
  injuredWildlife('injured_wildlife'),
  other;

  const ObservationCategory([String? wireName]) : _wire = wireName;
  final String? _wire;

  String get wire => _wire ?? name;

  static ObservationCategory fromWire(String? v) {
    for (final s in values) {
      if (s.wire == v) return s;
    }
    return other;
  }
}

enum RecordType {
  observation,
  issue;

  String get wire => name;

  static RecordType fromWire(String? v) =>
      v == 'issue' ? RecordType.issue : RecordType.observation;
}

enum UserRole {
  citizen,
  researcher,
  reviewer,
  admin;

  String get wire => name;

  static UserRole fromWire(String? v) {
    for (final r in values) {
      if (r.name == v) return r;
    }
    return UserRole.citizen;
  }

  bool get isStaffReviewer => this == reviewer || this == admin;
  bool get canSimulate => this == researcher || this == admin;
}

enum AiAnalysisStatus {
  pending,
  running,
  done,
  failed,
  skipped,
  unknown;

  static AiAnalysisStatus fromWire(String? v) {
    for (final r in values) {
      if (r.name == v) return r;
    }
    return AiAnalysisStatus.unknown;
  }
}

enum RegionKind {
  province,
  district,
  village,
  grid,
  custom,
  unknown;

  static RegionKind fromWire(String? v) {
    for (final r in values) {
      if (r.name == v) return r;
    }
    return RegionKind.unknown;
  }
}

enum ReviewDecisionType {
  startReview('start_review'),
  verify('verify'),
  reject('reject'),
  requestInfo('request_info'),
  resolve('resolve');

  const ReviewDecisionType(this.wire);
  final String wire;
}

enum ChatMode {
  generative,
  retrievalOnly('retrieval_only');

  const ChatMode([String? wireName]) : _wire = wireName;
  final String? _wire;
  String get wire => _wire ?? name;

  static ChatMode fromWire(String? v) =>
      v == 'generative' ? ChatMode.generative : ChatMode.retrievalOnly;
}
