import '../../models/models.dart';
import '../api_client.dart';

/// `/ai/*`.
class AiApi {
  const AiApi(this._c);
  final ApiClient _c;

  Future<AiProvidersStatus> providersStatus() async =>
      AiProvidersStatus.fromJson((await _c.get('/ai/providers/status')).object);

  /// Geo-aware hybrid RAG answer. Works without an AI key (retrieval_only).
  Future<ChatAnswer> chat(ChatRequest req) async =>
      ChatAnswer.fromJson((await _c.post('/ai/chat', body: req.toJson())).object);

  /// Queues/runs an AI analysis of an observation's photos. Result is only a
  /// suggestion. Throws ApiException(ai_not_configured) on 503.
  Future<AIAnalysis> analyzeObservation(String observationId) async =>
      AIAnalysis.fromJson((await _c.post(
        '/ai/analyze-observation',
        body: {'observation_id': observationId},
      ))
          .object);
}
