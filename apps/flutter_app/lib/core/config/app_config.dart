/// Build-time configuration. Override with
/// `--dart-define=API_BASE_URL=https://host/api/v1`.
class AppConfig {
  const AppConfig({required this.apiBaseUrl});

  static const String _envBaseUrl = String.fromEnvironment(
    'API_BASE_URL',
    defaultValue: 'http://localhost:8080/api/v1',
  );

  /// Config used by the running app.
  static const AppConfig fromEnvironment = AppConfig(apiBaseUrl: _envBaseUrl);

  /// Base URL including the `/api/v1` prefix, without a trailing slash.
  final String apiBaseUrl;

  /// Server origin (scheme://host:port) derived from [apiBaseUrl].
  String get origin {
    final uri = Uri.parse(apiBaseUrl);
    return '${uri.scheme}://${uri.authority}';
  }

  /// Resolves a (possibly relative) URL from the API, e.g. Media.url
  /// (`/api/v1/media/{id}`), to an absolute URL.
  String resolveUrl(String pathOrUrl) {
    if (pathOrUrl.startsWith('http://') || pathOrUrl.startsWith('https://')) {
      return pathOrUrl;
    }
    if (pathOrUrl.startsWith('/')) return '$origin$pathOrUrl';
    return '$apiBaseUrl/$pathOrUrl';
  }

  /// Absolute URL of a media item, optionally the 400px thumbnail.
  String mediaUrl(String mediaId, {bool thumb = false}) =>
      '$apiBaseUrl/media/$mediaId${thumb ? '?thumb=1' : ''}';
}

/// Map defaults. The OSM public tile servers are for light use only; point
/// `TILE_URL` at a proper provider / own tile server in production.
class MapConfig {
  const MapConfig._();

  static const String tileUrlTemplate = String.fromEnvironment(
    'TILE_URL',
    defaultValue: 'https://tile.openstreetmap.org/{z}/{x}/{y}.png',
  );
  static const String userAgentPackageName = 'org.ecoterminal.app';

  /// Kastamonu centre.
  static const double startLat = 41.38;
  static const double startLon = 33.78;
  static const double startZoom = 8;
}
