import 'enums.dart';
import 'json.dart';

class ProviderStatus {
  const ProviderStatus({
    this.configured = false,
    this.provider = '',
    this.model = '',
  });
  final bool configured;
  final String provider;
  final String model;

  factory ProviderStatus.fromJson(Json j) => ProviderStatus(
        configured: boolOf(j['configured']),
        provider: str(j['provider']),
        model: str(j['model']),
      );
  Json toJson() =>
      {'configured': configured, 'provider': provider, 'model': model};
}

/// Payload of GET /ai/providers/status.
class AiProvidersStatus {
  const AiProvidersStatus({
    this.textGeneration = const ProviderStatus(),
    this.vision = const ProviderStatus(),
    this.embedding = const ProviderStatus(),
    this.speciesClassification = const ProviderStatus(),
  });
  final ProviderStatus textGeneration;
  final ProviderStatus vision;
  final ProviderStatus embedding;
  final ProviderStatus speciesClassification;

  factory AiProvidersStatus.fromJson(Json j) => AiProvidersStatus(
        textGeneration: ProviderStatus.fromJson(asMap(j['text_generation'])),
        vision: ProviderStatus.fromJson(asMap(j['vision'])),
        embedding: ProviderStatus.fromJson(asMap(j['embedding'])),
        speciesClassification:
            ProviderStatus.fromJson(asMap(j['species_classification'])),
      );
  Json toJson() => {
        'text_generation': textGeneration.toJson(),
        'vision': vision.toJson(),
        'embedding': embedding.toJson(),
        'species_classification': speciesClassification.toJson(),
      };
}

/// Payload of GET /health.
class HealthStatus {
  const HealthStatus({
    this.status = '',
    this.version = '',
    this.aiConfigured = false,
    this.embeddingsConfigured = false,
    this.vectorSearch = false,
  });
  final String status;
  final String version;
  final bool aiConfigured;
  final bool embeddingsConfigured;
  final bool vectorSearch;

  factory HealthStatus.fromJson(Json j) => HealthStatus(
        status: str(j['status']),
        version: str(j['version']),
        aiConfigured: boolOf(j['ai_configured']),
        embeddingsConfigured: boolOf(j['embeddings_configured']),
        vectorSearch: boolOf(j['vector_search']),
      );
  Json toJson() => {
        'status': status,
        'version': version,
        'ai_configured': aiConfigured,
        'embeddings_configured': embeddingsConfigured,
        'vector_search': vectorSearch,
      };
}

/// One citation backing an answer.
class SourceRef {
  const SourceRef({
    required this.id,
    this.type = '',
    this.title = '',
    this.url,
    this.entityId,
    this.date,
    this.snippet = '',
    this.status,
  });

  /// `[n]` label used inside the answer text.
  final String id;

  /// observation | rss_article | scientific | region_report | sql_fact
  final String type;
  final String title;
  final String? url;

  /// Open this observation / article in the UI.
  final String? entityId;
  final String? date;
  final String snippet;

  /// Verification status for observations.
  final String? status;

  factory SourceRef.fromJson(Json j) => SourceRef(
        id: str(j['id']),
        type: str(j['type']),
        title: str(j['title']),
        url: strOrNull(j['url']),
        entityId: strOrNull(j['entity_id']),
        date: strOrNull(j['date']),
        snippet: str(j['snippet']),
        status: strOrNull(j['status']),
      );

  Json toJson() => {
        'id': id,
        'type': type,
        'title': title,
        'url': url,
        'entity_id': entityId,
        'date': date,
        'snippet': snippet,
        'status': status,
      };
}

class ChatFact {
  const ChatFact({this.label = '', this.value = '', this.queryNote = ''});
  final String label;
  final String value;
  final String queryNote;

  factory ChatFact.fromJson(Json j) => ChatFact(
        label: str(j['label']),
        value: str(j['value']),
        queryNote: str(j['query_note']),
      );
  Json toJson() =>
      {'label': label, 'value': value, 'query_note': queryNote};
}

class ChatRegionRef {
  const ChatRegionRef({required this.id, this.name = ''});
  final String id;
  final String name;

  factory ChatRegionRef.fromJson(Json j) =>
      ChatRegionRef(id: str(j['id']), name: str(j['name']));
  Json toJson() => {'id': id, 'name': name};
}

class ChatAnswer {
  const ChatAnswer({
    this.answer = '',
    this.mode = ChatMode.retrievalOnly,
    this.region,
    this.period = const AnalyticsPeriodRef(),
    this.facts = const [],
    this.sources = const [],
    this.limitations = const [],
    this.needsRegion = false,
  });

  final String answer;
  final ChatMode mode;
  final ChatRegionRef? region;
  final AnalyticsPeriodRef period;

  /// Numbers computed by SQL (never by the LLM).
  final List<ChatFact> facts;
  final List<SourceRef> sources;
  final List<String> limitations;
  final bool needsRegion;

  factory ChatAnswer.fromJson(Json j) => ChatAnswer(
        answer: str(j['answer']),
        mode: ChatMode.fromWire(strOrNull(j['mode'])),
        region: asMapOrNull(j['region']) == null
            ? null
            : ChatRegionRef.fromJson(asMap(j['region'])),
        period: AnalyticsPeriodRef.fromJson(asMap(j['period'])),
        facts: listOf(j['facts'], ChatFact.fromJson),
        sources: listOf(j['sources'], SourceRef.fromJson),
        limitations: stringList(j['limitations']),
        needsRegion: boolOf(j['needs_region']),
      );

  Json toJson() => {
        'answer': answer,
        'mode': mode.wire,
        'region': region?.toJson(),
        'period': period.toJson(),
        'facts': facts.map((e) => e.toJson()).toList(),
        'sources': sources.map((e) => e.toJson()).toList(),
        'limitations': limitations,
        'needs_region': needsRegion,
      };
}

/// `{from, to}` pair as used in ChatAnswer.period.
class AnalyticsPeriodRef {
  const AnalyticsPeriodRef({this.from = '', this.to = ''});
  final String from;
  final String to;

  factory AnalyticsPeriodRef.fromJson(Json j) =>
      AnalyticsPeriodRef(from: str(j['from']), to: str(j['to']));
  Json toJson() => {'from': from, 'to': to};
}

class ChatHistoryItem {
  const ChatHistoryItem({required this.role, required this.content});

  /// `user` or `assistant`.
  final String role;
  final String content;

  factory ChatHistoryItem.fromJson(Json j) =>
      ChatHistoryItem(role: str(j['role']), content: str(j['content']));
  Json toJson() => {'role': role, 'content': content};
}

/// Body of POST /ai/chat.
class ChatRequest {
  const ChatRequest({
    required this.message,
    this.regionId,
    this.from,
    this.to,
    this.history = const [],
  });
  final String message;
  final String? regionId;
  final String? from;
  final String? to;
  final List<ChatHistoryItem> history;

  factory ChatRequest.fromJson(Json j) => ChatRequest(
        message: str(j['message']),
        regionId: strOrNull(j['region_id']),
        from: strOrNull(j['from']),
        to: strOrNull(j['to']),
        history: listOf(j['history'], ChatHistoryItem.fromJson),
      );

  Json toJson() => compact({
        'message': message,
        'region_id': regionId,
        'from': from,
        'to': to,
        'history':
            history.isEmpty ? null : history.map((e) => e.toJson()).toList(),
      });
}
