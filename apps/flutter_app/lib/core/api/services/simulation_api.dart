import '../../models/models.dart';
import '../api_client.dart';

/// `/simulations*` and `/environment/measurements`.
class SimulationApi {
  const SimulationApi(this._c);
  final ApiClient _c;

  /// Researcher/admin. Result `status` may be `insufficient_data`.
  Future<SimulationResult> run(SimulationRunRequest req) async =>
      SimulationResult.fromJson(
          (await _c.post('/simulations/run', body: req.toJson())).object);

  Future<List<SimulationResult>> list() async =>
      (await _c.get('/simulations')).list.map(SimulationResult.fromJson).toList();

  Future<SimulationResult> get(String id) async =>
      SimulationResult.fromJson((await _c.get('/simulations/$id')).object);

  /// [type]: `precipitation` | `temperature_mean`; [resolution]: `daily` | `monthly`.
  Future<EnvironmentSeries> measurements({
    required String regionId,
    required String type,
    String? from,
    String? to,
    String? resolution,
  }) async =>
      EnvironmentSeries.fromJson((await _c.get('/environment/measurements', query: {
        'region_id': regionId,
        'type': type,
        'from': from,
        'to': to,
        'resolution': resolution,
      }))
          .object);
}
