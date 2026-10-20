import 'json.dart';

/// Body of POST /simulations/run.
class SimulationRunRequest {
  const SimulationRunRequest({
    this.name,
    this.model = 'climate_drought_v1',
    required this.regionId,
    required this.referenceFrom,
    required this.referenceTo,
    this.precipChangePct,
    this.tempChangeC,
    this.explain,
  });

  final String? name;
  final String model;
  final String regionId;

  /// `YYYY-MM` inclusive (>= 10 years recommended).
  final String referenceFrom;
  final String referenceTo;

  /// e.g. -20 means 20% less precipitation.
  final double? precipChangePct;

  /// e.g. 2 means +2 degC.
  final double? tempChangeC;
  final bool? explain;

  factory SimulationRunRequest.fromJson(Json j) => SimulationRunRequest(
        name: strOrNull(j['name']),
        model: str(j['model'], 'climate_drought_v1'),
        regionId: str(j['region_id']),
        referenceFrom: str(j['reference_from']),
        referenceTo: str(j['reference_to']),
        precipChangePct: doubleOrNull(j['precip_change_pct']),
        tempChangeC: doubleOrNull(j['temp_change_c']),
        explain: j['explain'] == null ? null : boolOf(j['explain']),
      );

  Json toJson() => compact({
        'name': name,
        'model': model,
        'region_id': regionId,
        'reference_from': referenceFrom,
        'reference_to': referenceTo,
        'precip_change_pct': precipChangePct,
        'temp_change_c': tempChangeC,
        'explain': explain,
      });
}

class SimulationIndicator {
  const SimulationIndicator({
    required this.key,
    this.label = '',
    this.unit = '',
    this.reference = 0,
    this.scenario = 0,
    this.change = 0,
  });
  final String key;
  final String label;
  final String unit;
  final double reference;
  final double scenario;
  final double change;

  factory SimulationIndicator.fromJson(Json j) => SimulationIndicator(
        key: str(j['key']),
        label: str(j['label']),
        unit: str(j['unit']),
        reference: doubleOf(j['reference']),
        scenario: doubleOf(j['scenario']),
        change: doubleOf(j['change']),
      );

  Json toJson() => {
        'key': key,
        'label': label,
        'unit': unit,
        'reference': reference,
        'scenario': scenario,
        'change': change,
      };
}

class SeriesPoint {
  const SeriesPoint({
    required this.month,
    this.reference = 0,
    this.scenario = 0,
  });
  final String month;
  final double reference;
  final double scenario;

  factory SeriesPoint.fromJson(Json j) => SeriesPoint(
        month: str(j['month']),
        reference: doubleOf(j['reference']),
        scenario: doubleOf(j['scenario']),
      );
  Json toJson() =>
      {'month': month, 'reference': reference, 'scenario': scenario};
}

class SimulationSeries {
  const SimulationSeries({required this.key, this.points = const []});
  final String key;
  final List<SeriesPoint> points;

  factory SimulationSeries.fromJson(Json j) => SimulationSeries(
        key: str(j['key']),
        points: listOf(j['points'], SeriesPoint.fromJson),
      );
  Json toJson() =>
      {'key': key, 'points': points.map((e) => e.toJson()).toList()};
}

class SimulationDataSource {
  const SimulationDataSource({
    this.dataset = '',
    this.source = '',
    this.license = '',
    this.coverage = 0,
  });
  final String dataset;
  final String source;
  final String license;
  final double coverage;

  factory SimulationDataSource.fromJson(Json j) => SimulationDataSource(
        dataset: str(j['dataset']),
        source: str(j['source']),
        license: str(j['license']),
        coverage: doubleOf(j['coverage']),
      );
  Json toJson() => {
        'dataset': dataset,
        'source': source,
        'license': license,
        'coverage': coverage,
      };
}

class SimulationResult {
  const SimulationResult({
    required this.id,
    this.scenarioId = '',
    this.name = '',
    this.status = 'ok',
    this.params = const {},
    this.indicators = const [],
    this.series = const [],
    this.dataSources = const [],
    this.assumptions = const [],
    this.limitations = const [],
    this.insufficientReason,
    this.narrative,
    this.createdAt,
  });

  final String id;
  final String scenarioId;
  final String name;

  /// ok | insufficient_data | error
  final String status;
  final Map<String, dynamic> params;
  final List<SimulationIndicator> indicators;
  final List<SimulationSeries> series;
  final List<SimulationDataSource> dataSources;
  final List<String> assumptions;
  final List<String> limitations;
  final String? insufficientReason;
  final String? narrative;
  final DateTime? createdAt;

  bool get isOk => status == 'ok';

  factory SimulationResult.fromJson(Json j) => SimulationResult(
        id: str(j['id']),
        scenarioId: str(j['scenario_id']),
        name: str(j['name']),
        status: str(j['status'], 'ok'),
        params: asMap(j['params']),
        indicators: listOf(j['indicators'], SimulationIndicator.fromJson),
        series: listOf(j['series'], SimulationSeries.fromJson),
        dataSources: listOf(j['data_sources'], SimulationDataSource.fromJson),
        assumptions: stringList(j['assumptions']),
        limitations: stringList(j['limitations']),
        insufficientReason: strOrNull(j['insufficient_reason']),
        narrative: strOrNull(j['narrative']),
        createdAt: dateOrNull(j['created_at']),
      );

  Json toJson() => {
        'id': id,
        'scenario_id': scenarioId,
        'name': name,
        'status': status,
        'params': params,
        'indicators': indicators.map((e) => e.toJson()).toList(),
        'series': series.map((e) => e.toJson()).toList(),
        'data_sources': dataSources.map((e) => e.toJson()).toList(),
        'assumptions': assumptions,
        'limitations': limitations,
        'insufficient_reason': insufficientReason,
        'narrative': narrative,
        'created_at': isoOrNull(createdAt),
      };
}

class EnvironmentPoint {
  const EnvironmentPoint({required this.ts, this.value = 0});
  final String ts;
  final double value;

  factory EnvironmentPoint.fromJson(Json j) =>
      EnvironmentPoint(ts: str(j['ts']), value: doubleOf(j['value']));
  Json toJson() => {'ts': ts, 'value': value};
}

/// Payload of GET /environment/measurements.
class EnvironmentSeries {
  const EnvironmentSeries({
    this.dataset = '',
    this.unit = '',
    this.coverage = 0,
    this.points = const [],
  });
  final String dataset;
  final String unit;

  /// Fraction of expected points present.
  final double coverage;
  final List<EnvironmentPoint> points;

  factory EnvironmentSeries.fromJson(Json j) => EnvironmentSeries(
        dataset: str(j['dataset']),
        unit: str(j['unit']),
        coverage: doubleOf(j['coverage']),
        points: listOf(j['points'], EnvironmentPoint.fromJson),
      );
  Json toJson() => {
        'dataset': dataset,
        'unit': unit,
        'coverage': coverage,
        'points': points.map((e) => e.toJson()).toList(),
      };
}
