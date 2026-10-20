/// Tolerant JSON readers shared by the model classes. The backend is Go, so
/// numbers may arrive as int or double and absent fields as null/missing.
typedef Json = Map<String, dynamic>;

Json asMap(Object? v) {
  if (v is Map<String, dynamic>) return v;
  if (v is Map) return Map<String, dynamic>.from(v);
  return const <String, dynamic>{};
}

Json? asMapOrNull(Object? v) => v == null ? null : asMap(v);

String str(Object? v, [String fallback = '']) => v == null ? fallback : '$v';

String? strOrNull(Object? v) => v == null ? null : '$v';

int intOf(Object? v, [int fallback = 0]) {
  if (v is int) return v;
  if (v is num) return v.toInt();
  if (v is String) return int.tryParse(v) ?? fallback;
  return fallback;
}

int? intOrNull(Object? v) => v == null ? null : intOf(v);

double doubleOf(Object? v, [double fallback = 0]) {
  if (v is num) return v.toDouble();
  if (v is String) return double.tryParse(v) ?? fallback;
  return fallback;
}

double? doubleOrNull(Object? v) => v == null ? null : doubleOf(v);

bool boolOf(Object? v, [bool fallback = false]) {
  if (v is bool) return v;
  if (v is num) return v != 0;
  if (v is String) return v == 'true' || v == '1';
  return fallback;
}

DateTime? dateOrNull(Object? v) {
  if (v == null) return null;
  return DateTime.tryParse('$v');
}

DateTime dateOf(Object? v) =>
    dateOrNull(v) ?? DateTime.fromMillisecondsSinceEpoch(0, isUtc: true);

List<T> listOf<T>(Object? v, T Function(Json) parse) {
  if (v is! List) return const [];
  return List<T>.unmodifiable(v.map((e) => parse(asMap(e))));
}

List<String> stringList(Object? v) {
  if (v is! List) return const [];
  return List<String>.unmodifiable(v.map((e) => '$e'));
}

List<double> doubleList(Object? v) {
  if (v is! List) return const [];
  return List<double>.unmodifiable(v.map(doubleOf));
}

String? isoOrNull(DateTime? d) => d?.toUtc().toIso8601String();

/// Removes null values so request bodies stay minimal.
Json compact(Json m) =>
    Json.fromEntries(m.entries.where((e) => e.value != null));
